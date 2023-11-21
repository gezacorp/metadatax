package metadatax

import "context"

type Collector interface {
	GetMetadata(ctx context.Context) (MetadataContainer, error)
}

type Collectors []Collector
