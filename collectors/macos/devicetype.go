package macos

import "strings"

type DeviceType string

const (
	DeviceTypeLaptop  DeviceType = "laptop"
	DeviceTypeDesktop DeviceType = "desktop"
	DeviceTypeMini    DeviceType = "mini"
	DeviceTypeVirtual DeviceType = "virtual"
	DeviceTypeUnknown DeviceType = "unknown"
)

// DetectDeviceType classifies a Mac by its model name, falling back to the legacy model identifier.
func DetectDeviceType(modelName, modelIdentifier string, isVirtual bool) DeviceType {
	if isVirtual || strings.HasPrefix(modelIdentifier, "VirtualMac") {
		return DeviceTypeVirtual
	}

	name := strings.ToLower(modelName)
	switch {
	case strings.HasPrefix(name, "macbook"):
		return DeviceTypeLaptop
	case strings.HasPrefix(name, "mac mini"):
		return DeviceTypeMini
	case strings.HasPrefix(name, "imac"),
		strings.HasPrefix(name, "mac studio"),
		strings.HasPrefix(name, "mac pro"):
		return DeviceTypeDesktop
	}

	switch {
	case strings.HasPrefix(modelIdentifier, "MacBook"):
		return DeviceTypeLaptop
	case strings.HasPrefix(modelIdentifier, "Macmini"):
		return DeviceTypeMini
	case strings.HasPrefix(modelIdentifier, "iMac"),
		strings.HasPrefix(modelIdentifier, "MacPro"):
		return DeviceTypeDesktop
	}

	return DeviceTypeUnknown
}
