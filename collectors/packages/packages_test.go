package packages_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gezacorp/metadatax"
	"github.com/gezacorp/metadatax/collectors/packages"
)

func write(t *testing.T, path, content string) {
	t.Helper()

	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
}

func TestHomebrew(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		path string
		want packages.Package
		ok   bool
	}{
		{
			path: "/opt/homebrew/Cellar/node/26.10.0_1/bin/node",
			want: packages.Package{
				Manager: "brew",
				Name:    "node",
				Version: "26.10.0_1",
			},
			ok: true,
		},
		{
			path: "/home/linuxbrew/.linuxbrew/Cellar/python@3.13/3.13.5/bin/python3.13",
			want: packages.Package{
				Manager: "brew",
				Name:    "python@3.13",
				Version: "3.13.5",
			},
			ok: true,
		},
		{
			path: "/usr/bin/curl",
		},
	} {
		got, ok := packages.Homebrew{}.Find(context.Background(), "", tc.path)
		assert.Equal(t, tc.ok, ok, tc.path)
		assert.Equal(t, tc.want, got, tc.path)
	}
}

func TestSnap(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	write(t, filepath.Join(root, "/snap/google-cloud-cli/512/meta/snap.yaml"), "name: google-cloud-cli\nversion: '589.0.0'\nsummary: x\n")

	got, ok := packages.Snap{}.Find(context.Background(), root, "/snap/google-cloud-cli/512/bin/kubectl")
	require.True(t, ok)
	assert.Equal(t, packages.Package{Manager: "snap", Name: "google-cloud-cli", Version: "589.0.0"}, got)

	got, ok = packages.Snap{}.Find(context.Background(), root, "/snap/other/1/bin/x")
	require.True(t, ok)
	assert.Equal(t, packages.Package{Manager: "snap", Name: "other"}, got)

	_, ok = packages.Snap{}.Find(context.Background(), root, "/usr/bin/python3.8")
	assert.False(t, ok)
}

func dpkgRoot(t *testing.T) string {
	t.Helper()

	root := t.TempDir()
	write(t, filepath.Join(root, "/var/lib/dpkg/info/curl.list"), "/.\n/usr\n/usr/bin\n/usr/bin/curl\n")
	write(t, filepath.Join(root, "/var/lib/dpkg/info/coreutils.list"), "/.\n/bin\n/bin/sleep\n")
	write(t, filepath.Join(root, "/var/lib/dpkg/info/python3.12-minimal:arm64.list"), "/usr/bin/python3.12\n")
	write(t, filepath.Join(root, "/var/lib/dpkg/status"), `Package: curl
Status: install ok installed
Architecture: arm64
Version: 8.5.0-2ubuntu10.16

Package: python3.12-minimal
Status: install ok installed
Architecture: amd64
Version: 3.12.0-1

Package: python3.12-minimal
Status: install ok installed
Architecture: arm64
Version: 3.12.3-1ubuntu0.8

Package: coreutils
Architecture: arm64
Version: 9.4-3ubuntu6`)

	return root
}

func TestDpkg(t *testing.T) {
	t.Parallel()

	root := dpkgRoot(t)

	for _, tc := range []struct {
		path string
		want packages.Package
	}{
		{
			path: "/usr/bin/curl",
			want: packages.Package{
				Manager: "dpkg",
				Name:    "curl",
				Version: "8.5.0-2ubuntu10.16",
			},
		},
		{
			path: "/usr/bin/python3.12",
			want: packages.Package{
				Manager: "dpkg",
				Name:    "python3.12-minimal",
				Version: "3.12.3-1ubuntu0.8",
				Arch:    "arm64",
			},
		},
		{
			path: "/usr/bin/sleep",
			want: packages.Package{
				Manager: "dpkg",
				Name:    "coreutils",
				Version: "9.4-3ubuntu6",
			},
		},
	} {
		got, ok := packages.NewDpkg().Find(context.Background(), root, tc.path)
		require.True(t, ok, tc.path)
		assert.Equal(t, tc.want, got, tc.path)
	}

	_, ok := packages.NewDpkg().Find(context.Background(), root, "/usr/local/bin/tool")
	assert.False(t, ok)

	_, ok = packages.NewDpkg().Find(context.Background(), t.TempDir(), "/usr/bin/curl")
	assert.False(t, ok)
}

