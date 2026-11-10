//go:build darwin && cgo

package macosprocess

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInspect_Self(t *testing.T) {
	t.Parallel()

	p, err := inspect(context.Background(), int32(os.Getpid()))
	require.NoError(t, err)

	exe, err := os.Executable()
	require.NoError(t, err)

	exe, err = filepath.EvalSymlinks(exe)
	require.NoError(t, err)

	assert.Equal(t, exe, p.Path)
	assert.Equal(t, os.Args, p.Args)
	assert.Equal(t, int32(os.Getppid()), p.PPID)
	assert.Equal(t, uint32(os.Getuid()), p.RUID)
	assert.Equal(t, uint32(os.Geteuid()), p.EUID)
	assert.Equal(t, uint32(os.Getgid()), p.RGID)
	assert.True(t, p.Audit)
	assert.NotZero(t, p.Signature.Status&csValid)
	assert.Len(t, p.Signature.CDHash, 40)
	assert.Positive(t, p.Responsible.PID)
	assert.NotEmpty(t, p.Responsible.Path)
	assert.NotZero(t, p.Responsible.Signature.Status&csValid)
}

func TestInspect_ResponsibleForItself(t *testing.T) {
	t.Parallel()

	self, err := inspect(context.Background(), int32(os.Getpid()))
	require.NoError(t, err)

	p, err := inspect(context.Background(), self.Responsible.PID)
	require.NoError(t, err)

	if p.Responsible.PID != self.Responsible.PID {
		t.Skip("the test's responsible app is not responsible for itself")
	}

	assert.Equal(t, p.Signature, p.Responsible.Signature)
	assert.Equal(t, p.Bundle, p.Responsible.Bundle)
	assert.Equal(t, self.Responsible.Path, p.Responsible.Path)
}

func writePlist(t *testing.T, app, id, version string, at time.Time) {
	t.Helper()

	plist := filepath.Join(app, "Contents", "Info.plist")
	require.NoError(t, os.MkdirAll(filepath.Dir(plist), 0o755))
	require.NoError(t, os.WriteFile(plist, []byte(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
<key>CFBundleIdentifier</key><string>`+id+`</string>
<key>CFBundleName</key><string>Tool</string>
<key>CFBundleShortVersionString</key><string>`+version+`</string>
<key>CFBundleExecutable</key><string>tool</string>
</dict></plist>
`), 0o644))
	require.NoError(t, os.Chtimes(plist, at, at))
}

func TestBundleOf_CachedUntilInfoPlistChanges(t *testing.T) {
	t.Parallel()

	app := filepath.Join(t.TempDir(), "Tool.app")
	exe := filepath.Join(app, "Contents", "MacOS", "tool")
	past := time.Now().Add(-time.Hour)

	writePlist(t, app, "io.example.tool", "1.0", past)
	assert.Equal(t, "1.0", bundleOf(exe).Version)

	writePlist(t, app, "io.example.tool", "2.0", past)
	assert.Equal(t, "1.0", bundleOf(exe).Version, "cached while Info.plist's time is unchanged")

	writePlist(t, app, "io.example.tool", "2.0", time.Now())
	assert.Equal(t, "2.0", bundleOf(exe).Version)
}

func TestBundleOf_BinaryInfoPlist(t *testing.T) {
	t.Parallel()

	app := filepath.Join(t.TempDir(), "Tool.app")
	writePlist(t, app, "io.example.binary", "3.1", time.Now())
	require.NoError(t, exec.Command("plutil", "-convert", "binary1", filepath.Join(app, "Contents", "Info.plist")).Run())

	b := bundleOf(filepath.Join(app, "Contents", "MacOS", "tool"))
	assert.Equal(t, "io.example.binary", b.ID)
	assert.Equal(t, "3.1", b.Version)
}

func TestInspect_Launchd(t *testing.T) {
	t.Parallel()

	p, err := inspect(context.Background(), 1)
	require.NoError(t, err)

	assert.Equal(t, "launchd", p.Name)
	assert.Equal(t, "com.apple.xpc.launchd", p.Signature.Identifier)
	assert.NotZero(t, p.Signature.Status&csPlatformBinary)
}

func TestInspect_NoProcess(t *testing.T) {
	t.Parallel()

	_, err := inspect(context.Background(), 99999)
	require.ErrorIs(t, err, ErrProcessNotFound)
	require.ErrorIs(t, err, os.ErrNotExist)
}
