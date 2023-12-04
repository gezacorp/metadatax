package procfs

import (
	"context"
	_ "crypto/sha256"
	"os"
	"strconv"
	"strings"

	"emperror.dev/errors"
	"github.com/opencontainers/go-digest"
	"github.com/shirou/gopsutil/v3/process"

	"github.com/gezacorp/metadatax"
)

const (
	name = "process"
)

type collector struct {
	extractEnvs bool

	rawDataGetterFunc RawDataGetterFunc
	mdcontainerGetter func() metadatax.MetadataContainer
}

type RawDataGetterFunc func(ctx context.Context, pid int32) (RawData, error)

type RawData interface {
	NameWithContext(ctx context.Context) (string, error)
	CmdlineWithContext(ctx context.Context) (string, error)
	UidsWithContext(ctx context.Context) ([]int32, error)
	GidsWithContext(ctx context.Context) ([]int32, error)
	GroupsWithContext(ctx context.Context) ([]int32, error)
	EnvironWithContext(ctx context.Context) ([]string, error)
	ExeWithContext(ctx context.Context) (string, error)
}

type CollectorOption func(*collector)

func CollectorWithExtractENVs() CollectorOption {
	return func(c *collector) {
		c.extractEnvs = true
	}
}

func CollectorWithRawDataGetterFunc(fn RawDataGetterFunc) CollectorOption {
	return func(c *collector) {
		c.rawDataGetterFunc = fn
	}
}

func CollectorWithMetadataContainerGetter(getter func() metadatax.MetadataContainer) CollectorOption {
	return func(c *collector) {
		c.mdcontainerGetter = getter
	}
}

func New(opts ...CollectorOption) metadatax.Collector {
	c := &collector{}

	for _, f := range opts {
		f(c)
	}

	if c.rawDataGetterFunc == nil {
		c.rawDataGetterFunc = func(ctx context.Context, pid int32) (RawData, error) {
			return process.NewProcessWithContext(ctx, pid)
		}
	}

	if c.mdcontainerGetter == nil {
		c.mdcontainerGetter = func() metadatax.MetadataContainer {
			return metadatax.New(metadatax.WithPrefix(name))
		}
	}

	return c
}

func (c *collector) GetMetadata(ctx context.Context) (metadatax.MetadataContainer, error) {
	md := c.mdcontainerGetter()

	pid, found := metadatax.PIDFromContext(ctx)
	if !found {
		return nil, metadatax.PIDNotFoundError
	}

	data, err := c.rawDataGetterFunc(ctx, pid)
	if err != nil {
		return nil, errors.WrapIf(err, "could not create new process instance")
	}

	md.AddLabel("pid", strconv.Itoa(int(pid)))

	getters := []func(context.Context, RawData, metadatax.MetadataContainer){
		c.base,
		c.uids,
		c.gids,
		c.binary,
	}

	if c.extractEnvs {
		getters = append(getters, c.envs)
	}

	for _, f := range getters {
		f(ctx, data, md)
	}

	return md, nil
}

func (c *collector) base(ctx context.Context, data RawData, md metadatax.MetadataContainer) {
	if name, err := data.NameWithContext(ctx); err == nil {
		md.AddLabel("name", name)
	}

	if parts, err := data.CmdlineWithContext(ctx); err == nil {
		md.AddLabel("cmdline", parts)
	}
}

func (c *collector) uids(ctx context.Context, data RawData, md metadatax.MetadataContainer) {
	if uids, err := data.UidsWithContext(ctx); err == nil {
		if len(uids) == 4 {
			uidmd := md.Segment("uid")
			uidmd.AddLabel("", strconv.Itoa(int(uids[1])))
			uidmd.AddLabel("real", strconv.Itoa(int(uids[0])))
			uidmd.AddLabel("effective", strconv.Itoa(int(uids[1])))
		}
	}
}

func (c *collector) gids(ctx context.Context, data RawData, md metadatax.MetadataContainer) {
	gidmd := md.Segment("gid")

	if gids, err := data.GidsWithContext(ctx); err == nil {
		if len(gids) == 4 {
			gidmd.AddLabel("", strconv.Itoa(int(gids[1])))
			gidmd.AddLabel("real", strconv.Itoa(int(gids[0])))
			gidmd.AddLabel("effective", strconv.Itoa(int(gids[1])))
		}
	}

	if groups, err := data.GroupsWithContext(ctx); err == nil {
		for _, groupID := range groups {
			gidmd.AddLabel("additional", strconv.Itoa(int(groupID)))
		}
	}
}

func (c *collector) envs(ctx context.Context, data RawData, md metadatax.MetadataContainer) {
	envmd := md.Segment("env")

	if envs, err := data.EnvironWithContext(ctx); err == nil {
		for _, env := range envs {
			if !strings.Contains(env, "=") {
				continue
			}
			parts := strings.SplitN(env, "=", 2)
			envmd.AddLabel(strings.ToUpper(parts[0]), parts[1])
		}
	}
}

func (c *collector) binary(ctx context.Context, data RawData, md metadatax.MetadataContainer) {
	bmd := md.Segment("binary")

	if exe, err := data.ExeWithContext(ctx); err == nil {
		bmd.AddLabel("path", exe)

		file, err := os.Open(exe)
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