func TestDpkg_ProgramsInTheUsualDirectoriesAreServedFromMemory(t *testing.T) {
	t.Parallel()

	root := dpkgRoot(t)
	status := filepath.Join(root, "/var/lib/dpkg/status")
	dpkg := packages.NewDpkg()

	ts := time.Now().Add(-time.Hour)
	require.NoError(t, os.Chtimes(status, ts, ts))

	_, ok := dpkg.Find(context.Background(), root, "/usr/bin/curl")
	require.True(t, ok)

	require.NoError(t, os.Remove(filepath.Join(root, "/var/lib/dpkg/info/curl.list")))

	_, ok = dpkg.Find(context.Background(), root, "/usr/bin/curl")
	assert.True(t, ok, "answered from memory while the database is unchanged")

	ts = time.Now()
	require.NoError(t, os.Chtimes(status, ts, ts))

	_, ok = dpkg.Find(context.Background(), root, "/usr/bin/curl")
	assert.False(t, ok, "reloaded once the database changed")
}

func TestDpkg_ProgramsElsewhereAreLookedUpOnDisk(t *testing.T) {
	t.Parallel()

	root := dpkgRoot(t)
	list := filepath.Join(root, "/var/lib/dpkg/info/git.list")
	write(t, list, "/usr/lib/git-core\n/usr/lib/git-core/git-remote-https\n")

	dpkg := packages.NewDpkg()

	got, ok := dpkg.Find(context.Background(), root, "/usr/lib/git-core/git-remote-https")
	require.True(t, ok)
	assert.Equal(t, "git", got.Name)

	require.NoError(t, os.Remove(list))

	_, ok = dpkg.Find(context.Background(), root, "/usr/lib/git-core/git-remote-https")
	assert.False(t, ok)
}

func TestApk(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	write(t, filepath.Join(root, "/lib/apk/db/installed"), `C:Q1abc=
P:musl
V:1.2.5-r10
A:aarch64
F:lib
R:ld-musl-aarch64.so.1

C:Q1def=
P:busybox
V:1.37.0-r31
A:aarch64
r:busybox-initscripts
F:bin
R:busybox
a:0:0:755
Z:Q1orGk=
F:etc/busybox-paths.d
R:busybox

P:curl
V:8.14.1-r1
A:aarch64
F:usr/bin
R:curl
`)

	for _, tc := range []struct {
		path string
		want packages.Package
	}{
		{
			path: "/bin/busybox",
			want: packages.Package{
				Manager: "apk",
				Name:    "busybox",
				Version: "1.37.0-r31",
				Arch:    "aarch64",
			},
		},
		{
			path: "/usr/bin/curl",
			want: packages.Package{
				Manager: "apk",
				Name:    "curl",
				Version: "8.14.1-r1",
				Arch:    "aarch64",
			},
		},
	} {
		got, ok := packages.NewApk().Find(context.Background(), root, tc.path)
		require.True(t, ok, tc.path)
		assert.Equal(t, tc.want, got, tc.path)
	}

	for _, path := range []string{"/etc/busybox", "/usr/local/bin/tool"} {
		_, ok := packages.NewApk().Find(context.Background(), root, path)
		assert.False(t, ok, path)
	}

	_, ok := packages.NewApk().Find(context.Background(), t.TempDir(), "/bin/busybox")
	assert.False(t, ok, "no apk database under root")
}

