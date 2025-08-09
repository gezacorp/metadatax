package procfs

import (
	"context"
	_ "crypto/sha256"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"emperror.dev/errors"
	"github.com/opencontainers/go-digest"
	"github.com/shirou/gopsutil/v4/net"
	"github.com/shirou/gopsutil/v4/process"

	"github.com/gezacorp/metadatax"
)

const (
	name = "process"

	basePath = "/proc/cmdline"
)

type collector struct {
	hasProcfs       bool
	extractEnvs     bool
	processInfoFunc ProcessInfoFunc

	mdContainerInitFunc func() metadatax.MetadataContainer
	skipOnSoftError     bool
}

type ProcessInfoFunc func(ctx context.Context, pid int32) (ProcessInfo, error)

type ProcessInfo interface {
	NameWithContext(ctx context.Context) (string, error)
	CmdlineWithContext(ctx context.Context) (string, error)
	UidsWithContext(ctx context.Context) ([]uint32, error)
	GidsWithContext(ctx context.Context) ([]uint32, error)
	GroupsWithContext(ctx context.Context) ([]uint32, error)
	EnvironWithContext(ctx context.Context) ([]string, error)
	ExeWithContext(ctx context.Context) (string, error)
	ConnectionsWithContext(ctx context.Context) ([]net.ConnectionStat, error)
}

type CollectorOption func(*collector)

func CollectorWithExtractENVs() CollectorOption {
	return func(c *collector) {
		c.extractEnvs = true
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

func WithForceHasProcFS() CollectorOption {
	return func(c *collector) {
		c.hasProcfs = true
	}
}

func WithSkipOnSoftError() CollectorOption {
	return func(c *collector) {
		c.skipOnSoftError = true
	}
}

func New(opts ...CollectorOption) metadatax.Collector {
	c := &collector{}

	for _, f := range opts {
		f(c)
	}

	if c.processInfoFunc == nil {
		c.processInfoFunc = func(ctx context.Context, pid int32) (ProcessInfo, error) {
			return process.NewProcessWithContext(ctx, pid)
		}
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

	if !c.hasProcFS() {
		return md, nil
	}

	pid, found := metadatax.PIDFromContext(ctx)
	if !found {
		return nil, metadatax.PIDNotFoundError
	}

	processInfo, err := c.processInfoFunc(ctx, pid)
	if err != nil {
		if c.skipOnSoftError {
			return md, nil
		}

		return nil, errors.WrapIf(err, "could not create new process instance")
	}

	md.AddLabel("pid", strconv.Itoa(int(pid)))

	getters := []func(context.Context, ProcessInfo, metadatax.MetadataContainer){
		c.base,
		c.uids,
		c.gids,
		c.binary,
		c.network,
	}

	if c.extractEnvs {
		getters = append(getters, c.envs)
	}

	for _, f := range getters {
		f(ctx, processInfo, md)
	}

	return md, nil
}

func (c *collector) base(ctx context.Context, processInfo ProcessInfo, md metadatax.MetadataContainer) {
	if name, err := processInfo.NameWithContext(ctx); err == nil {
		md.AddLabel("name", name)
	}

	if parts, err := processInfo.CmdlineWithContext(ctx); err == nil {
		md.AddLabel("cmdline", parts)
	}
}

func (c *collector) uids(ctx context.Context, processInfo ProcessInfo, md metadatax.MetadataContainer) {
	if uids, err := processInfo.UidsWithContext(ctx); err == nil {
		if len(uids) == 4 {
			uidmd := md.Segment("uid")
			uidmd.AddLabel("", strconv.Itoa(int(uids[1])))
			uidmd.AddLabel("real", strconv.Itoa(int(uids[0])))
			uidmd.AddLabel("effective", strconv.Itoa(int(uids[1])))
		}
	}
}

func (c *collector) gids(ctx context.Context, processInfo ProcessInfo, md metadatax.MetadataContainer) {
	gidmd := md.Segment("gid")

	if gids, err := processInfo.GidsWithContext(ctx); err == nil {
		if len(gids) == 4 {
			gidmd.AddLabel("", strconv.Itoa(int(gids[1])))
			gidmd.AddLabel("real", strconv.Itoa(int(gids[0])))
			gidmd.AddLabel("effective", strconv.Itoa(int(gids[1])))
		}
	}

	if groups, err := processInfo.GroupsWithContext(ctx); err == nil {
		for _, groupID := range groups {
			gidmd.AddLabel("additional", strconv.Itoa(int(groupID)))
		}
	}
}

func (c *collector) envs(ctx context.Context, processInfo ProcessInfo, md metadatax.MetadataContainer) {
	envmd := md.Segment("env")

	if envs, err := processInfo.EnvironWithContext(ctx); err == nil {
		for _, env := range envs {
			if !strings.Contains(env, "=") {
				continue
			}
			parts := strings.SplitN(env, "=", 2)
			envmd.AddLabel(strings.ToUpper(parts[0]), parts[1])
		}
	}
}

func (c *collector) binary(ctx context.Context, processInfo ProcessInfo, md metadatax.MetadataContainer) {
	bmd := md.Segment("binary")

	if exe, err := processInfo.ExeWithContext(ctx); err == nil {
		bmd.AddLabel("path", exe)

		pid, _ := metadatax.PIDFromContext(ctx)
		file, err := os.Open(filepath.Join(procPath(), strconv.Itoa(int(pid)), "exe"))
		if errors.Is(err, os.ErrNotExist) {
			file, err = os.Open(exe)
		}
		if err != nil {
			return
		}

		hash, err := digest.SHA256.FromReader(file)
		if err != nil {
			return
		}

		bmd.AddLabel("hash", hash.String())
	}
}

func (c *collector) network(ctx context.Context, processInfo ProcessInfo, md metadatax.MetadataContainer) {
	netmd := md.Segment("network")
	if conns, err := processInfo.ConnectionsWithContext(ctx); err == nil {
		var bindings []string
		for _, conn := range conns {
			if conn.Status == "LISTEN" {
				bindings = append(bindings, conn.Laddr.IP+":"+strconv.Itoa(int(conn.Laddr.Port)))
			}
		}
		if len(bindings) > 0 {
			netmd.AddLabel("binding", bindings...)
		}
	}
}

func procPath() string {
	p := os.Getenv("HOST_PROC")
	if p != "" {
		return p
	}

	return "/proc"
}

func (c *collector) hasProcFS() bool {
	if c.hasProcfs {
		return true
	}

	v := HasProcFS()
	if v {
		c.hasProcfs = true
	}

	return v
}

func HasProcFS() bool {
	if _, err := os.Stat(basePath); err != nil {
		return false
	}

	return true
}
