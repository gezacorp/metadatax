package packages

import (
	"bufio"
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var (
	cellar   = regexp.MustCompile(`/Cellar/([^/]+)/([^/]+)/`)
	snapPath = regexp.MustCompile(`^/snap/([^/]+)/([^/]+)/`)
)

type Homebrew struct{}

func (Homebrew) Find(_ context.Context, _, path string) (Package, bool) {
	m := cellar.FindStringSubmatch(path)
	if m == nil {
		return Package{}, false
	}

	return Package{Manager: "brew", Name: m[1], Version: m[2]}, true
}

type Snap struct{}

func (Snap) Find(_ context.Context, root, path string) (Package, bool) {
	m := snapPath.FindStringSubmatch(path)
	if m == nil {
		return Package{}, false
	}

	pkg := Package{Manager: "snap", Name: m[1]}

	f, err := os.Open(filepath.Join(root, "/snap", m[1], m[2], "meta/snap.yaml"))
	if err != nil {
		return pkg, true
	}
	defer f.Close()

	s := bufio.NewScanner(f)
	for s.Scan() {
		if v, ok := strings.CutPrefix(s.Text(), "version:"); ok {
			pkg.Version = strings.Trim(strings.TrimSpace(v), `'"`)

			break
		}
	}

	return pkg, true
}

func modTimes(root string, paths ...string) string {
	var newest int64

	for _, p := range paths {
		if fi, err := os.Stat(filepath.Join(root, p)); err == nil {
			newest = max(newest, fi.ModTime().UnixNano())
		}
	}

	if newest == 0 {
		return ""
	}

	return strconv.FormatInt(newest, 10)
}

func NewDpkg() Finder {
	return newDB(dpkgWalk, func(root string) string {
		return modTimes(root, "/var/lib/dpkg/status")
	})
}

func dpkgWalk(root string, match func([]byte) bool, yield func(path string, pkg Package) bool) {
	lists, err := filepath.Glob(filepath.Join(root, "/var/lib/dpkg/info/*.list"))
	if err != nil || len(lists) == 0 {
		return
	}

	var versions map[string]string

	for _, list := range lists {
		b, err := os.ReadFile(list)
		if err != nil {
			continue
		}

		for line := range bytes.Lines(b) {
			line = bytes.TrimSuffix(line, []byte("\n"))
			if !match(line) {
				continue
			}

			if versions == nil {
				versions = dpkgVersions(filepath.Join(root, "/var/lib/dpkg/status"))
			}

			pkg := Package{Manager: "dpkg"}
			pkg.Name, pkg.Arch, _ = strings.Cut(strings.TrimSuffix(filepath.Base(list), ".list"), ":")
			pkg.Version = versions[pkg.Name+":"+pkg.Arch]

			if !yield(string(line), pkg) {
				return
			}
		}
	}
}

// dpkgVersions returns the versions in a dpkg status file by <name>:<arch>,
// and by <name>: for the first architecture of each name.
func dpkgVersions(status string) map[string]string {
	versions := map[string]string{}

	f, err := os.Open(status)
	if err != nil {
		return versions
	}
	defer f.Close()

	var name, arch, version string

	add := func() {
		if name == "" {
			return
		}

		versions[name+":"+arch] = version

		if _, ok := versions[name+":"]; !ok {
			versions[name+":"] = version
		}
	}

	s := bufio.NewScanner(f)
	s.Buffer(make([]byte, 64*1024), 1024*1024)

	for s.Scan() {
		line := s.Text()

		switch {
		case line == "":
			add()
			name, arch, version = "", "", ""
		case strings.HasPrefix(line, "Package: "):
			name = strings.TrimPrefix(line, "Package: ")
		case strings.HasPrefix(line, "Architecture: "):
			arch = strings.TrimPrefix(line, "Architecture: ")
		case strings.HasPrefix(line, "Version: "):
			version = strings.TrimPrefix(line, "Version: ")
		}
	}

	add()

	return versions
}

func NewApk() Finder {
	return newDB(apkWalk, func(root string) string {
		return modTimes(root, "/lib/apk/db/installed")
	})
}

func apkWalk(root string, match func([]byte) bool, yield func(path string, pkg Package) bool) {
	f, err := os.Open(filepath.Join(root, "/lib/apk/db/installed"))
	if err != nil {
		return
	}
	defer f.Close()

	pkg := Package{Manager: "apk"}

	var (
		dir  string
		path []byte
	)

	s := bufio.NewScanner(f)
	s.Buffer(make([]byte, 64*1024), 1024*1024)

	for s.Scan() {
		line := s.Bytes()
		if len(line) < 2 || line[1] != ':' {
			pkg, dir = Package{Manager: "apk"}, ""

			continue
		}

		value := line[2:]

		switch line[0] {
		case 'P':
			pkg.Name = string(value)
		case 'V':
			pkg.Version = string(value)
		case 'A':
			pkg.Arch = string(value)
		case 'F':
			dir = string(value)
		case 'R':
			path = append(append(append(path[:0], '/'), dir...), '/')
			if dir == "" {
				path = path[:1]
			}

			path = append(path, value...)

			if pkg.Name != "" && match(path) && !yield(string(path), pkg) {
				return
			}
		}
	}
}

func NewPacman() Finder {
	return newDB(pacmanWalk, func(root string) string {
		return modTimes(root, "/var/lib/pacman/local")
	})
}

func pacmanWalk(root string, match func([]byte) bool, yield func(path string, pkg Package) bool) {
	lists, err := filepath.Glob(filepath.Join(root, "/var/lib/pacman/local/*/files"))
	if err != nil {
		return
	}

	var path []byte

	for _, list := range lists {
		b, err := os.ReadFile(list)
		if err != nil {
			continue
		}

		var (
			pkg   *Package
			files bool
		)

		for line := range bytes.Lines(b) {
			line = bytes.TrimSuffix(line, []byte("\n"))

			if bytes.HasPrefix(line, []byte("%")) {
				files = string(line) == "%FILES%"

				continue
			}

			if !files || len(line) == 0 {
				continue
			}

			path = append(append(path[:0], '/'), line...)
			if !match(path) {
				continue
			}

			if pkg == nil {
				desc := pacmanDesc(filepath.Join(filepath.Dir(list), "desc"))
				pkg = &Package{Manager: "pacman", Name: desc["NAME"], Version: desc["VERSION"], Arch: desc["ARCH"]}
			}

			if pkg.Name != "" && !yield(string(path), *pkg) {
				return
			}
		}
	}
}

func pacmanDesc(path string) map[string]string {
	desc := map[string]string{}

	f, err := os.Open(path)
	if err != nil {
		return desc
	}
	defer f.Close()

	var key string

	s := bufio.NewScanner(f)
	for s.Scan() {
		line := s.Text()

		switch {
		case line == "":
			key = ""
		case strings.HasPrefix(line, "%") && strings.HasSuffix(line, "%") && len(line) > 2:
			key = line[1 : len(line)-1]
		case key != "" && desc[key] == "":
			desc[key] = line
		}
	}

	return desc
}

type CommandRunner func(ctx context.Context, name string, args ...string) ([]byte, error)

type RPM struct {
	run CommandRunner
}

const rpmTimeout = 5 * time.Second

func NewRPM(run CommandRunner) RPM {
	if run == nil {
		run = func(ctx context.Context, name string, args ...string) ([]byte, error) {
			ctx, cancel := context.WithTimeout(ctx, rpmTimeout)
			defer cancel()

			return exec.CommandContext(ctx, name, args...).Output()
		}
	}

	return RPM{run: run}
}

// Stamp is the newest modification time of the rpm database files, sqlite
// with its WAL, Berkeley DB or ndb, in either database location.
func (RPM) Stamp(root string) string {
	var paths []string

	for _, dir := range []string{"/var/lib/rpm", "/usr/lib/sysimage/rpm"} {
		for _, db := range []string{"rpmdb.sqlite", "rpmdb.sqlite-wal", "Packages", "Packages.db"} {
			paths = append(paths, filepath.Join(dir, db))
		}
	}

	return modTimes(root, paths...)
}

func (r RPM) Find(ctx context.Context, root, path string) (Package, bool) {
	if _, err := os.Stat(filepath.Join(root, "/var/lib/rpm")); err != nil {
		return Package{}, false
	}

	args := []string{"-qf", "--queryformat", `%{NAME}\t%{VERSION}-%{RELEASE}\t%{ARCH}\n`, path}

	if root != "" {
		args = append([]string{"--root", root}, args...)
	}

	out, err := r.run(ctx, "rpm", args...)
	if err != nil {
		return Package{}, false
	}

	line, _, _ := strings.Cut(string(out), "\n")

	fields := strings.Split(line, "\t")
	if len(fields) != 3 || fields[0] == "" {
		return Package{}, false
	}

	return Package{Manager: "rpm", Name: fields[0], Version: fields[1], Arch: fields[2]}, true
}
