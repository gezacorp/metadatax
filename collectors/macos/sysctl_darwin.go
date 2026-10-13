//go:build darwin

package macos

import "golang.org/x/sys/unix"

func getSysctl(name string) (string, error) {
	return unix.Sysctl(name)
}

func getSysctlUint32(name string) (uint32, error) {
	return unix.SysctlUint32(name)
}
