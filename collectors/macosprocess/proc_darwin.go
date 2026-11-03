//go:build darwin && cgo

package macosprocess

/*
#cgo LDFLAGS: -lbsm -framework CoreFoundation
#include <stdlib.h>
#include <string.h>
#include <libproc.h>
#include <sys/proc_info.h>
#include <mach/mach.h>
#include <bsm/libbsm.h>
#include <CoreFoundation/CoreFoundation.h>

extern pid_t responsibility_get_pid_responsible_for_pid(pid_t);
extern int csops(pid_t, unsigned int, void *, size_t);

#define CS_OPS_STATUS 0
#define CS_OPS_CDHASH 5
#define CS_OPS_IDENTITY 11
#define CS_OPS_TEAMID 14

static int mxp_audit(pid_t pid, uid_t *auid, int *asid)
{
	mach_port_t task;
	if (task_name_for_pid(mach_task_self(), pid, &task) != KERN_SUCCESS)
		return -1;

	audit_token_t tok;
	mach_msg_type_number_t n = TASK_AUDIT_TOKEN_COUNT;
	kern_return_t kr = task_info(task, TASK_AUDIT_TOKEN, (task_info_t)&tok, &n);
	mach_port_deallocate(mach_task_self(), task);
	if (kr != KERN_SUCCESS || audit_token_to_pid(tok) != pid)
		return -1;

	*auid = audit_token_to_auid(tok);
	*asid = audit_token_to_asid(tok);

	return 0;
}

// mxp_csblob reads a csops string blob, the identity or the team ID.
static int mxp_csblob(pid_t pid, unsigned int op, char *out, size_t size)
{
	struct {
		uint32_t magic, length;
		char s[256];
	} blob;

	memset(&blob, 0, sizeof(blob));
	if (csops(pid, op, &blob, sizeof(blob)) != 0)
		return -1;

	blob.s[sizeof(blob.s) - 1] = 0;
	strlcpy(out, blob.s, size);

	return 0;
}

static void mxp_cfstring(CFTypeRef v, char *out, size_t size)
{
	out[0] = 0;
	if (v && CFGetTypeID(v) == CFStringGetTypeID())
		CFStringGetCString((CFStringRef)v, out, size, kCFStringEncodingUTF8);
}

// mxp_plist reads a bundle's identity from its Info.plist, XML or binary.
// Parsing the file directly, not through CFBundle, which keeps bundles and
// their Info.plist cached for the life of the process.
static int mxp_plist(const UInt8 *bytes, CFIndex len, char *id, char *name, char *version, size_t size)
{
	CFDataRef data = CFDataCreate(NULL, bytes, len);
	if (!data)
		return -1;

	CFPropertyListRef plist = CFPropertyListCreateWithData(NULL, data, kCFPropertyListImmutable, NULL, NULL);
	CFRelease(data);
	if (!plist)
		return -1;

	if (CFGetTypeID(plist) != CFDictionaryGetTypeID()) {
		CFRelease(plist);
		return -1;
	}

	CFDictionaryRef dict = (CFDictionaryRef)plist;
	mxp_cfstring(CFDictionaryGetValue(dict, CFSTR("CFBundleIdentifier")), id, size);
	mxp_cfstring(CFDictionaryGetValue(dict, CFSTR("CFBundleName")), name, size);
	mxp_cfstring(CFDictionaryGetValue(dict, CFSTR("CFBundleShortVersionString")), version, size);
	CFRelease(plist);

	return 0;
}
*/
import "C"

import (
	"context"
	"encoding/hex"
	"os"
	"path/filepath"
	"sync"
	"unsafe"

	"golang.org/x/sys/unix"
)

const (
	csOpsStatus   = C.CS_OPS_STATUS
	csOpsCDHash   = C.CS_OPS_CDHASH
	csOpsIdentity = C.CS_OPS_IDENTITY
	csOpsTeamID   = C.CS_OPS_TEAMID
)

