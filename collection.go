package metadatax

import (
	"context"
	"sync"

	"emperror.dev/errors"
	"github.com/google/uuid"
)

type CollectorCollection interface {
	Collector

	Add(Collector)
	List() Collectors
	Clear()
}

type collectorCollection struct {
	collectors sync.Map
}

func NewCollectorCollection() CollectorCollection {
	return &collectorCollection{
		collectors: sync.Map{},
	}
}

func (c *collectorCollection) Add(collector Collector) {
	c.collectors.Store(uuid.NewString(), collector)
}

func (c *collectorCollection) List() Collectors {
	collectors := make(Collectors, 0)

	c.collectors.Range(func(k, v any) bool {
		if collector, ok := v.(Collector); ok {
			collectors = append(collectors, collector)
		}
		return true
	})

	return collectors
}

func (c *collectorCollection) Clear() {
	c.collectors = sync.Map{}
}

func (c *collectorCollection) GetMetadata(ctx context.Context) (MetadataContainer, error) {
	md := New()

	var multiErr error
	for _, collector := range c.List() {
		meta, err := collector.GetMetadata(ctx)
		if err != nil {
			multiErr = errors.Combine(multiErr, err)
			continue
		}

		md.AddLabels(meta.GetLabels())
	}

	return md, multiErr
}
