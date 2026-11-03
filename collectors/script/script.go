// Package script collects what an interpreter runs: the script, module or
// jar, and the package that owns it, so that processes of one interpreter
// running different programs can be told apart.
package script

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strconv"

	"emperror.dev/errors"
	"github.com/shirou/gopsutil/v4/process"

	"github.com/gezacorp/metadatax"
)

const name = "script"

type ProcessInfo interface {
	ExeWithContext(ctx context.Context) (string, error)
	CmdlineSliceWithContext(ctx context.Context) ([]string, error)
	CwdWithContext(ctx context.Context) (string, error)
}

type ProcessInfoFunc func(ctx context.Context, pid int32) (ProcessInfo, error)

type collector struct {
	processInfoFunc     ProcessInfoFunc
	rootFunc            func(pid int32) string
	mdContainerInitFunc func() metadatax.MetadataContainer
	skipOnSoftError     bool
	packages            *packageCache
}

type CollectorOption func(*collector)

func CollectorWithProcessInfoFunc(fn ProcessInfoFunc) CollectorOption {
	return func(c *collector) {
		c.processInfoFunc = fn
	}
}

// CollectorWithRootFunc sets where a process's files are seen from, its
// root directory as this process can open it.
func CollectorWithRootFunc(fn func(pid int32) string) CollectorOption {
	return func(c *collector) {
		c.rootFunc = fn
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
		packages: newPackageCache(maxCachedPackages),
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

	args, err := p.CmdlineSliceWithContext(ctx)
	if err != nil || len(args) == 0 {
		return md, nil
	}

	exe, _ := p.ExeWithContext(ctx)

	target, ok := Parse(exe, args)
	if !ok {
		return md, nil
	}

	md.AddLabel("interpreter", target.Interpreter)

	if target.Inline {
		md.AddLabel("inline", "true")

		return md, nil
	}

	addLabel(md, "module", target.Module)
	addLabel(md, "class", target.Class)
	addLabel(md, "task", target.Task)

	// A task is the project's, so its package is found from the working
	// directory, as the interpreter finds the task.
	file := target.Path

	switch {
	case target.Jar != "":
		file = target.Jar
	case target.Task != "":
		file = "package.json"
	case file == "":
		return md, nil
	}

	if !filepath.IsAbs(file) {
		cwd, err := p.CwdWithContext(ctx)
		if err != nil {
			return md, nil
		}

		file = filepath.Join(cwd, file)
	}

	root := c.rootFunc(pid)
	file = resolveIn(root, file)

	switch {
	case target.Jar != "":
		md.AddLabel("jar", file)
	case target.Task == "":
		md.AddLabel("path", file)
	}

	if pkg, ok := c.packages.find(root, target.Interpreter, file, args[0]); ok {
		pmd := md.Segment("package")
		pmd.AddLabel("manager", pkg.Manager)
		pmd.AddLabel("name", pkg.Name)
		addLabel(pmd, "version", pkg.Version)
		pmd.AddLabel("path", pkg.Path)
	}

	return md, nil
}

func addLabel(md metadatax.MetadataContainer, name, value string) {
	if value != "" {
		md.AddLabel(name, value)
	}
}

// processRoot is a process's root directory on Linux, so that files of a
// process in another mount namespace, such as a container, are read from
// its view; elsewhere the host's.
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
