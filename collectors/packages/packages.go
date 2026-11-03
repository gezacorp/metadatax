package packages

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"

	"emperror.dev/errors"
	"github.com/shirou/gopsutil/v4/process"

	"github.com/gezacorp/metadatax"
)

const (
	name = "package"

	maxCachedLookups = 1024
)

type Package struct {
	Manager string
	Name    string
	Version string
	Arch    string
}

type ProcessInfo interface {
	ExeWithContext(ctx context.Context) (string, error)
}

type ProcessInfoFunc func(ctx context.Context, pid int32) (ProcessInfo, error)

type Finder interface {
	Find(ctx context.Context, root, path string) (Package, bool)
}

// stamper is implemented by a Finder whose Stamp changes with every install,
// upgrade or removal. The collector's cached answers expire when it changes.
type stamper interface {
	Stamp(root string) string
}

type collector struct {
	processInfoFunc     ProcessInfoFunc
	rootFunc            func(pid int32) string
	finders             []Finder
	mdContainerInitFunc func() metadatax.MetadataContainer
	skipOnSoftError     bool

	mu    sync.Mutex
	cache map[cacheKey]lookup
}

type cacheKey struct {
	db    string
	stamp string
	path  string
}

type lookup struct {
	pkg   Package
	found bool
}

type CollectorOption func(*collector)

func CollectorWithProcessInfoFunc(fn ProcessInfoFunc) CollectorOption {
	return func(c *collector) {
		c.processInfoFunc = fn
	}
}

func CollectorWithRootFunc(fn func(pid int32) string) CollectorOption {
	return func(c *collector) {
		c.rootFunc = fn
	}
}

func CollectorWithFinders(finders ...Finder) CollectorOption {
	return func(c *collector) {
		c.finders = finders
	}
}

func CollectorWithMetadataContainerInitFunc(fn func() metadatax.MetadataContainer) CollectorOption {
	return func(c *collector) {
		c.mdContainerInitFunc = fn
	}
}

func WithSkipOnSoftError() CollectorOption {
	return func(c *collector) {
		c.skipOnSoftError = true
	}
}

func New(opts ...CollectorOption) metadatax.Collector {
	c := &collector{
		processInfoFunc: func(ctx context.Context, pid int32) (ProcessInfo, error) {
			return process.NewProcessWithContext(ctx, pid)
		},
		rootFunc: processRoot,
		finders: []Finder{
			Homebrew{},
			Snap{},
			NewDpkg(),
			NewApk(),
			NewPacman(),
			NewRPM(nil),
		},
		cache: map[cacheKey]lookup{},
	}

	for _, f := range opts {
		f(c)
	}

	if c.mdContainerInitFunc == nil {
		c.mdContainerInitFunc = func() metadatax.MetadataContainer {
			return metadatax.New(metadatax.WithPrefix(name))
		}
	}

	return c
}

func (c *collector) GetMetadata(ctx context.Context) (metadatax.MetadataContainer, error) {
	md := c.mdContainerInitFunc()

	pid, found := metadatax.PIDFromContext(ctx)
	if !found {
		return nil, metadatax.ErrPIDNotFound
	}

	p, err := c.processInfoFunc(ctx, pid)
	if err != nil {
		if c.skipOnSoftError {
			return md, nil
		}

		return nil, errors.WrapIf(err, "could not create new process instance")
	}

	exe, err := p.ExeWithContext(ctx)
	if err != nil || exe == "" {
		return md, nil
	}

	pkg, ok := c.find(ctx, c.rootFunc(pid), exe)
	if !ok {
		return md, nil
	}

	md.AddLabel("manager", pkg.Manager)
	md.AddLabel("name", pkg.Name)

	if pkg.Version != "" {
		md.AddLabel("version", pkg.Version)
	}

	if pkg.Arch != "" {
		md.AddLabel("arch", pkg.Arch)
	}

	return md, nil
}

func (c *collector) find(ctx context.Context, root, path string) (Package, bool) {
	key := cacheKey{db: dbIdentity(root), stamp: c.stamp(root), path: path}

	c.mu.Lock()
	l, cached := c.cache[key]
	c.mu.Unlock()

	if cached {
		return l.pkg, l.found
	}

	for _, f := range c.finders {
		if l.pkg, l.found = f.Find(ctx, root, path); l.found {
			break
		}
	}

	c.mu.Lock()

	if len(c.cache) >= maxCachedLookups {
		clear(c.cache)
	}

	c.cache[key] = l
	c.mu.Unlock()

	return l.pkg, l.found
}

func (c *collector) stamp(root string) string {
	var b strings.Builder

	for _, f := range c.finders {
		if s, ok := f.(stamper); ok {
			b.WriteString(s.Stamp(root))
			b.WriteByte('/')
		}
	}

	return b.String()
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

// processRoot returns the directory through which pid's own root filesystem
// is read, or "" outside Linux.
func processRoot(pid int32) string {
	if runtime.GOOS != "linux" {
		return ""
	}

	proc := os.Getenv("HOST_PROC")
	if proc == "" {
		proc = "/proc"
	}

	return filepath.Join(proc, strconv.Itoa(int(pid)), "root")
}
