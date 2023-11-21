package linuxos

import (
	"context"
	"strings"

	"github.com/signalfx/golib/metadata/hostmetadata"

	"github.com/gezacorp/metadatax"
)

const (
	name = "linuxos"
)

type collector struct {
	hostMetadataGetter func() (*hostmetadata.OS, error)
	mdcontainer        metadatax.MetadataContainer
}

type CollectorOption func(*collector)

func CollectorWithHostMetadataGetter(getter func() (*hostmetadata.OS, error)) CollectorOption {
	return func(c *collector) {
		c.hostMetadataGetter = getter
	}
}

func CollectorWithMetadataContainer(mdcontainer metadatax.MetadataContainer) CollectorOption {
	return func(c *collector) {
		c.mdcontainer = mdcontainer
	}
}

func New(opts ...CollectorOption) metadatax.Collector {
	c := &collector{}

	for _, f := range opts {
		f(c)
	}

	if c.hostMetadataGetter == nil {
		c.hostMetadataGetter = hostmetadata.GetOS
	}

	if c.mdcontainer == nil {
		c.mdcontainer = metadatax.New(metadatax.WithPrefix(name))
	}

	return c
}

func (c *collector) GetMetadata(ctx context.Context) (metadatax.MetadataContainer, error) {
	info, err := c.hostMetadataGetter()
	if err != nil {
		return nil, err
	}

	md := c.mdcontainer
	md.AddLabel("name", info.HostOSName)
	md.AddLabel("version", info.HostLinuxVersion)

	kernel := md.Level("kernel")
	kernel.AddLabel("release", strings.Trim(strings.Join([]string{info.HostKernelName, info.HostKernelRelease}, "-"), "-"))
	kernel.AddLabel("version", info.HostKernelVersion)

	return md, nil
}
