// Package macosprocess collects process metadata on macOS, under the procfs
// collector's label names where both platforms have the same data.
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

var ErrProcessNotFound = errors.WrapIf(os.ErrNotExist, "process not found")

const (
	csValid          = 0x00000001
	csAdhoc          = 0x00000002
	csRuntime        = 0x00010000
	csPlatformBinary = 0x04000000

	noAuditID = 0xffffffff

	defaultMaxArgs       = 32
	defaultMaxCmdlineLen = 1024
)

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}

	return strings.ToValidUTF8(s[:n], "")
}

// argsWithin returns at most maxArgs leading args whose space-joined line
// fits in maxLen bytes, the last one cut to fit.
func argsWithin(args []string, maxArgs, maxLen int) []string {
	var within []string

	for i, a := range args {
		if i > 0 {
			maxLen--
		}

		if i == maxArgs || maxLen <= 0 {
			break
		}

		within = append(within, truncate(a, maxLen))
		maxLen -= len(a)
	}

	return within
}

type ProcessInfo struct {
	Name string
	// Path is empty when the executable is no longer on disk. ExecPath is
	// the path it was started with.
	Path     string
	ExecPath string
	Args     []string
	PPID     int32

	RUID, EUID, RGID, EGID uint32
	Audit                  bool
	AUID                   uint32
	Session                int32

	Signature Signature
	Bundle    Bundle

	Responsible Responsible
}

type Responsible struct {
	PID       int32
	Path      string
	Signature Signature
	Bundle    Bundle
}

// Signature is the code signature of the running process as the kernel
// reports it.
type Signature struct {
	Identifier string
	Team       string
	CDHash     string
	Status     uint32
}

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
	maxArgs             int
	maxCmdlineLen       int
}

type CollectorOption func(*collector)

func CollectorWithMaxArgs(n int) CollectorOption {
	return func(c *collector) {
		c.maxArgs = n
	}
}

func CollectorWithMaxCmdlineLen(n int) CollectorOption {
	return func(c *collector) {
		c.maxCmdlineLen = n
	}
}

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
		maxArgs:         defaultMaxArgs,
		maxCmdlineLen:   defaultMaxCmdlineLen,
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
	args := argsWithin(p.Args, c.maxArgs, c.maxCmdlineLen)
	addLabel(md, "cmdline", strings.Join(args, " "))

	argmd := md.Segment("arg")
	for i, a := range args {
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

func bundlePath(exe string) string {
	const marker = "/Contents/MacOS/"

	i := strings.LastIndex(exe, marker)
	if i <= 0 || strings.Contains(exe[i+len(marker):], "/") {
		return ""
	}

	return exe[:i]
}

// parseProcArgs returns the exec path and arguments of a KERN_PROCARGS2
// buffer: argc, the exec path, NUL padding, the NUL-terminated arguments.
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
