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

	mdcontainerGetter func() metadatax.MetadataContainer
}

type CollectorOption func(*collector)

func CollectorWithHostMetadataGetter(getter func() (*hostmetadata.OS, error)) CollectorOption {
	return func(c *collector) {
		c.hostMetadataGetter = getter
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

	if c.hostMetadataGetter == nil {
		c.hostMetadataGetter = hostmetadata.GetOS
	}

	if c.mdcontainerGetter == nil {
		c.mdcontainerGetter = func() metadatax.MetadataContainer {
			return metadatax.New(metadatax.WithPrefix(name))
		}
	}

	return c
}

func (c *collector) GetMetadata(ctx context.Context) (metadatax.MetadataContainer, error) {
	info, err := c.hostMetadataGetter()
	if err != nil {
		return nil, err
	}

	md := c.mdcontainerGetter()
	md.AddLabel("name", info.HostOSName)
	md.AddLabel("version", info.HostLinuxVersion)

	kernel := md.Segment("kernel")
	kernel.AddLabel("release", strings.Trim(strings.Join([]string{info.HostKernelName, info.HostKernelRelease}, "-"), "-"))
	kernel.AddLabel("version", info.HostKernelVersion)

	return md, nil
}
