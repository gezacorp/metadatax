// Package macosprocess collects the metadata of a macOS process, with the
// label names the procfs collector uses for what both platforms have.
package macosprocess

import (
	"bytes"
	"context"
	"encoding/binary"
	stderrors "errors"
	"os"
	"strconv"
	"strings"

	"emperror.dev/errors"

	"github.com/gezacorp/metadatax"
)

const name = "process"

// ErrProcessNotFound is returned for a PID with no process behind it.
var ErrProcessNotFound = errors.WrapIf(os.ErrNotExist, "process not found")

const (
	csValid          = 0x00000001
	csAdhoc          = 0x00000002
	csRuntime        = 0x00010000
	csPlatformBinary = 0x04000000

	noAuditID = 0xffffffff

	// The arguments that tell what a process runs come first; the rest are
	// its own options, as long as an Electron helper's can be.
	maxArgs       = 8
	maxCmdlineLen = 1024
)

// truncate cuts s to at most n bytes without splitting a character.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}

	return strings.ToValidUTF8(s[:n], "")
}

// ProcessInfo is what the collector reads about a process.
type ProcessInfo struct {
	// Name is the kernel's short command name.
	Name string
	// Path is the executable; empty when it is no longer on disk.
	Path string
	// ExecPath is the path the process was started with.
	ExecPath string
	Args     []string
	PPID     int32

	RUID, EUID, RGID, EGID uint32
	// Audit is false when the audit token could not be read.
	Audit bool
	// AUID is the audit user ID, the user who logged in; noAuditID when
	// the process has none.
	AUID    uint32
	Session int32

	Signature Signature
	Bundle    Bundle

	Responsible Responsible
}

// Responsible is the process launchd attributes the process to, the app it
// runs on behalf of.
type Responsible struct {
	PID       int32
	Path      string
	Signature Signature
	Bundle    Bundle
}

// Signature is the code signature of the running code as the kernel has it.
type Signature struct {
	Identifier string
	Team       string
	CDHash     string
	Status     uint32
}

// Bundle is the innermost bundle the executable is in.
type Bundle struct {
	Path    string
	ID      string
	Name    string
	Version string
}

type ProcessInfoFunc func(ctx context.Context, pid int32) (ProcessInfo, error)

type collector struct {
	processInfoFunc     ProcessInfoFunc
	mdContainerInitFunc func() metadatax.MetadataContainer
	skipOnSoftError     bool
}

type CollectorOption func(*collector)

func CollectorWithProcessInfoFunc(fn ProcessInfoFunc) CollectorOption {
	return func(c *collector) {
		c.processInfoFunc = fn
	}
}

func CollectorWithMetadataContainerInitFunc(fn func() metadatax.MetadataContainer) CollectorOption {
	return func(c *collector) {
		c.mdContainerInitFunc = fn
	}
}

func WithSkipOnSoftError() CollectorOption {
	return func(c *collector) {
		c.skipOnSoftError = true
	}
}

func New(opts ...CollectorOption) metadatax.Collector {
	c := &collector{
		processInfoFunc: inspect,
	}

	for _, f := range opts {
		f(c)
	}

	if c.mdContainerInitFunc == nil {
		c.mdContainerInitFunc = func() metadatax.MetadataContainer {
			return metadatax.New(metadatax.WithPrefix(name))
		}
	}

	return c
}

