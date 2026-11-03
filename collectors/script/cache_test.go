package script

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPackageCache_IsBounded(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	c := newPackageCache(2)

	for i := range 3 {
		file := filepath.Join("/srv", strconv.Itoa(i), "server.js")
		require.NoError(t, os.MkdirAll(filepath.Join(root, filepath.Dir(file)), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(root, filepath.Dir(file), "package.json"), []byte(`{"name": "app`+strconv.Itoa(i)+`"}`), 0o644))

		_, ok := c.find(root, "node", file, "node")
		require.True(t, ok)
	}

	assert.Len(t, c.entries, 2)
}
