package script

import (
	"os"
	"path/filepath"
	"strconv"
	"sync"
)

const maxCachedPackages = 1024

// deps maps the files and directories an answer was read from, relative to
// the process's root, to their modification times.
type deps map[string]int64

func (d deps) add(root, path string) {
	if _, ok := d[path]; !ok {
		d[path] = modTime(filepath.Join(root, path))
	}
}

func (d deps) unchanged(root string) bool {
	for path, t := range d {
		if modTime(filepath.Join(root, path)) != t {
			return false
		}
	}

	return true
}

func modTime(path string) int64 {
	fi, err := os.Stat(path)
	if err != nil {
		return -1
	}

	return fi.ModTime().UnixNano()
}

type packageCache struct {
	max int

	mu      sync.Mutex
	tick    uint64
	entries map[packageKey]*packageEntry
}

type packageKey struct {
	db          string
	interpreter string
	file        string
	argv0       string
}

type packageEntry struct {
	pkg   Package
	found bool
	deps  deps
	used  uint64
}

func newPackageCache(max int) *packageCache {
	return &packageCache{max: max, entries: map[packageKey]*packageEntry{}}
}

func (c *packageCache) find(root, interpreter, file, argv0 string) (Package, bool) {
	key := packageKey{db: dbIdentity(root), interpreter: interpreter, file: file, argv0: argv0}

	c.mu.Lock()
	e, ok := c.entries[key]
	c.mu.Unlock()

	if ok && e.deps.unchanged(root) {
		c.mu.Lock()
		c.tick++
		e.used = c.tick
		c.mu.Unlock()

		return e.pkg, e.found
	}

	d := deps{}
	pkg, found := findPackage(root, interpreter, file, argv0, d)

	c.mu.Lock()
	defer c.mu.Unlock()

	if _, ok := c.entries[key]; !ok && len(c.entries) >= c.max {
		c.evict()
	}

	c.tick++
	c.entries[key] = &packageEntry{pkg: pkg, found: found, deps: d, used: c.tick}

	return pkg, found
}

// evict removes the least recently used entry. The caller holds mu.
func (c *packageCache) evict() {
	var (
		oldest packageKey
		used   uint64
		seen   bool
	)

	for key, e := range c.entries {
		if !seen || e.used < used {
			oldest, used, seen = key, e.used, true
		}
	}

	delete(c.entries, oldest)
}

// dbIdentity returns the device and inode of root, one value for all PIDs of
// a container, or root itself when they cannot be read.
func dbIdentity(root string) string {
	if root == "" {
		return ""
	}

	dev, ino, ok := fileID(root)
	if !ok {
		return root
	}

	return strconv.FormatUint(dev, 10) + ":" + strconv.FormatUint(ino, 10)
}
