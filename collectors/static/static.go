package static

import (
	"context"

	"github.com/gezacorp/metadatax"
)

type collector struct {
	mdcontainer metadatax.MetadataContainer
}

type CollectorOption func(*collector)

func CollectorWithMetadataContainer(mdcontainer metadatax.MetadataContainer) CollectorOption {
	return func(c *collector) {
		c.mdcontainer = mdcontainer
	}
}

func New(labels map[string][]string, opts ...CollectorOption) metadatax.Collector {
	c := &collector{}

	for _, f := range opts {
		f(c)
	}

	if c.mdcontainer == nil {
		c.mdcontainer = metadatax.New()
	}

	c.mdcontainer.AddLabels(labels)

	return c
}

func (c *collector) GetMetadata(ctx context.Context) (metadatax.MetadataContainer, error) {
	return c.mdcontainer, nil
}
