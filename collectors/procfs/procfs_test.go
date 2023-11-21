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

type metadataGetter struct {
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

func (g *metadataGetter) NameWithContext(ctx context.Context) (string, error) {
	return g.name, nil
}

func (g *metadataGetter) CmdlineWithContext(ctx context.Context) (string, error) {
	return g.cmdLine, nil
}

func (g *metadataGetter) UidsWithContext(ctx context.Context) ([]int32, error) {
	return []int32{g.uid, g.uid, g.uid, g.uid}, nil
}

func (g *metadataGetter) GidsWithContext(ctx context.Context) ([]int32, error) {
	return []int32{g.gid, g.gid, g.gid, g.gid}, nil
}

func (g *metadataGetter) GroupsWithContext(ctx context.Context) ([]int32, error) {
	return g.agids, nil
}

func (g *metadataGetter) EnvironWithContext(ctx context.Context) ([]string, error) {
	return g.envs, nil
}

func (g *metadataGetter) ExeWithContext(ctx context.Context) (string, error) {
	return g.exe, nil
}

func TestGetMetadata(t *testing.T) {
	t.Parallel()

	fileContent := "test"
	file, err := os.CreateTemp("", "mxtest*")
	assert.Nil(t, err)
	_, err = file.WriteString(fileContent)
	assert.Nil(t, err)

	getter := &metadataGetter{
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
		"process:binary:path":    {getter.exe},
		"process:binary:hash":    {getter.hash},
		"process:cmdline":        {getter.cmdLine},
		"process:gid":            {strconv.Itoa(int(getter.gid))},
		"process:gid:additional": {strconv.Itoa(int(getter.agids[0])), strconv.Itoa(int(getter.agids[1])), strconv.Itoa(int(getter.agids[2])), strconv.Itoa(int(getter.agids[3]))},
		"process:gid:effective":  {strconv.Itoa(int(getter.gid))},
		"process:gid:real":       {strconv.Itoa(int(getter.gid))},
		"process:name":           {getter.name},
		"process:pid":            {strconv.Itoa(int(getter.pid))},
		"process:uid":            {strconv.Itoa(int(getter.uid))},
		"process:uid:effective":  {strconv.Itoa(int(getter.uid))},
		"process:uid:real":       {strconv.Itoa(int(getter.uid))},
	}

	ctx := metadatax.ContextWithPID(context.Background(), int32(getter.pid))
	md, err := procfs.New(procfs.CollectorWithMetadataGetter(getter)).GetMetadata(ctx)
	assert.Nil(t, err)
	assert.Equal(t, expected, map[string][]string(md.GetLabels()))
}