func TestPacman(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	local := filepath.Join(root, "/var/lib/pacman/local")
	write(t, filepath.Join(local, "curl-8.15.0-1/desc"), "%NAME%\ncurl\n\n%VERSION%\n8.15.0-1\n\n%BASE%\ncurl\n\n%ARCH%\naarch64\n\n")
	write(t, filepath.Join(local, "curl-8.15.0-1/files"), "%FILES%\nusr/\nusr/bin/\nusr/bin/curl\nusr/lib/libcurl.so\n\n%BACKUP%\n")
	write(t, filepath.Join(local, "python-3.13.5-1/desc"), "%NAME%\npython\n\n%VERSION%\n3.13.5-1\n\n%ARCH%\naarch64\n")
	write(t, filepath.Join(local, "python-3.13.5-1/files"), "%FILES%\nusr/bin/python3.13\n")
	write(t, filepath.Join(local, "ALPM_DB_VERSION"), "9\n")

	got, ok := packages.NewPacman().Find(context.Background(), root, "/usr/bin/curl")
	require.True(t, ok)
	assert.Equal(t, packages.Package{Manager: "pacman", Name: "curl", Version: "8.15.0-1", Arch: "aarch64"}, got)

	got, ok = packages.NewPacman().Find(context.Background(), root, "/bin/python3.13")
	require.True(t, ok, "merged /usr: /bin is a link into /usr/bin")
	assert.Equal(t, "python", got.Name)

	for _, path := range []string{"/usr/bin", "/usr/bin/curl2", "/opt/tool"} {
		_, ok = packages.NewPacman().Find(context.Background(), root, path)
		assert.False(t, ok, path)
	}

	_, ok = packages.NewPacman().Find(context.Background(), t.TempDir(), "/usr/bin/curl")
	assert.False(t, ok, "no pacman database under root")
}

func TestRPM(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "/var/lib/rpm"), 0o755))

	var gotArgs []string

	rpm := packages.NewRPM(func(_ context.Context, name string, args ...string) ([]byte, error) {
		gotArgs = append([]string{name}, args...)

		return []byte("curl\t8.2.1-3.el9\taarch64\n"), nil
	})

	got, ok := rpm.Find(context.Background(), root, "/usr/bin/curl")
	require.True(t, ok)
	assert.Equal(t, packages.Package{Manager: "rpm", Name: "curl", Version: "8.2.1-3.el9", Arch: "aarch64"}, got)
	assert.Equal(t, []string{"rpm", "--root", root, "-qf", "--queryformat", `%{NAME}\t%{VERSION}-%{RELEASE}\t%{ARCH}\n`, "/usr/bin/curl"}, gotArgs)

	notOwned := packages.NewRPM(func(context.Context, string, ...string) ([]byte, error) {
		return []byte("file /opt/x is not owned by any package\n"), errors.New("exit status 1")
	})

	_, ok = notOwned.Find(context.Background(), root, "/opt/x")
	assert.False(t, ok)

	_, ok = rpm.Find(context.Background(), t.TempDir(), "/usr/bin/curl")
	assert.False(t, ok, "no rpm database under root")
}

func TestRPMStamp(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	dir := filepath.Join(root, "/var/lib/rpm")
	write(t, filepath.Join(dir, "rpmdb.sqlite"), "db")
	write(t, filepath.Join(dir, "rpmdb.sqlite-wal"), "")

	at := func(name string, d time.Duration) {
		ts := time.Now().Add(d)
		require.NoError(t, os.Chtimes(filepath.Join(dir, name), ts, ts))
	}

	at("rpmdb.sqlite", -time.Hour)
	at("rpmdb.sqlite-wal", -time.Hour)
	at(".", -time.Hour)

	rpm := packages.NewRPM(nil)
	before := rpm.Stamp(root)

	write(t, filepath.Join(dir, "rpmdb.sqlite-shm"), "")
	require.NoError(t, os.Remove(filepath.Join(dir, "rpmdb.sqlite-shm")))
	assert.Equal(t, before, rpm.Stamp(root), "a reader's side files come and go")

	at("rpmdb.sqlite-wal", time.Minute)
	assert.NotEqual(t, before, rpm.Stamp(root), "a write lands in the WAL")
}

type processInfo string

func (p processInfo) ExeWithContext(context.Context) (string, error) {
	return string(p), nil
}

type countingFinder struct {
	calls int
	pkg   packages.Package
}

