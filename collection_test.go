package metadatax

import (
	"context"
	"testing"

	"emperror.dev/errors"
	"github.com/stretchr/testify/assert"
)

type fakeCollector struct {
	meta MetadataContainer
	err  error
}

func (f *fakeCollector) GetMetadata(ctx context.Context) (MetadataContainer, error) {
	return f.meta, f.err
}

func TestCollectorCollectionDropsMetadataOnError(t *testing.T) {
	partial := New(WithPrefix("partial"))
	partial.AddLabel("vmid", "100")

	c := NewCollectorCollection()
	c.Add(&fakeCollector{meta: partial, err: errors.New("inspection failed")})

	md, err := c.GetMetadata(context.Background())
	assert.Error(t, err)
	assert.Empty(t, md.GetLabels(), "metadata is discarded whenever a collector returns an error, partial or not")
}

func TestCollectorCollectionSkipsNilMetadataOnError(t *testing.T) {
	c := NewCollectorCollection()
	c.Add(&fakeCollector{meta: nil, err: errors.New("total failure")})

	md, err := c.GetMetadata(context.Background())
	assert.Error(t, err)
	assert.Empty(t, md.GetLabels())
}

func TestCollectorCollectionMergesSuccessfulMetadata(t *testing.T) {
	ok := New(WithPrefix("ok"))
	ok.AddLabel("name", "test")

	c := NewCollectorCollection()
	c.Add(&fakeCollector{meta: ok, err: nil})

	md, err := c.GetMetadata(context.Background())
	assert.NoError(t, err)
	assert.Equal(t, []string{"test"}, md.GetLabels()["ok:name"])
}
