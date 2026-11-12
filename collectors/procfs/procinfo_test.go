package procfs_test

import (
	"context"
	"os"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/shirou/gopsutil/v4/process"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gezacorp/metadatax"
	"github.com/gezacorp/metadatax/collectors/procfs"
)

func TestGetMetadata_NoLoginUID(t *testing.T) {
	t.Parallel()

	p := &processInfo{
		name:     "nginx",
		pid:      4321,
		ppid:     1,
		loginUID: 4294967295,
		sid:      4321,
		cgroup:   "/system.slice/nginx.service",
	}

	md, err := procfs.New(
		procfs.CollectorWithProcessInfoFunc(func(context.Context, int32) (procfs.ProcessInfo, error) {
			return p, nil
		}),
		procfs.WithForceHasProcFS(),
	).GetMetadata(metadatax.ContextWithPID(context.Background(), 4321))
	require.NoError(t, err)

	labels := md.GetLabels()
	assert.NotContains(t, labels, "process:uid:login")
	assert.Equal(t, []string{"4321"}, labels["process:sid"])
	assert.Equal(t, []string{"/system.slice/nginx.service"}, labels["process:cgroup"])
	assert.Equal(t, []string{"1"}, labels["process:ppid"])
}

func TestGetMetadata_GopsutilProcessAsProcessInfo(t *testing.T) {
	t.Parallel()

	pid := int32(os.Getpid())

	md, err := procfs.New(
		procfs.CollectorWithProcessInfoFunc(func(ctx context.Context, pid int32) (procfs.ProcessInfo, error) {
			return process.NewProcessWithContext(ctx, pid)
		}),
		procfs.WithForceHasProcFS(),
	).GetMetadata(metadatax.ContextWithPID(context.Background(), pid))
	require.NoError(t, err)

	labels := md.GetLabels()
	assert.Equal(t, []string{strconv.Itoa(os.Getppid())}, labels["process:ppid"])
	assert.Contains(t, labels, "process:arg:0")
	assert.NotContains(t, labels, "process:sid")
	assert.NotContains(t, labels, "process:cgroup")
	assert.NotContains(t, labels, "process:uid:login")
}

func TestGetMetadata_LongCommandLine(t *testing.T) {
	t.Parallel()

	args := []string{"/usr/bin/java"}
	for i := range 300 {
		args = append(args, "-Dproperty."+strconv.Itoa(i)+"=café")
	}

	p := &processInfo{
		name: "java",
		args: args,
	}

	md, err := procfs.New(
		procfs.CollectorWithProcessInfoFunc(func(context.Context, int32) (procfs.ProcessInfo, error) {
			return p, nil
		}),
		procfs.WithForceHasProcFS(),
	).GetMetadata(metadatax.ContextWithPID(context.Background(), 4321))
	require.NoError(t, err)

	labels := md.GetLabels()

	cmdline := labels["process:cmdline"][0]
	assert.LessOrEqual(t, len(cmdline), 1024)
	assert.True(t, utf8.ValidString(cmdline))
	assert.True(t, strings.HasPrefix(strings.Join(args, " "), cmdline))

	var covered []string
	for i := 0; labels["process:arg:"+strconv.Itoa(i)] != nil; i++ {
		covered = append(covered, labels["process:arg:"+strconv.Itoa(i)][0])
	}

	assert.Len(t, covered, 32)
	assert.Equal(t, strings.Join(covered, " "), cmdline)
}

func TestGetMetadata_InvalidUTF8InArguments(t *testing.T) {
	t.Parallel()

	args := []string{"tool", "b\xffb", "\xff"}

	md, err := procfs.New(
		procfs.CollectorWithProcessInfoFunc(func(context.Context, int32) (procfs.ProcessInfo, error) {
			return &processInfo{
				name: "tool",
				args: args,
			}, nil
		}),
		procfs.WithForceHasProcFS(),
	).GetMetadata(metadatax.ContextWithPID(context.Background(), 4321))
	require.NoError(t, err)

	labels := md.GetLabels()
	assert.Equal(t, []string{"tool bb"}, labels["process:cmdline"])
	assert.Equal(t, []string{"bb"}, labels["process:arg:1"])
	assert.NotContains(t, labels, "process:arg:2")
}

