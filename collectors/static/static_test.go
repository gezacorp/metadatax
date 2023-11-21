package static_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/gezacorp/metadatax/collectors/static"
)

func TestGetMetadata(t *testing.T) {
	t.Parallel()

	data := map[string][]string{
		"key": {"value"},
	}

	collector := static.New(data)
	md, err := collector.GetMetadata(context.Background())
	assert.Nil(t, err)
	assert.Equal(t, data, map[string][]string(md.GetLabels()))
}
