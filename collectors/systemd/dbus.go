package systemd

import (
	"context"

	systemddbus "github.com/coreos/go-systemd/v22/dbus"
)

type dbusUnitPropertiesGetter struct{}

// GetUnitProperties opens a short-lived system bus connection to query the
// properties of a single unit, since callers do not run frequently enough
// for a long-lived connection to be worth managing. It fetches properties
// across all interfaces (not just org.freedesktop.systemd1.Unit) so that
// unit-type-specific identity properties, such as a service's User/Group/
// DynamicUser/CapabilityBoundingSet/ExecStart, are included too.
func (dbusUnitPropertiesGetter) GetUnitProperties(ctx context.Context, unitName string) (map[string]any, error) {
	conn, err := systemddbus.NewSystemConnectionContext(ctx)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	return conn.GetAllPropertiesContext(ctx, unitName)
}
