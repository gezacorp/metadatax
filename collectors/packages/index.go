package packages

import (
	"bytes"
	"context"
	"slices"
	"strings"
	"sync"
)

// maxIndexes bounds how many databases' indexes are kept: the host's and
// those of the containers seen most recently.
const maxIndexes = 16

// usualBinDirs are where packages install the programs people run.
var usualBinDirs = []string{"/bin", "/sbin", "/usr/bin", "/usr/sbin", "/usr/local/bin", "/usr/local/sbin", "/usr/games"}

// walkFunc yields the paths a package database lists that match, with their
// package, until yield returns false. Matching sees the raw line, so that
// the many paths that do not match cost no string.
type walkFunc func(root string, match func(path []byte) bool, yield func(path string, pkg Package) bool)

// db finds packages in a database read through walk. The paths in the
// usual program directories are kept in memory per database, until its
// stamp changes; a path elsewhere is looked up by walking the database.
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

// index is the database's programs in the usual directories, read anew
// when its stamp changed since.
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

// evict drops the index used longest ago; call with mu held.
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

// inUsualBinDirBytes reports whether path is a file right in one of the
// usual program directories, without making a string of it.
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

// mergedUsrAliases is path and its other name on a system where /bin, /sbin
// and /lib are links into /usr: packages built before the merge list the
// short one.
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
