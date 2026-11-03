package procfs

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/shirou/gopsutil/v4/process"
)

// noLoginUID is the login uid of a process no login started, such as a
// service or a container.
const noLoginUID = 4294967295

// procInfo adds what gopsutil does not read to its process.
type procInfo struct {
	*process.Process
}

func (p *procInfo) LoginUIDWithContext(context.Context) (uint32, error) {
	b, err := os.ReadFile(filepath.Join(procPath(), strconv.Itoa(int(p.Pid)), "loginuid"))
	if err != nil {
		return 0, err
	}

	v, err := strconv.ParseUint(strings.TrimSpace(string(b)), 10, 32)

	return uint32(v), err
}

func (p *procInfo) CgroupWithContext(context.Context) (string, error) {
	b, err := os.ReadFile(filepath.Join(procPath(), strconv.Itoa(int(p.Pid)), "cgroup"))
	if err != nil {
		return "", err
	}

	return CgroupPath(string(b)), nil
}

func (p *procInfo) SessionIDWithContext(context.Context) (int32, error) {
	b, err := os.ReadFile(filepath.Join(procPath(), strconv.Itoa(int(p.Pid)), "stat"))
	if err != nil {
		return 0, err
	}

	sid, ok := StatSessionID(string(b))
	if !ok {
		return 0, os.ErrInvalid
	}

	return sid, nil
}

// StatSessionID reads the session ID, the sixth field, from the content of
// a /proc/<pid>/stat file. The command name in the second field can hold
// spaces and parentheses, so the fields after it are counted from its last
// closing parenthesis.
func StatSessionID(stat string) (int32, bool) {
	i := strings.LastIndexByte(stat, ')')
	if i < 0 {
		return 0, false
	}

	// state, ppid, pgrp, session
	fields := strings.Fields(stat[i+1:])
	if len(fields) < 4 {
		return 0, false
	}

	sid, err := strconv.ParseInt(fields[3], 10, 32)
	if err != nil {
		return 0, false
	}

	return int32(sid), true
}