func inspect(_ context.Context, pid int32) (ProcessInfo, error) {
	var bsd C.struct_proc_bsdshortinfo

	if n := C.proc_pidinfo(C.int(pid), C.PROC_PIDT_SHORTBSDINFO, 0, unsafe.Pointer(&bsd), C.int(unsafe.Sizeof(bsd))); n != C.int(unsafe.Sizeof(bsd)) {
		return ProcessInfo{}, ErrProcessNotFound
	}

	p := ProcessInfo{
		Name: C.GoString(&bsd.pbsi_comm[0]),
		PPID: int32(bsd.pbsi_ppid),
		RUID: uint32(bsd.pbsi_ruid),
		EUID: uint32(bsd.pbsi_uid),
		RGID: uint32(bsd.pbsi_rgid),
		EGID: uint32(bsd.pbsi_gid),
	}

	path := make([]byte, C.PROC_PIDPATHINFO_MAXSIZE)

	if n := C.proc_pidpath(C.int(pid), unsafe.Pointer(&path[0]), C.uint32_t(len(path))); n > 0 {
		p.Path = string(path[:n])
	}

	if buf, err := unix.SysctlRaw("kern.procargs2", int(pid)); err == nil {
		p.ExecPath, p.Args = parseProcArgs(buf)
	}

	var (
		auid C.uid_t
		asid C.int
	)

	if C.mxp_audit(C.pid_t(pid), &auid, &asid) == 0 {
		p.Audit, p.AUID, p.Session = true, uint32(auid), int32(asid)
	}

	p.Signature = signature(pid)

	exe := p.Path
	if exe == "" {
		exe = p.ExecPath
	}

	p.Bundle = bundleOf(exe)

	r := int32(C.responsibility_get_pid_responsible_for_pid(C.pid_t(pid)))

	switch {
	case r <= 0:
	case r == pid:
		p.Responsible = Responsible{PID: r, Path: exe, Signature: p.Signature, Bundle: p.Bundle}
	default:
		p.Responsible = Responsible{
			PID:       r,
			Signature: signature(r),
		}

		if n := C.proc_pidpath(C.int(r), unsafe.Pointer(&path[0]), C.uint32_t(len(path))); n > 0 {
			p.Responsible.Path = string(path[:n])
		} else if buf, err := unix.SysctlRaw("kern.procargs2", int(r)); err == nil {
			p.Responsible.Path, _ = parseProcArgs(buf)
		}

		p.Responsible.Bundle = bundleOf(p.Responsible.Path)
	}

	return p, nil
}

func bundleOf(exe string) Bundle {
	if bp := bundlePath(exe); bp != "" {
		return bundles.get(bp)
	}

	return Bundle{}
}

// maxCachedBundles bounds the bundles kept; a Mac runs a few hundred apps
// and helpers at most.
const maxCachedBundles = 256

// bundles keeps each bundle's identity until its Info.plist changes, which
// an app update does.
var bundles = &bundleCache{entries: map[string]*bundleEntry{}}

type bundleCache struct {
	mu      sync.Mutex
	tick    uint64
	entries map[string]*bundleEntry
}

type bundleEntry struct {
	bundle  Bundle
	modTime int64
	used    uint64
}

func (c *bundleCache) get(path string) Bundle {
	plist := filepath.Join(path, "Contents", "Info.plist")

	var modTime int64

	if fi, err := os.Stat(plist); err == nil {
		modTime = fi.ModTime().UnixNano()
	}

	c.mu.Lock()
	c.tick++

	if e, ok := c.entries[path]; ok && e.modTime == modTime {
		e.used = c.tick
		c.mu.Unlock()

		return e.bundle
	}

	c.mu.Unlock()

	b := bundle(path, plist)

	c.mu.Lock()
	defer c.mu.Unlock()

	if _, ok := c.entries[path]; !ok && len(c.entries) >= maxCachedBundles {
		c.evict()
	}

	c.entries[path] = &bundleEntry{bundle: b, modTime: modTime, used: c.tick}

	return b
}

// evict drops the bundle used longest ago; call with mu held.
func (c *bundleCache) evict() {
	var (
		oldest string
		used   uint64
	)

	for path, e := range c.entries {
		if oldest == "" || e.used < used {
			oldest, used = path, e.used
		}
	}

	delete(c.entries, oldest)
}

func signature(pid int32) Signature {
	var s Signature

	var status C.uint32_t

	if C.csops(C.pid_t(pid), csOpsStatus, unsafe.Pointer(&status), C.size_t(unsafe.Sizeof(status))) != 0 {
		return s
	}

	s.Status = uint32(status)

	var cdhash [20]byte

	if C.csops(C.pid_t(pid), csOpsCDHash, unsafe.Pointer(&cdhash[0]), C.size_t(len(cdhash))) == 0 {
		s.CDHash = hex.EncodeToString(cdhash[:])
	}

	buf := (*C.char)(C.malloc(256))
	defer C.free(unsafe.Pointer(buf))

	if C.mxp_csblob(C.pid_t(pid), csOpsIdentity, buf, 256) == 0 {
		s.Identifier = C.GoString(buf)
	}

	if C.mxp_csblob(C.pid_t(pid), csOpsTeamID, buf, 256) == 0 {
		s.Team = C.GoString(buf)
	}

	return s
}

func bundle(path, plist string) Bundle {
	const (
		size     = 512
		maxPlist = 1 << 20
	)

	b := Bundle{Path: path}

	data, err := os.ReadFile(plist)
	if err != nil || len(data) == 0 || len(data) > maxPlist {
		return b
	}

	id := (*C.char)(C.malloc(size))
	defer C.free(unsafe.Pointer(id))

	name := (*C.char)(C.malloc(size))
	defer C.free(unsafe.Pointer(name))

	version := (*C.char)(C.malloc(size))
	defer C.free(unsafe.Pointer(version))

	if C.mxp_plist((*C.UInt8)(unsafe.Pointer(&data[0])), C.CFIndex(len(data)), id, name, version, size) == 0 {
		b.ID, b.Name, b.Version = C.GoString(id), C.GoString(name), C.GoString(version)
	}

	return b
}