func (c *collector) GetMetadata(ctx context.Context) (metadatax.MetadataContainer, error) {
	md := c.mdContainerInitFunc()

	pid, found := metadatax.PIDFromContext(ctx)
	if !found {
		return nil, metadatax.ErrPIDNotFound
	}

	p, err := c.processInfoFunc(ctx, pid)
	if errors.Is(err, stderrors.ErrUnsupported) {
		return md, nil
	}

	if err != nil {
		if c.skipOnSoftError {
			return md, nil
		}

		return nil, errors.WrapIf(err, "could not read process")
	}

	md.AddLabel("pid", strconv.Itoa(int(pid)))
	addLabel(md, "name", p.Name)
	addLabel(md, "cmdline", truncate(strings.Join(p.Args, " "), maxCmdlineLen))

	argmd := md.Segment("arg")
	for i, a := range p.Args[:min(len(p.Args), maxArgs)] {
		argmd.AddLabel(strconv.Itoa(i), a)
	}

	md.AddLabel("ppid", strconv.Itoa(int(p.PPID)))

	uid := md.Segment("uid")
	uid.AddLabel("real", strconv.FormatUint(uint64(p.RUID), 10))
	uid.AddLabel("effective", strconv.FormatUint(uint64(p.EUID), 10))
	uid.AddLabel("", strconv.FormatUint(uint64(p.EUID), 10))

	if p.Audit && p.AUID != noAuditID {
		uid.AddLabel("login", strconv.FormatUint(uint64(p.AUID), 10))
	}

	gid := md.Segment("gid")
	gid.AddLabel("real", strconv.FormatUint(uint64(p.RGID), 10))
	gid.AddLabel("effective", strconv.FormatUint(uint64(p.EGID), 10))
	gid.AddLabel("", strconv.FormatUint(uint64(p.EGID), 10))

	if p.Audit {
		md.AddLabel("session", strconv.Itoa(int(p.Session)))
	}

	bin := md.Segment("binary")

	if p.Path != "" {
		bin.AddLabel("path", p.Path)
	} else {
		addLabel(bin, "path", p.ExecPath)
	}

	addSignature(md, p.Signature)
	addBundle(md, p.Bundle)

	if r := p.Responsible; r.PID > 0 {
		rmd := md.Segment("responsible")
		rmd.AddLabel("pid", strconv.Itoa(int(r.PID)))
		addLabel(rmd, "path", r.Path)
		addSignature(rmd, r.Signature)
		addBundle(rmd, r.Bundle)
	}

	return md, nil
}

func addSignature(md metadatax.MetadataContainer, s Signature) {
	if s.Status&csValid == 0 {
		return
	}

	sig := md.Segment("signature")
	addLabel(sig, "identifier", s.Identifier)
	addLabel(sig, "team", s.Team)
	addLabel(sig, "cdhash", s.CDHash)
	sig.AddLabel("platform", strconv.FormatBool(s.Status&csPlatformBinary != 0))
	sig.AddLabel("adhoc", strconv.FormatBool(s.Status&csAdhoc != 0))
	sig.AddLabel("hardened-runtime", strconv.FormatBool(s.Status&csRuntime != 0))
}

func addBundle(md metadatax.MetadataContainer, b Bundle) {
	if b.Path == "" {
		return
	}

	bundle := md.Segment("bundle")
	bundle.AddLabel("path", b.Path)
	addLabel(bundle, "id", b.ID)
	addLabel(bundle, "name", b.Name)
	addLabel(bundle, "version", b.Version)
}

func addLabel(md metadatax.MetadataContainer, name, value string) {
	if value != "" {
		md.AddLabel(name, value)
	}
}

// bundlePath is the innermost bundle exe sits in as its main executable,
// the directory holding Contents/MacOS, or empty.
func bundlePath(exe string) string {
	const marker = "/Contents/MacOS/"

	i := strings.LastIndex(exe, marker)
	if i <= 0 || strings.Contains(exe[i+len(marker):], "/") {
		return ""
	}

	return exe[:i]
}

// parseProcArgs reads a KERN_PROCARGS2 buffer: the argument count, the
// exec path, padding, then the arguments, each NUL terminated.
func parseProcArgs(buf []byte) (string, []string) {
	if len(buf) < 4 {
		return "", nil
	}

	argc := int(binary.LittleEndian.Uint32(buf))
	rest := buf[4:]

	i := bytes.IndexByte(rest, 0)
	if i < 0 {
		return string(rest), nil
	}

	execPath := string(rest[:i])
	rest = rest[i:]

	for len(rest) > 0 && rest[0] == 0 {
		rest = rest[1:]
	}

	args := make([]string, 0, min(argc, len(rest)))
	for len(args) < argc && len(rest) > 0 {
		i := bytes.IndexByte(rest, 0)
		if i < 0 {
			args = append(args, string(rest))

			break
		}

		args = append(args, string(rest[:i]))
		rest = rest[i+1:]
	}

	return execPath, args
}
