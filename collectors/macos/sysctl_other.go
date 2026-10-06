//go:build !darwin

package macos

import "emperror.dev/errors"

var errSysctlNotSupported = errors.New("sysctl is only supported on darwin")

func getSysctl(string) (string, error) {
	return "", errSysctlNotSupported
}

func getSysctlUint32(string) (uint32, error) {
	return 0, errSysctlNotSupported
}
