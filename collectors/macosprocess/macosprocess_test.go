package macosprocess_test

import (
	"context"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gezacorp/metadatax"
	"github.com/gezacorp/metadatax/collectors/macosprocess"
)

func collect(t *testing.T, p macosprocess.ProcessInfo, opts ...macosprocess.CollectorOption) metadatax.Labels {
	t.Helper()

	c := macosprocess.New(append(opts, macosprocess.CollectorWithProcessInfoFunc(func(context.Context, int32) (macosprocess.ProcessInfo, error) {
		return p, nil
	}))...)

	md, err := c.GetMetadata(metadatax.ContextWithPID(context.Background(), 4242))
	require.NoError(t, err)

	return md.GetLabels()
}

func TestCollector_SignedAppInABundle(t *testing.T) {
	t.Parallel()

	labels := collect(t, macosprocess.ProcessInfo{
		Name:     "zed",
		Path:     "/Applications/Zed.app/Contents/MacOS/zed",
		ExecPath: "/Applications/Zed.app/Contents/MacOS/zed",
		Args:     []string{"/Applications/Zed.app/Contents/MacOS/zed", "--foreground"},
		PPID:     1,
		RUID:     501,
		EUID:     501,
		RGID:     20,
		EGID:     20,
		Audit:    true,
		AUID:     501,
		Session:  100045,
		Signature: macosprocess.Signature{
			Identifier: "dev.zed.Zed",
			Team:       "MQ55VZLNZQ",
			CDHash:     "0322b0bbdae7e5d35b3c8fb7da4f6baf22c3dde1",
			Status:     0x22010301,
		},
		Bundle: macosprocess.Bundle{
			Path:    "/Applications/Zed.app",
			ID:      "dev.zed.Zed",
			Name:    "Zed",
			Version: "0.210.4",
		},
		Responsible: macosprocess.Responsible{
			PID:  4242,
			Path: "/Applications/Zed.app/Contents/MacOS/zed",
		},
	})

	assert.Equal(t, metadatax.Labels{
		"process:pid":                        {"4242"},
		"process:name":                       {"zed"},
		"process:cmdline":                    {"/Applications/Zed.app/Contents/MacOS/zed --foreground"},
		"process:arg:0":                      {"/Applications/Zed.app/Contents/MacOS/zed"},
		"process:arg:1":                      {"--foreground"},
		"process:ppid":                       {"1"},
		"process:uid":                        {"501"},
		"process:uid:real":                   {"501"},
		"process:uid:effective":              {"501"},
		"process:uid:login":                  {"501"},
		"process:gid":                        {"20"},
		"process:gid:real":                   {"20"},
		"process:gid:effective":              {"20"},
		"process:session":                    {"100045"},
		"process:binary:path":                {"/Applications/Zed.app/Contents/MacOS/zed"},
		"process:signature:identifier":       {"dev.zed.Zed"},
		"process:signature:team":             {"MQ55VZLNZQ"},
		"process:signature:cdhash":           {"0322b0bbdae7e5d35b3c8fb7da4f6baf22c3dde1"},
		"process:signature:platform":         {"false"},
		"process:signature:adhoc":            {"false"},
		"process:signature:hardened-runtime": {"true"},
		"process:bundle:path":                {"/Applications/Zed.app"},
		"process:bundle:id":                  {"dev.zed.Zed"},
		"process:bundle:name":                {"Zed"},
		"process:bundle:version":             {"0.210.4"},
		"process:responsible:pid":            {"4242"},
		"process:responsible:path":           {"/Applications/Zed.app/Contents/MacOS/zed"},
	}, labels)
}

func TestCollector_ResponsibleAppSignatureAndBundle(t *testing.T) {
	t.Parallel()

	labels := collect(t, macosprocess.ProcessInfo{
		Name: "claude",
		Path: "/opt/claude/claude",
		Signature: macosprocess.Signature{
			Identifier: "com.anthropic.claude-code",
			Team:       "Q6L2SF6YDW",
			Status:     0x22010301,
		},
		Responsible: macosprocess.Responsible{
			PID:  1628,
			Path: "/Applications/Zed.app/Contents/MacOS/zed",
			Signature: macosprocess.Signature{
				Identifier: "dev.zed.Zed",
				Team:       "MQ55VZLNZQ",
				CDHash:     "8a1f0c2e9d3b4a5f6e7d8c9b0a1f2e3d4c5b6a79",
				Status:     0x22010301,
			},
			Bundle: macosprocess.Bundle{
				Path:    "/Applications/Zed.app",
				ID:      "dev.zed.Zed",
				Name:    "Zed",
				Version: "0.210.4",
			},
		},
	})

	assert.Equal(t, []string{"com.anthropic.claude-code"}, labels["process:signature:identifier"])
	assert.Equal(t, []string{"1628"}, labels["process:responsible:pid"])
	assert.Equal(t, []string{"/Applications/Zed.app/Contents/MacOS/zed"}, labels["process:responsible:path"])
	assert.Equal(t, []string{"dev.zed.Zed"}, labels["process:responsible:signature:identifier"])
	assert.Equal(t, []string{"MQ55VZLNZQ"}, labels["process:responsible:signature:team"])
	assert.Equal(t, []string{"8a1f0c2e9d3b4a5f6e7d8c9b0a1f2e3d4c5b6a79"}, labels["process:responsible:signature:cdhash"])
	assert.Equal(t, []string{"false"}, labels["process:responsible:signature:platform"])
	assert.Equal(t, []string{"dev.zed.Zed"}, labels["process:responsible:bundle:id"])
	assert.Equal(t, []string{"0.210.4"}, labels["process:responsible:bundle:version"])
	assert.NotContains(t, labels, "process:bundle:path")
}

