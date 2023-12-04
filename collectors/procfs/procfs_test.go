package procfs_test

import (
	"context"
	"os"
	"strconv"
	"testing"

	"github.com/opencontainers/go-digest"
	"github.com/stretchr/testify/assert"

	"github.com/gezacorp/metadatax"
	"github.com/gezacorp/metadatax/collectors/procfs"
)

type rawData struct {
	exe     string
	hash    string
	cmdLine string
	uid     int32
	gid     int32
	name    string
	pid     int
	agids   []int32
	envs    []string
}

func (g *rawData) NameWithContext(ctx context.Context) (string, error) {
	return g.name, nil
}

func (g *rawData) CmdlineWithContext(ctx context.Context) (string, error) {
	return g.cmdLine, nil
}

func (g *rawData) UidsWithContext(ctx context.Context) ([]int32, error) {
	return []int32{g.uid, g.uid, g.uid, g.uid}, nil
}

func (g *rawData) GidsWithContext(ctx context.Context) ([]int32, error) {
	return []int32{g.gid, g.gid, g.gid, g.gid}, nil
}

func (g *rawData) GroupsWithContext(ctx context.Context) ([]int32, error) {
	return g.agids, nil
}

func (g *rawData) EnvironWithContext(ctx context.Context) ([]string, error) {
	return g.envs, nil
}

func (g *rawData) ExeWithContext(ctx context.Context) (string, error) {
	return g.exe, nil
}

func TestGetMetadata(t *testing.T) {
	t.Parallel()

	fileContent := "test"
	file, err := os.CreateTemp("", "mxtest*")
	assert.Nil(t, err)
	_, err = file.WriteString(fileContent)
	assert.Nil(t, err)

	rawData := &rawData{
		exe:     file.Name(),
		hash:    digest.SHA256.FromString(fileContent).String(),
		cmdLine: "./test-command",
		uid:     501,
		gid:     502,
		name:    "test",
		pid:     1001,
		agids:   []int32{101, 102, 103, 104},
		envs:    []string{"a=b", "c=d"},
	}

	expected := map[string][]string{
		"process:binary:path":    {rawData.exe},
		"process:binary:hash":    {rawData.hash},
		"process:cmdline":        {rawData.cmdLine},
		"process:gid":            {strconv.Itoa(int(rawData.gid))},
		"process:gid:additional": {strconv.Itoa(int(rawData.agids[0])), strconv.Itoa(int(rawData.agids[1])), strconv.Itoa(int(rawData.agids[2])), strconv.Itoa(int(rawData.agids[3]))},
		"process:gid:effective":  {strconv.Itoa(int(rawData.gid))},
		"process:gid:real":       {strconv.Itoa(int(rawData.gid))},
		"process:name":           {rawData.name},
		"process:pid":            {strconv.Itoa(int(rawData.pid))},
		"process:uid":            {strconv.Itoa(int(rawData.uid))},
		"process:uid:effective":  {strconv.Itoa(int(rawData.uid))},
		"process:uid:real":       {strconv.Itoa(int(rawData.uid))},
	}

	ctx := metadatax.ContextWithPID(context.Background(), int32(rawData.pid))
	md, err := procfs.New(
		procfs.CollectorWithRawDataGetterFunc(
			func(ctx context.Context, pid int32) (procfs.RawData, error) {
				return rawData, nil
			},
		),
	).GetMetadata(ctx)
	assert.Nil(t, err)
	assert.Equal(t, expected, map[string][]string(md.GetLabels()))
}
