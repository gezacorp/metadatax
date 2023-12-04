package static_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/gezacorp/metadatax"
	"github.com/gezacorp/metadatax/collectors/static"
)

func TestGetMetadata(t *testing.T) {
	t.Parallel()

	data := map[string][]string{
		"key": {"value"},
	}

	expected := map[string][]string{
		"test:key": {"value"},
	}

	collector := static.New(data, static.CollectorWithMetadataContainerInitFunc(
		func() metadatax.MetadataContainer {
			return metadatax.New(metadatax.WithPrefix("test"))
		},
	))
	md, err := collector.GetMetadata(context.Background())
	assert.Nil(t, err)
	assert.Equal(t, expected, map[string][]string(md.GetLabels()))
}
