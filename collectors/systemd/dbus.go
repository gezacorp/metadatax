package systemd

import (
	"context"
	"errors"
	"os"

	systemddbus "github.com/coreos/go-systemd/v22/dbus"
	"github.com/godbus/dbus/v5"
	"github.com/shirou/gopsutil/v4/process"
)

// noUnitForPIDErrorName is the D-Bus error systemd returns from
// GetUnitByPID/GetUnitNameByPID both when the PID doesn't belong to any
// loaded unit (e.g. a kernel thread) and when the PID doesn't exist at all -
// confirmed live against a running systemd 255 instance. The two cases are
// disambiguated separately via process.PidExists.
const noUnitForPIDErrorName = "org.freedesktop.systemd1.NoUnitForPID"

func isNoUnitForPID(err error) bool {
	var dbusErr dbus.Error
	if errors.As(err, &dbusErr) {
		return dbusErr.Name == noUnitForPIDErrorName
	}

	return false
}

type dbusUnitNameGetter struct{}

// GetUnitNameForPID asks systemd itself which unit owns pid, via
// org.freedesktop.systemd1.Manager.GetUnitByPID. This is authoritative and
// handles cases a cgroup-path-parsing approach would need extra logic for -
// nested/delegated cgroups (docker.service's containers), login sessions,
// etc. - since systemd resolves it internally rather than us re-deriving it
// from cgroup path shape.
func (dbusUnitNameGetter) GetUnitNameForPID(pid int) (string, error) {
	if pid < 0 {
		return "", errors.New("invalid pid: must not be negative")
	}

	ctx := context.Background()

	conn, err := systemddbus.NewSystemConnectionContext(ctx)
	if err != nil {
		return "", err
	}
	defer conn.Close()

	name, err := conn.GetUnitNameByPID(ctx, uint32(pid))
	if err == nil {
		return name, nil
	}

	if !isNoUnitForPID(err) {
		return "", err
	}

	// GetUnitNameByPID reports the same error whether pid doesn't exist or
	// simply isn't unit-tracked; process.PidExists tells those two cases
	// apart so the caller can still distinguish "process gone" (os.ErrNotExist)
	// from "process alive, no unit" (empty name, no error). A failure of the
	// existence check itself is a real error and must not be silently
	// treated as either case.
	exists, err := process.PidExists(int32(pid))
	if err != nil {
		return "", err
	}

	if !exists {
		return "", os.ErrNotExist
	}

	return "", nil
}

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
