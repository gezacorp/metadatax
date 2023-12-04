package static

import (
	"context"

	"github.com/gezacorp/metadatax"
)

type collector struct {
	mdContainerInitFunc func() metadatax.MetadataContainer
	mdContainer         metadatax.MetadataContainer
}

type CollectorOption func(*collector)

func CollectorWithMetadataContainerInitFunc(fn func() metadatax.MetadataContainer) CollectorOption {
	return func(c *collector) {
		c.mdContainerInitFunc = fn
	}
}

func New(labels map[string][]string, opts ...CollectorOption) metadatax.Collector {
	c := &collector{}

	for _, f := range opts {
		f(c)
	}

	if c.mdContainerInitFunc == nil {
		c.mdContainerInitFunc = func() metadatax.MetadataContainer {
			return metadatax.New()
		}
	}

	c.mdContainer = c.mdContainerInitFunc()
	c.mdContainer.AddLabels(labels)

	return c
}

func (c *collector) GetMetadata(ctx context.Context) (metadatax.MetadataContainer, error) {
	return c.mdContainer, nil
}
