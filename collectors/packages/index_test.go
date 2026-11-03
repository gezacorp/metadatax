package packages

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDB_KeepsABoundedNumberOfIndexes(t *testing.T) {
	t.Parallel()

	d := NewDpkg().(*db)
	d.max = 2

	roots := make([]string, 3)
	for i := range roots {
		roots[i] = t.TempDir()
		list := filepath.Join(roots[i], "/var/lib/dpkg/info/curl.list")
		require.NoError(t, os.MkdirAll(filepath.Dir(list), 0o755))
		require.NoError(t, os.WriteFile(list, []byte("/usr/bin/curl\n"), 0o644))
		require.NoError(t, os.WriteFile(filepath.Join(roots[i], "/var/lib/dpkg/status"), []byte("Package: curl\nVersion: 1\n"), 0o644))

		_, ok := d.Find(context.Background(), roots[i], "/usr/bin/curl")
		require.True(t, ok)
	}

	assert.Len(t, d.indexes, 2)
	assert.NotContains(t, d.indexes, dbIdentity(roots[0]), "the least recently used goes")
}