func (f *countingFinder) Find(context.Context, string, string) (packages.Package, bool) {
	f.calls++

	return f.pkg, f.pkg.Name != ""
}

func TestCollector(t *testing.T) {
	t.Parallel()

	root := dpkgRoot(t)

	md, err := packages.New(
		packages.CollectorWithProcessInfoFunc(func(context.Context, int32) (packages.ProcessInfo, error) {
			return processInfo("/usr/bin/python3.12"), nil
		}),
		packages.CollectorWithRootFunc(func(int32) string { return root }),
	).GetMetadata(metadatax.ContextWithPID(context.Background(), 4242))
	require.NoError(t, err)

	assert.Equal(t, metadatax.Labels{
		"package:manager": {"dpkg"},
		"package:name":    {"python3.12-minimal"},
		"package:version": {"3.12.3-1ubuntu0.8"},
		"package:arch":    {"arm64"},
	}, md.GetLabels())
}

func TestCollector_CachesPerDatabaseAndPath(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	f := &countingFinder{pkg: packages.Package{Manager: "dpkg", Name: "curl"}}
	none := &countingFinder{}

	c := packages.New(
		packages.CollectorWithProcessInfoFunc(func(_ context.Context, pid int32) (packages.ProcessInfo, error) {
			if pid == 3 {
				return processInfo("/opt/tool"), nil
			}

			return processInfo("/usr/bin/curl"), nil
		}),
		packages.CollectorWithRootFunc(func(int32) string { return root }),
		packages.CollectorWithFinders(none, f),
	)

	for _, pid := range []int32{1, 2, 3, 3} {
		_, err := c.GetMetadata(metadatax.ContextWithPID(context.Background(), pid))
		require.NoError(t, err)
	}

	assert.Equal(t, 2, f.calls, "one lookup per path")
	assert.Equal(t, 2, none.calls)
}

func TestCollector_PackageDatabaseChangeDropsCachedAnswers(t *testing.T) {
	t.Parallel()

	root := dpkgRoot(t)
	exe := "/usr/bin/tool"

	c := packages.New(
		packages.CollectorWithProcessInfoFunc(func(context.Context, int32) (packages.ProcessInfo, error) {
			return processInfo(exe), nil
		}),
		packages.CollectorWithRootFunc(func(int32) string { return root }),
	)

	labelsNow := func() metadatax.Labels {
		md, err := c.GetMetadata(metadatax.ContextWithPID(context.Background(), 4242))
		require.NoError(t, err)

		return md.GetLabels()
	}

	status := filepath.Join(root, "/var/lib/dpkg/status")
	touch := func(at time.Time) {
		require.NoError(t, os.Chtimes(status, at, at))
	}

	assert.Empty(t, labelsNow(), "not installed yet")

	write(t, filepath.Join(root, "/var/lib/dpkg/info/tool.list"), "/usr/bin/tool\n")
	b, err := os.ReadFile(status)
	require.NoError(t, err)
	write(t, status, string(b)+"\n\nPackage: tool\nVersion: 1.0\n")
	touch(time.Now().Add(time.Minute))

	assert.Equal(t, []string{"1.0"}, labelsNow()["package:version"], "installed after the miss")

	b, err = os.ReadFile(status)
	require.NoError(t, err)
	write(t, status, strings.Replace(string(b), "Package: tool\nVersion: 1.0", "Package: tool\nVersion: 2.0", 1))
	touch(time.Now().Add(2 * time.Minute))

	assert.Equal(t, []string{"2.0"}, labelsNow()["package:version"], "upgraded")
}

func TestCollector_NoPackage(t *testing.T) {
	t.Parallel()

	md, err := packages.New(
		packages.CollectorWithProcessInfoFunc(func(context.Context, int32) (packages.ProcessInfo, error) {
			return processInfo("/opt/tools/agent"), nil
		}),
		packages.CollectorWithRootFunc(func(int32) string { return t.TempDir() }),
	).GetMetadata(metadatax.ContextWithPID(context.Background(), 4242))
	require.NoError(t, err)
	assert.Empty(t, md.GetLabels())
}
