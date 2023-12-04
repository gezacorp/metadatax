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
	getHostMetadataFunc GetHostMetadataFunc

	mdContainerInitFunc func() metadatax.MetadataContainer
}

type CollectorOption func(*collector)

type GetHostMetadataFunc func() (*hostmetadata.OS, error)

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
		c.getHostMetadataFunc = hostmetadata.GetOS
	}

	if c.mdContainerInitFunc == nil {
		c.mdContainerInitFunc = func() metadatax.MetadataContainer {
			return metadatax.New(metadatax.WithPrefix(name))
		}
	}

	return c
}

func (c *collector) GetMetadata(ctx context.Context) (metadatax.MetadataContainer, error) {
	metadata, err := c.getHostMetadataFunc()
	if err != nil {
		return nil, err
	}

	md := c.mdContainerInitFunc()
	md.AddLabel("name", metadata.HostOSName)
	md.AddLabel("version", metadata.HostLinuxVersion)

	kernel := md.Segment("kernel")
	kernel.AddLabel("release", strings.Trim(strings.Join([]string{metadata.HostKernelName, metadata.HostKernelRelease}, "-"), "-"))
	kernel.AddLabel("version", metadata.HostKernelVersion)

	return md, nil
}
