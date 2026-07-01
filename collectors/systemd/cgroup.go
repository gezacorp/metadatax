package systemd

import (
	"os"
	"regexp"
	"slices"
	"strings"

	"emperror.dev/errors"
	"github.com/prometheus/procfs"
)

var unitSuffixRegex = regexp.MustCompile(`\.(service|socket|timer|scope|mount|swap|path)$`)

func getProc(pid int) (procfs.Proc, error) {
	hostProc := os.Getenv("HOST_PROC")
	if hostProc == "" {
		return procfs.NewProc(pid)
	}

	fs, err := procfs.NewFS(hostProc)
	if err != nil {
		return procfs.Proc{}, errors.WrapIf(err, "could not create a new procfs")
	}

	return fs.Proc(pid)
}

// GetUnitNameForPID returns the systemd unit name owning pid, derived from
// its cgroup membership. It returns an empty string if pid does not belong
// to any systemd unit.
func GetUnitNameForPID(pid int) (string, error) {
	proc, err := getProc(pid)
	if err != nil {
		return "", errors.WrapIf(err, "could not get process info")
	}

	cgroups, err := proc.Cgroups()
	if err != nil {
		return "", errors.WrapIf(err, "could not get cgroups")
	}

	return unitNameFromCgroups(cgroups), nil
}

func unitNameFromCgroups(cgroups []procfs.Cgroup) string {
	cgroup := systemdCgroup(cgroups)
	if cgroup == nil {
		return ""
	}

	parts := strings.Split(strings.Trim(cgroup.Path, "/"), "/")
	if len(parts) == 0 {
		return ""
	}

	last := parts[len(parts)-1]
	if unitSuffixRegex.MatchString(last) {
		return last
	}

	return ""
}

// systemdCgroup picks the cgroup entry that tracks systemd unit membership:
// the "name=systemd" controller on cgroup v1 hosts, or the single unified
// hierarchy on cgroup v2 hosts.
func systemdCgroup(cgroups []procfs.Cgroup) *procfs.Cgroup {
	for i, cgroup := range cgroups {
		if slices.Contains(cgroup.Controllers, "name=systemd") {
			return &cgroups[i]
		}
	}

	for i, cgroup := range cgroups {
		if cgroup.HierarchyID == 0 {
			return &cgroups[i]
		}
	}

	return nil
}
