package packages

import (
	"bytes"
	"context"
	"slices"
	"strings"
	"sync"
)

const maxIndexes = 16

var usualBinDirs = []string{"/bin", "/sbin", "/usr/bin", "/usr/sbin", "/usr/local/bin", "/usr/local/sbin", "/usr/games"}

// walkFunc calls yield with each path in the database at root that match
// accepts, and its package.
type walkFunc func(root string, match func(path []byte) bool, yield func(path string, pkg Package) bool)

// db looks up paths in usualBinDirs in an index per root, rebuilt when the
// stamp changes, and walks the database for other paths.
type db struct {
	walk  walkFunc
	stamp func(root string) string
	max   int

	mu      sync.Mutex
	tick    uint64
	indexes map[string]*binIndex
}

type binIndex struct {
	stamp string
	paths map[string]Package
	used  uint64
}

func newDB(walk walkFunc, stamp func(root string) string) *db {
	return &db{walk: walk, stamp: stamp, max: maxIndexes, indexes: map[string]*binIndex{}}
}

func (d *db) Stamp(root string) string {
	return d.stamp(root)
}

func (d *db) Find(_ context.Context, root, path string) (Package, bool) {
	aliases := mergedUsrAliases(path)

	if slices.ContainsFunc(aliases, inUsualBinDir) {
		paths := d.index(root)

		for _, a := range aliases {
			if pkg, ok := paths[a]; ok {
				return pkg, true
			}
		}

		return Package{}, false
	}

	var (
		found Package
		ok    bool
	)

	isAlias := func(p []byte) bool {
		return slices.ContainsFunc(aliases, func(a string) bool { return string(p) == a })
	}

	d.walk(root, isAlias, func(_ string, pkg Package) bool {
		found, ok = pkg, true

		return false
	})

	return found, ok
}

func (d *db) index(root string) map[string]Package {
	key, stamp := dbIdentity(root), d.stamp(root)

	d.mu.Lock()
	defer d.mu.Unlock()

	d.tick++

	if idx, ok := d.indexes[key]; ok && idx.stamp == stamp {
		idx.used = d.tick

		return idx.paths
	}

	paths := map[string]Package{}

	d.walk(root, inUsualBinDirBytes, func(p string, pkg Package) bool {
		paths[p] = pkg

		return true
	})

	if _, ok := d.indexes[key]; !ok && len(d.indexes) >= d.max {
		d.evict()
	}

	d.indexes[key] = &binIndex{stamp: stamp, paths: paths, used: d.tick}

	return paths
}

// evict removes the least recently used index. The caller holds mu.
func (d *db) evict() {
	var (
		oldest string
		used   uint64
	)

	for key, idx := range d.indexes {
		if oldest == "" || idx.used < used {
			oldest, used = key, idx.used
		}
	}

	delete(d.indexes, oldest)
}

func inUsualBinDir(path string) bool {
	return inUsualBinDirBytes([]byte(path))
}

func inUsualBinDirBytes(path []byte) bool {
	i := bytes.LastIndexByte(path, '/')
	if i <= 0 || i == len(path)-1 {
		return false
	}

	for _, dir := range usualBinDirs {
		if string(path[:i]) == dir {
			return true
		}
	}

	return false
}

// mergedUsrAliases returns path with its spelling on the other side of the
// /usr merge, e.g. /usr/bin/x and /bin/x.
func mergedUsrAliases(path string) []string {
	aliases := []string{path}

	for _, dir := range []string{"/bin/", "/sbin/", "/lib/", "/lib64/"} {
		if rest, ok := strings.CutPrefix(path, "/usr"+dir); ok {
			aliases = append(aliases, dir+rest)
		} else if rest, ok := strings.CutPrefix(path, dir); ok {
			aliases = append(aliases, "/usr"+dir+rest)
		}
	}

	return aliases
}