func TestCollector_DaemonWithoutLoginUser(t *testing.T) {
	t.Parallel()

	labels := collect(t, macosprocess.ProcessInfo{
		Name:      "mDNSResponder",
		Path:      "/usr/sbin/mDNSResponder",
		RUID:      65,
		EUID:      65,
		Audit:     true,
		AUID:      0xffffffff,
		Session:   100015,
		Signature: macosprocess.Signature{Identifier: "com.apple.mDNSResponder", Status: 0x26015b11},
	})

	assert.NotContains(t, labels, "process:uid:login")
	assert.Equal(t, []string{"true"}, labels["process:signature:platform"])
	assert.NotContains(t, labels, "process:signature:team")
	assert.NotContains(t, labels, "process:bundle:path")
	assert.NotContains(t, labels, "process:responsible:pid")
}

func TestCollector_ReplacedBinaryFallsBackToTheExecPath(t *testing.T) {
	t.Parallel()

	labels := collect(t, macosprocess.ProcessInfo{
		ExecPath:  "/opt/claude/claude",
		Signature: macosprocess.Signature{Identifier: "com.anthropic.claude-code", Team: "Q6L2SF6YDW", Status: 0x22010301},
	})

	assert.Equal(t, []string{"/opt/claude/claude"}, labels["process:binary:path"])
	assert.Equal(t, []string{"Q6L2SF6YDW"}, labels["process:signature:team"])
}

func TestCollector_NoAuditTokenNoLoginOrSession(t *testing.T) {
	t.Parallel()

	labels := collect(t, macosprocess.ProcessInfo{Path: "/bin/sh", AUID: 501})

	assert.NotContains(t, labels, "process:uid:login")
	assert.NotContains(t, labels, "process:session")
	assert.NotContains(t, labels, "process:signature:identifier")
}

func TestCollector_ProcessGone(t *testing.T) {
	t.Parallel()

	c := macosprocess.New(macosprocess.CollectorWithProcessInfoFunc(func(context.Context, int32) (macosprocess.ProcessInfo, error) {
		return macosprocess.ProcessInfo{}, macosprocess.ErrProcessNotFound
	}))

	_, err := c.GetMetadata(metadatax.ContextWithPID(context.Background(), 1))
	require.ErrorIs(t, err, macosprocess.ErrProcessNotFound)

	md, err := macosprocess.New(
		macosprocess.CollectorWithProcessInfoFunc(func(context.Context, int32) (macosprocess.ProcessInfo, error) {
			return macosprocess.ProcessInfo{}, macosprocess.ErrProcessNotFound
		}),
		macosprocess.WithSkipOnSoftError(),
	).GetMetadata(metadatax.ContextWithPID(context.Background(), 1))
	require.NoError(t, err)
	assert.Empty(t, md.GetLabels())
}

func TestCollector_LongCommandLine(t *testing.T) {
	t.Parallel()

	args := []string{"/Applications/Slack.app/Contents/Frameworks/Slack Helper.app/Contents/MacOS/Slack Helper"}
	for i := range 300 {
		args = append(args, "--switch-"+strconv.Itoa(i)+"=café")
	}

	labels := collect(t, macosprocess.ProcessInfo{Args: args})

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

func TestCollector_CommandLineLimits(t *testing.T) {
	t.Parallel()

	p := macosprocess.ProcessInfo{Args: []string{"tool", "--password", "correct horse battery staple", "--user", "bob"}}

	labels := collect(t, p, macosprocess.CollectorWithMaxArgs(2))
	assert.Equal(t, []string{"tool --password"}, labels["process:cmdline"])
	assert.NotContains(t, labels, "process:arg:2")

	labels = collect(t, p, macosprocess.CollectorWithMaxCmdlineLen(20))
	assert.Equal(t, []string{"tool --password corr"}, labels["process:cmdline"])
	assert.Equal(t, []string{"corr"}, labels["process:arg:2"])
	assert.NotContains(t, labels, "process:arg:3")
}

func TestCollector_ArgumentsKeepTheirSpaces(t *testing.T) {
	t.Parallel()

	labels := collect(t, macosprocess.ProcessInfo{Args: []string{"tool", "--password", "correct horse battery staple"}})

	assert.Equal(t, []string{"tool --password correct horse battery staple"}, labels["process:cmdline"])
	assert.Equal(t, []string{"correct horse battery staple"}, labels["process:arg:2"])
}

func TestCollector_ArgumentLongerThanTheCommandLine(t *testing.T) {
	t.Parallel()

	args := []string{"/usr/local/bin/node", "-e", strings.Repeat("é", 1000)}

	labels := collect(t, macosprocess.ProcessInfo{Args: args})

	cmdline := labels["process:cmdline"][0]
	assert.LessOrEqual(t, len(cmdline), 1024)
	assert.True(t, utf8.ValidString(cmdline))

	last := labels["process:arg:2"][0]
	assert.True(t, utf8.ValidString(last))
	assert.True(t, strings.HasPrefix(args[2], last))
	assert.Equal(t, "/usr/local/bin/node -e "+last, cmdline)
	assert.NotContains(t, labels, "process:arg:3")
}

func TestCollector_NoPID(t *testing.T) {
	t.Parallel()

	_, err := macosprocess.New().GetMetadata(context.Background())
	require.ErrorIs(t, err, metadatax.ErrPIDNotFound)
}