func TestGetMetadata_CommandLineLimits(t *testing.T) {
	t.Parallel()

	collect := func(args []string, opts ...procfs.CollectorOption) metadatax.Labels {
		md, err := procfs.New(append(opts,
			procfs.CollectorWithProcessInfoFunc(func(context.Context, int32) (procfs.ProcessInfo, error) {
				return &processInfo{
					name: "tool",
					args: args,
				}, nil
			}),
			procfs.WithForceHasProcFS(),
		)...).GetMetadata(metadatax.ContextWithPID(context.Background(), 4321))
		require.NoError(t, err)

		return md.GetLabels()
	}

	args := []string{"tool", "--password", "correct horse battery staple", "--user", "bob"}

	labels := collect(args, procfs.CollectorWithMaxArgs(2))
	assert.Equal(t, []string{"tool --password"}, labels["process:cmdline"])
	assert.NotContains(t, labels, "process:arg:2")

	labels = collect(args, procfs.CollectorWithMaxCmdlineLen(20))
	assert.Equal(t, []string{"tool --password corr"}, labels["process:cmdline"])
	assert.Equal(t, []string{"corr"}, labels["process:arg:2"])
	assert.NotContains(t, labels, "process:arg:3")

	many := []string{"tool"}
	for i := range 40 {
		many = append(many, "-a"+strconv.Itoa(i))
	}

	for _, opt := range []procfs.CollectorOption{
		procfs.CollectorWithMaxArgs(0),
		procfs.CollectorWithMaxArgs(-1),
		procfs.CollectorWithMaxCmdlineLen(0),
		procfs.CollectorWithMaxCmdlineLen(-1),
	} {
		labels = collect(many, opt)
		assert.Equal(t, []string{strings.Join(many[:32], " ")}, labels["process:cmdline"])
	}

	labels = collect([]string{"aaa", "é"}, procfs.CollectorWithMaxCmdlineLen(5))
	assert.Equal(t, []string{"aaa"}, labels["process:cmdline"])
	assert.NotContains(t, labels, "process:arg:1")

	labels = collect([]string{"aaa", "", "bbb"})
	assert.Equal(t, []string{"aaa bbb"}, labels["process:cmdline"])
	assert.Equal(t, []string{"bbb"}, labels["process:arg:1"])
	assert.NotContains(t, labels, "process:arg:2")
}

func TestGetMetadata_ArgumentsKeepTheirSpaces(t *testing.T) {
	t.Parallel()

	args := []string{"tool", "--password", "correct horse battery staple"}

	md, err := procfs.New(
		procfs.CollectorWithProcessInfoFunc(func(context.Context, int32) (procfs.ProcessInfo, error) {
			return &processInfo{
				name: "tool",
				args: args,
			}, nil
		}),
		procfs.WithForceHasProcFS(),
	).GetMetadata(metadatax.ContextWithPID(context.Background(), 4321))
	require.NoError(t, err)

	labels := md.GetLabels()
	assert.Equal(t, []string{"tool --password correct horse battery staple"}, labels["process:cmdline"])
	assert.Equal(t, []string{"correct horse battery staple"}, labels["process:arg:2"])
}

func TestGetMetadata_ArgumentLongerThanTheCommandLine(t *testing.T) {
	t.Parallel()

	args := []string{"/usr/bin/node", "-e", strings.Repeat("é", 1000)}

	md, err := procfs.New(
		procfs.CollectorWithProcessInfoFunc(func(context.Context, int32) (procfs.ProcessInfo, error) {
			return &processInfo{
				name: "node",
				args: args,
			}, nil
		}),
		procfs.WithForceHasProcFS(),
	).GetMetadata(metadatax.ContextWithPID(context.Background(), 4321))
	require.NoError(t, err)

	labels := md.GetLabels()

	cmdline := labels["process:cmdline"][0]
	assert.LessOrEqual(t, len(cmdline), 1024)
	assert.True(t, utf8.ValidString(cmdline))

	last := labels["process:arg:2"][0]
	assert.True(t, utf8.ValidString(last))
	assert.True(t, strings.HasPrefix(args[2], last))
	assert.Equal(t, "/usr/bin/node -e "+last, cmdline)
	assert.NotContains(t, labels, "process:arg:3")
}

func TestStatSessionID(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		stat string
		want int32
		ok   bool
	}{
		{
			name: "plain",
			stat: "320848 (curl) S 1 320848 320712 0 -1 4194560 120 0 0 0 0 0 0 0 20 0 1 0 1234 0 0\n",
			want: 320712,
			ok:   true,
		},
		{
			name: "command with spaces and parens",
			stat: "4242 (my (odd) cmd) R 1 4242 4200 0 -1\n",
			want: 4200,
			ok:   true,
		},
		{
			name: "truncated",
			stat: "4242 (x) R 1",
		},
		{
			name: "garbage",
			stat: "nothing here",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, ok := procfs.StatSessionID(tc.stat)
			assert.Equal(t, tc.ok, ok)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestCgroupPath(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name    string
		content string
		want    string
	}{
		{
			name:    "unified",
			content: "0::/user.slice/user-501.slice/session-5.scope\n",
			want:    "/user.slice/user-501.slice/session-5.scope",
		},
		{
			name:    "hybrid prefers the unified hierarchy",
			content: "12:cpu,cpuacct:/system.slice/docker.service\n1:name=systemd:/system.slice/docker.service\n0::/system.slice/docker-abc.scope\n",
			want:    "/system.slice/docker-abc.scope",
		},
		{
			name:    "v1 takes the systemd hierarchy",
			content: "12:cpu,cpuacct:/kubepods/burstable\n1:name=systemd:/kubepods/burstable/pod1/abc\n",
			want:    "/kubepods/burstable/pod1/abc",
		},
		{
			name:    "nothing usable",
			content: "garbage\n",
			want:    "",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.want, procfs.CgroupPath(tc.content))
		})
	}
}
