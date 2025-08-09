package node

import (
	"context"

	"github.com/shirou/gopsutil/v3/host"

	"github.com/gezacorp/metadatax"
)

const (
	name = "node"
)

type collector struct {
	getHostMetadataFunc GetHostMetadataFunc

	mdContainerInitFunc func() metadatax.MetadataContainer
}

type CollectorOption func(*collector)

type GetHostMetadataFunc func(context.Context) (*host.InfoStat, error)

func CollectorWithGetHostMetadataFunc(fn GetHostMetadataFunc) CollectorOption {
	return func(c *collector) {
		c.getHostMetadataFunc = fn
	}
}

func CollectorWithMetadataContainerInitFunc(fn func() metadatax.MetadataContainer) CollectorOption {
	return func(c *collector) {
		c.mdContainerInitFunc = fn
	}
}

func New(opts ...CollectorOption) metadatax.Collector {
	c := &collector{}

	for _, f := range opts {
		f(c)
	}

	if c.getHostMetadataFunc == nil {
		c.getHostMetadataFunc = host.InfoWithContext
	}

	if c.mdContainerInitFunc == nil {
		c.mdContainerInitFunc = func() metadatax.MetadataContainer {
			return metadatax.New(metadatax.WithPrefix(name))
		}
	}

	return c
}

func (c *collector) GetMetadata(ctx context.Context) (metadatax.MetadataContainer, error) {
	info, err := c.getHostMetadataFunc(ctx)
	if err != nil {
		return nil, err
	}

	md := c.mdContainerInitFunc()
	md.AddLabel("hostname", info.Hostname)
	md.AddLabel("uuid", info.HostID)

	os := md.Segment("os")
	os.AddLabel("type", info.OS)
	if info.Platform != "" && info.PlatformVersion != "" {
		os.AddLabel("version", info.Platform+"-"+info.PlatformVersion)
	}

	platform := md.Segment("platform")
	platform.AddLabel("name", info.Platform)
	platform.AddLabel("family", info.PlatformFamily)
	platform.AddLabel("version", info.PlatformVersion)

	virt := md.Segment("virtualization")
	virt.AddLabel("type", info.VirtualizationSystem)
	virt.AddLabel("role", info.VirtualizationRole)

	kernel := md.Segment("kernel")
	kernel.AddLabel("version", info.KernelVersion)
	kernel.AddLabel("arch", info.KernelArch)

	return md, nil
}
