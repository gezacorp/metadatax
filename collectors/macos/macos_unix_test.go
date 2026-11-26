//go:build unix

package macos_test

import (
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetMetadataManagedPreferenceFIFO(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	require.NoError(t, syscall.Mkfifo(filepath.Join(dir, "com.example.app.plist"), 0o600))

	done := make(chan map[string][]string)
	go func() {
		done <- managedPreferenceLabels(t, dir, commandOutput{output: enrolledOutput},
			managedPreference{domain: "com.example.app", key: "InstallOnLaunch", label: "app-install"})
	}()

	select {
	case labels := <-done:
		assert.Empty(t, labels)
	case <-time.After(5 * time.Second):
		t.Fatal("reading a FIFO blocked GetMetadata")
	}
}
