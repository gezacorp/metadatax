//go:build unix

package packages

import (
	"os"
	"syscall"
)

func fileID(path string) (uint64, uint64, bool) {
	fi, err := os.Stat(path)
	if err != nil {
		return 0, 0, false
	}

	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, 0, false
	}

	return uint64(st.Dev), st.Ino, true //nolint:unconvert
}
