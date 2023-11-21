package static

import (
	"context"

	"github.com/gezacorp/metadatax"
)

type collector struct {
	md metadatax.MetadataContainer
}

func New(labels map[string][]string) metadatax.Collector {
	md := metadatax.New()
	md.AddLabels(labels)

	return &collector{
		md: md,
	}
}

func (c *collector) GetMetadata(ctx context.Context) (metadatax.MetadataContainer, error) {
	return c.md, nil
}
