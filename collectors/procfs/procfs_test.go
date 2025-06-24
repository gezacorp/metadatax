package procfs_test

import (
	"context"
	"os"
	"strconv"
	"testing"

	"github.com/opencontainers/go-digest"
	"github.com/shirou/gopsutil/v4/net"
	"github.com/stretchr/testify/assert"

	"github.com/gezacorp/metadatax"
	"github.com/gezacorp/metadatax/collectors/procfs"
)

type processInfo struct {
	exe         string
	hash        string
	cmdLine     string
	uid         uint32
	gid         uint32
	name        string
	pid         int
	agids       []uint32
	envs        []string
	connections []net.ConnectionStat
}

func (i *processInfo) NameWithContext(ctx context.Context) (string, error) {
	return i.name, nil
}

func (i *processInfo) CmdlineWithContext(ctx context.Context) (string, error) {
	return i.cmdLine, nil
}

func (i *processInfo) UidsWithContext(ctx context.Context) ([]uint32, error) {
	return []uint32{i.uid, i.uid, i.uid, i.uid}, nil
}

func (i *processInfo) GidsWithContext(ctx context.Context) ([]uint32, error) {
	return []uint32{i.gid, i.gid, i.gid, i.gid}, nil
}

func (i *processInfo) GroupsWithContext(ctx context.Context) ([]uint32, error) {
	return i.agids, nil
}

func (i *processInfo) EnvironWithContext(ctx context.Context) ([]string, error) {
	return i.envs, nil
}

func (i *processInfo) ExeWithContext(ctx context.Context) (string, error) {
	return i.exe, nil
}

func (i *processInfo) ConnectionsWithContext(ctx context.Context) ([]net.ConnectionStat, error) {
	return i.connections, nil
}

func TestGetMetadata(t *testing.T) {
	t.Parallel()

	fileContent := "test"
	file, err := os.CreateTemp("", "mxtest*")
	assert.Nil(t, err)
	defer os.Remove(file.Name())
	_, err = file.WriteString(fileContent)
	assert.Nil(t, err)

	processInfo := &processInfo{
		exe:     file.Name(),
		hash:    digest.SHA256.FromString(fileContent).String(),
		cmdLine: "./test-command",
		uid:     501,
		gid:     502,
		name:    "test",
		pid:     1001,
		agids:   []uint32{101, 102, 103, 104},
		envs:    []string{"a=b", "c=d"},
		connections: []net.ConnectionStat{
			{
				Laddr:  net.Addr{IP: "127.0.0.1", Port: 8080},
				Status: "LISTEN",
			},
		},
	}

	expected := map[string][]string{
		"process:binary:path":    {processInfo.exe},
		"process:binary:hash":    {processInfo.hash},
		"process:cmdline":        {processInfo.cmdLine},
		"process:gid":            {strconv.Itoa(int(processInfo.gid))},
		"process:gid:additional": {strconv.Itoa(int(processInfo.agids[0])), strconv.Itoa(int(processInfo.agids[1])), strconv.Itoa(int(processInfo.agids[2])), strconv.Itoa(int(processInfo.agids[3]))},
		"process:gid:effective":  {strconv.Itoa(int(processInfo.gid))},
		"process:gid:real":       {strconv.Itoa(int(processInfo.gid))},
		"process:name":           {processInfo.name},
		"process:pid":            {strconv.Itoa(int(processInfo.pid))},
		"process:uid":            {strconv.Itoa(int(processInfo.uid))},
		"process:uid:effective":  {strconv.Itoa(int(processInfo.uid))},
		"process:uid:real":       {strconv.Itoa(int(processInfo.uid))},
		"process:network:bindings":   {"127.0.0.1:8080"},
	}

	ctx := metadatax.ContextWithPID(context.Background(), int32(processInfo.pid))
	md, err := procfs.New(
		procfs.CollectorWithProcessInfoFunc(
			func(ctx context.Context, pid int32) (procfs.ProcessInfo, error) {
				return processInfo, nil
			},
		),
		procfs.CollectorWithMetadataContainerInitFunc(func() metadatax.MetadataContainer {
			return metadatax.New(
				metadatax.WithPrefix("process"),
			)
		}),
	).GetMetadata(ctx)
	assert.Nil(t, err)
	assert.Equal(t, expected, map[string][]string(md.GetLabels()))
}
