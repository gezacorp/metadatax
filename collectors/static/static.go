package static

import (
	"context"

	"github.com/gezacorp/metadatax"
)

type collector struct {
	mdcontainerGetter func() metadatax.MetadataContainer
	mdcontainer       metadatax.MetadataContainer
}

type CollectorOption func(*collector)

func CollectorWithMetadataContainerGetter(getter func() metadatax.MetadataContainer) CollectorOption {
	return func(c *collector) {
		c.mdcontainerGetter = getter
	}
}

func New(labels map[string][]string, opts ...CollectorOption) metadatax.Collector {
	c := &collector{}

	for _, f := range opts {
		f(c)
	}

	if c.mdcontainerGetter == nil {
		c.mdcontainerGetter = func() metadatax.MetadataContainer {
			return metadatax.New()
		}
	}

	c.mdcontainer = c.mdcontainerGetter()
	c.mdcontainer.AddLabels(labels)

	return c
}

func (c *collector) GetMetadata(ctx context.Context) (metadatax.MetadataContainer, error) {
	return c.mdcontainer, nil
}
