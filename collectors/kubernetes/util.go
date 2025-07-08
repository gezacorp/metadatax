package kubernetes

import (
	"os"
	"regexp"

	"emperror.dev/errors"
	"github.com/prometheus/procfs"
)

type Cgroup = procfs.Cgroup

func GetCgroupsForPID(pid int) ([]Cgroup, error) {
	proc, err := procfs.NewProc(pid)
	if err != nil {
		return nil, errors.WrapIf(err, "could not get process info")
	}

	cgroups, err := proc.Cgroups()
	if err != nil {
		return nil, errors.WrapIf(err, "could not get cgroups")
	}

	return cgroups, nil
}

func GetContainerIDFromCgroups(cgroups []Cgroup) string {
	dockerCgroupRegex := regexp.MustCompile(`\b((?i)[a-z0-9]{64})`)

	for _, cgroup := range cgroups {
		if match := dockerCgroupRegex.FindStringSubmatch(cgroup.Path); len(match) > 0 {
			return match[1]
		}
	}

	return ""
}

func NodeName() (string, error) {
	return os.Hostname()
}
