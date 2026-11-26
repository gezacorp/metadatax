package macos_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gezacorp/metadatax/collectors/macos"
)

var errExit = errors.New("exit status 1")

type commandOutput struct {
	file   string
	output string
	err    error
}

func commandRunner(t *testing.T, outputs map[string]commandOutput) macos.CommandRunner {
	t.Helper()

	return func(_ context.Context, name string, args ...string) ([]byte, error) {
		cmd := strings.Join(append([]string{name}, args...), " ")
		for prefix, out := range outputs {
			if !strings.HasPrefix(cmd, prefix) {
				continue
			}

			if out.file != "" {
				content, err := os.ReadFile("testdata/" + out.file)
				require.NoError(t, err)

				return content, out.err
			}

			return []byte(out.output), out.err
		}

		return nil, errors.New("command not found: " + name)
	}
}

func sysctls(values map[string]string) macos.GetSysctlFunc {
	return func(name string) (string, error) {
		if v, ok := values[name]; ok {
			return v, nil
		}

		return "", errors.New("unknown oid")
	}
}

func sysctlUint32s(values map[string]uint32) macos.GetSysctlUint32Func {
	return func(name string) (uint32, error) {
		if v, ok := values[name]; ok {
			return v, nil
		}

		return 0, errors.New("unknown oid")
	}
}

func TestGetMetadataMacBook(t *testing.T) {
	t.Parallel()

	expectedLabels := map[string][]string{
		"macos:build":                      {"26A428"},
		"macos:computer-name":              {"Test MacBook Air"},
		"macos:kernel:version":             {"Darwin Kernel Version 27.0.0: Tue Aug 11 21:05:41 PDT 2026; root:xnu-13432.1.9~1/RELEASE_ARM64_T8122"},
		"macos:hardware:serial":            {"C02TEST0MBA"},
		"macos:hardware:provisioning-udid": {"00008122-0000AAAA0000BBBB"},
		"macos:hardware:virtual":           {"false"},
		"macos:hardware:model:name":        {"MacBook Air"},
		"macos:hardware:model:identifier":  {"Mac15,12"},
		"macos:hardware:model:number":      {"Z1G80019MMG/A"},
		"macos:hardware:cpu:brand":         {"Apple M3"},
		"macos:device:type":                {"laptop"},
		"macos:mdm:enrolled":               {"true"},
		"macos:mdm:dep":                    {"false"},
		"macos:mdm:server":                 {"mdm.example.com"},
	}

	collector := macos.New(
		macos.CollectorWithForceDarwin(),
		macos.CollectorWithGetSysctlFunc(sysctls(map[string]string{
			"kern.osversion":           "26A428",
			"kern.version":             "Darwin Kernel Version 27.0.0: Tue Aug 11 21:05:41 PDT 2026; root:xnu-13432.1.9~1/RELEASE_ARM64_T8122",
			"machdep.cpu.brand_string": "Apple M3",
		})),
		macos.CollectorWithGetSysctlUint32Func(sysctlUint32s(map[string]uint32{
			"kern.hv_vmm_present": 0,
		})),
		macos.CollectorWithCommandRunner(commandRunner(t, map[string]commandOutput{
			"system_profiler": {file: "system_profiler_macbook.json"},
			"profiles":        {file: "profiles_enrollment.txt"},
		})),
	)

	md, err := collector.GetMetadata(context.Background())
	require.NoError(t, err)
	assert.Equal(t, expectedLabels, map[string][]string(md.GetLabels()))
}

func TestGetMetadataIntelIMac(t *testing.T) {
	t.Parallel()

	expectedLabels := map[string][]string{
		"macos:computer-name":             {"Test iMac"},
		"macos:hardware:serial":           {"C02TESTIMAC"},
		"macos:hardware:virtual":          {"false"},
		"macos:hardware:model:name":       {"iMac"},
		"macos:hardware:model:identifier": {"iMac20,1"},
		"macos:hardware:model:number":     {"MXWV2LL/A"},
		"macos:device:type":               {"desktop"},
	}

	collector := macos.New(
		macos.CollectorWithForceDarwin(),
		macos.CollectorWithGetSysctlFunc(sysctls(map[string]string{})),
		macos.CollectorWithGetSysctlUint32Func(sysctlUint32s(map[string]uint32{
			"kern.hv_vmm_present": 0,
		})),
		macos.CollectorWithCommandRunner(commandRunner(t, map[string]commandOutput{
			"system_profiler": {file: "system_profiler_imac.json"},
			"profiles":        {err: errExit},
		})),
	)

	md, err := collector.GetMetadata(context.Background())
	require.NoError(t, err)
	assert.Equal(t, expectedLabels, map[string][]string(md.GetLabels()))
}

func TestGetMetadataVirtualizationUnknown(t *testing.T) {
	t.Parallel()

	collector := macos.New(
		macos.CollectorWithForceDarwin(),
		macos.CollectorWithGetSysctlFunc(sysctls(map[string]string{})),
		macos.CollectorWithGetSysctlUint32Func(sysctlUint32s(map[string]uint32{})),
		macos.CollectorWithCommandRunner(commandRunner(t, map[string]commandOutput{
			"system_profiler": {file: "system_profiler_macbook.json"},
			"profiles":        {err: errExit},
		})),
	)

	md, err := collector.GetMetadata(context.Background())
	require.NoError(t, err)

	_, ok := md.GetLabelValues("macos:hardware:virtual")
	assert.False(t, ok)
	assert.Equal(t, "unknown", md.GetLabelValue("macos:device:type"))
}

func TestGetMetadataMDM(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		output   commandOutput
		expected map[string][]string
	}{
		"enrolled": {
			output: commandOutput{file: "profiles_enrollment.txt"},
			expected: map[string][]string{
				"macos:mdm:enrolled": {"true"},
				"macos:mdm:dep":      {"false"},
				"macos:mdm:server":   {"mdm.example.com"},
			},
		},
		"output with non-zero exit status": {
			output: commandOutput{output: "Enrolled via DEP: Yes\nMDM enrollment: Yes\nMDM server: https://mdm.example.com/mdm\n", err: errExit},
			expected: map[string][]string{
				"macos:mdm:enrolled": {"true"},
				"macos:mdm:dep":      {"true"},
				"macos:mdm:server":   {"mdm.example.com"},
			},
		},
		"server without scheme": {
			output: commandOutput{output: "MDM enrollment: Yes\nMDM server: mdm.example.com:443/mdm\n"},
			expected: map[string][]string{
				"macos:mdm:enrolled": {"true"},
				"macos:mdm:dep":      {"false"},
				"macos:mdm:server":   {"mdm.example.com"},
			},
		},
		"not enrolled": {
			output: commandOutput{output: "Enrolled via DEP: No\nMDM enrollment: No\n"},
			expected: map[string][]string{
				"macos:mdm:enrolled": {"false"},
				"macos:mdm:dep":      {"false"},
			},
		},
		"no output": {
			output:   commandOutput{err: errExit},
			expected: map[string][]string{},
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			collector := macos.New(
				macos.CollectorWithForceDarwin(),
				macos.CollectorWithGetSysctlFunc(sysctls(map[string]string{})),
				macos.CollectorWithGetSysctlUint32Func(sysctlUint32s(map[string]uint32{})),
				macos.CollectorWithCommandRunner(commandRunner(t, map[string]commandOutput{
					"system_profiler": {file: "system_profiler_macbook.json"},
					"profiles":        tt.output,
				})),
			)

			md, err := collector.GetMetadata(context.Background())
			require.NoError(t, err)

			labels := map[string][]string{}
			for k, v := range md.GetLabels() {
				if strings.HasPrefix(k, "macos:mdm:") {
					labels[k] = v
				}
			}
			assert.Equal(t, tt.expected, labels)
		})
	}
}

func TestGetMetadataCommandTimeout(t *testing.T) {
	t.Parallel()

	collector := macos.New(
		macos.CollectorWithForceDarwin(),
		macos.CollectorWithCommandTimeout(10*time.Millisecond),
		macos.CollectorWithCommandRunner(func(ctx context.Context, _ string, _ ...string) ([]byte, error) {
			<-ctx.Done()

			return nil, ctx.Err()
		}),
	)

	_, err := collector.GetMetadata(context.Background())
	assert.ErrorIs(t, err, context.DeadlineExceeded)
}

func TestGetMetadataSystemProfilerFailure(t *testing.T) {
	t.Parallel()

	tests := map[string]commandOutput{
		"command failure":  {err: errExit},
		"invalid output":   {output: "not json"},
		"missing hardware": {output: `{"SPSoftwareDataType": [{"local_host_name": "Test"}]}`},
	}

	for name, output := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			collector := macos.New(
				macos.CollectorWithForceDarwin(),
				macos.CollectorWithCommandRunner(commandRunner(t, map[string]commandOutput{
					"system_profiler": output,
				})),
			)

			_, err := collector.GetMetadata(context.Background())
			assert.Error(t, err)
		})
	}
}

func TestGetMetadataNotDarwin(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == "darwin" {
		t.Skip("collector is active on darwin")
	}

	collector := macos.New(
		macos.CollectorWithCommandRunner(func(context.Context, string, ...string) ([]byte, error) {
			return nil, errors.New("should not be called")
		}),
	)

	md, err := collector.GetMetadata(context.Background())
	require.NoError(t, err)
	assert.Empty(t, md.GetLabels())
}

func TestDetectDeviceType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		modelName       string
		modelIdentifier string
		isVirtual       bool
		expected        macos.DeviceType
	}{
		{name: "apple silicon macbook air", modelName: "MacBook Air", modelIdentifier: "Mac15,12", expected: macos.DeviceTypeLaptop},
		{name: "apple silicon macbook pro", modelName: "MacBook Pro", modelIdentifier: "Mac15,6", expected: macos.DeviceTypeLaptop},
		{name: "apple silicon mac mini", modelName: "Mac mini", modelIdentifier: "Mac16,10", expected: macos.DeviceTypeMini},
		{name: "apple silicon mac studio", modelName: "Mac Studio", modelIdentifier: "Mac15,14", expected: macos.DeviceTypeDesktop},
		{name: "apple silicon imac", modelName: "iMac", modelIdentifier: "Mac15,4", expected: macos.DeviceTypeDesktop},
		{name: "apple silicon mac pro", modelName: "Mac Pro", modelIdentifier: "Mac14,8", expected: macos.DeviceTypeDesktop},
		{name: "intel macbook pro by identifier", modelIdentifier: "MacBookPro16,1", expected: macos.DeviceTypeLaptop},
		{name: "intel mac mini by identifier", modelIdentifier: "Macmini8,1", expected: macos.DeviceTypeMini},
		{name: "intel imac pro by identifier", modelIdentifier: "iMacPro1,1", expected: macos.DeviceTypeDesktop},
		{name: "intel mac pro by identifier", modelIdentifier: "MacPro7,1", expected: macos.DeviceTypeDesktop},
		{name: "unknown", modelIdentifier: "Mac99,1", expected: macos.DeviceTypeUnknown},
		{name: "virtualization framework vm", modelName: "Apple Virtual Machine 1", modelIdentifier: "VirtualMac2,1", expected: macos.DeviceTypeVirtual},
		{name: "hypervisor present", modelName: "MacBook Pro", modelIdentifier: "MacBookPro16,1", isVirtual: true, expected: macos.DeviceTypeVirtual},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.expected, macos.DetectDeviceType(tt.modelName, tt.modelIdentifier, tt.isVirtual))
		})
	}
}

func plistWith(body string) string {
	return `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>` + body + `</dict></plist>`
}

const enrolledOutput = "Enrolled via DEP: No\nMDM enrollment: Yes\nMDM server: https://mdm.example.com/mdm\n"

type managedPreference struct {
	domain, key, label string
}

// managedPreferenceLabels collects with the given preferences and enrollment
// and returns the managed preference labels.
func managedPreferenceLabels(t *testing.T, dir string, enrollment commandOutput, prefs ...managedPreference) map[string][]string {
	t.Helper()

	opts := []macos.CollectorOption{
		macos.CollectorWithForceDarwin(),
		macos.CollectorWithGetSysctlFunc(sysctls(map[string]string{})),
		macos.CollectorWithGetSysctlUint32Func(sysctlUint32s(map[string]uint32{})),
		macos.CollectorWithCommandRunner(commandRunner(t, map[string]commandOutput{
			"system_profiler": {file: "system_profiler_macbook.json"},
			"profiles":        enrollment,
		})),
		macos.CollectorWithManagedPreferencesDir(dir),
	}
	for _, p := range prefs {
		opts = append(opts, macos.CollectorWithManagedPreference(p.domain, p.key, p.label))
	}

	md, err := macos.New(opts...).GetMetadata(context.Background())
	require.NoError(t, err)

	labels := map[string][]string{}
	for k, v := range md.GetLabels() {
		if strings.HasPrefix(k, "macos:mdm:preference") {
			labels[k] = v
		}
	}

	return labels
}

func TestGetMetadataManagedPreference(t *testing.T) {
	t.Parallel()

	installOnLaunch := managedPreference{domain: "com.example.app", key: "InstallOnLaunch", label: "app-install"}

	tests := map[string]struct {
		content    *string
		setup      func(t *testing.T, dir string)
		enrollment commandOutput
		prefs      []managedPreference
		expected   map[string][]string
	}{
		"forced on": {
			content:  new(plistWith(`<key>InstallOnLaunch</key><true/>`)),
			expected: map[string][]string{"macos:mdm:preference:app-install": {"true"}},
		},
		"not enrolled": {
			content:    new(plistWith(`<key>InstallOnLaunch</key><true/>`)),
			enrollment: commandOutput{output: "Enrolled via DEP: No\nMDM enrollment: No\n"},
			expected:   map[string][]string{},
		},
		"enrollment unknown": {
			content:    new(plistWith(`<key>InstallOnLaunch</key><true/>`)),
			enrollment: commandOutput{err: errExit},
			expected:   map[string][]string{},
		},
		"forced off": {
			content:  new(plistWith(`<key>InstallOnLaunch</key><false/>`)),
			expected: map[string][]string{},
		},
		"not a boolean": {
			content:  new(plistWith(`<key>InstallOnLaunch</key><string>true</string>`)),
			expected: map[string][]string{},
		},
		"missing key": {
			content:  new(plistWith(`<key>Other</key><true/>`)),
			expected: map[string][]string{},
		},
		"invalid plist": {
			content:  new("not a plist"),
			expected: map[string][]string{},
		},
		"too large": {
			content:  new(plistWith(`<key>InstallOnLaunch</key><true/><key>Padding</key><string>` + strings.Repeat("x", 1<<20) + `</string>`)),
			expected: map[string][]string{},
		},
		"missing file": {
			expected: map[string][]string{},
		},
		"several keys of one domain": {
			content: new(plistWith(`<key>InstallOnLaunch</key><true/><key>Other</key><true/><key>Off</key><false/>`)),
			prefs: []managedPreference{
				installOnLaunch,
				{domain: "com.example.app", key: "Other", label: "other"},
				{domain: "com.example.app", key: "Off", label: "off"},
			},
			expected: map[string][]string{
				"macos:mdm:preference:app-install": {"true"},
				"macos:mdm:preference:other":       {"true"},
			},
		},
		"empty label": {
			content:  new(plistWith(`<key>InstallOnLaunch</key><true/>`)),
			prefs:    []managedPreference{{domain: "com.example.app", key: "InstallOnLaunch"}},
			expected: map[string][]string{},
		},
		"empty domain": {
			setup: func(t *testing.T, dir string) {
				t.Helper()
				require.NoError(t, os.WriteFile(filepath.Join(dir, ".plist"), []byte(plistWith(`<key>InstallOnLaunch</key><true/>`)), 0o600))
			},
			prefs:    []managedPreference{{key: "InstallOnLaunch", label: "app-install"}},
			expected: map[string][]string{},
		},
		"domain with a path separator": {
			setup: func(t *testing.T, dir string) {
				t.Helper()
				require.NoError(t, os.Mkdir(filepath.Join(dir, "sub"), 0o700))
				require.NoError(t, os.WriteFile(filepath.Join(dir, "sub", "com.example.app.plist"), []byte(plistWith(`<key>InstallOnLaunch</key><true/>`)), 0o600))
			},
			prefs:    []managedPreference{{domain: "sub/com.example.app", key: "InstallOnLaunch", label: "app-install"}},
			expected: map[string][]string{},
		},
		"symlink": {
			setup: func(t *testing.T, dir string) {
				t.Helper()
				target := filepath.Join(t.TempDir(), "target.plist")
				require.NoError(t, os.WriteFile(target, []byte(plistWith(`<key>InstallOnLaunch</key><true/>`)), 0o600))
				require.NoError(t, os.Symlink(target, filepath.Join(dir, "com.example.app.plist")))
			},
			expected: map[string][]string{},
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			if tt.content != nil {
				require.NoError(t, os.WriteFile(filepath.Join(dir, "com.example.app.plist"), []byte(*tt.content), 0o600))
			}
			if tt.setup != nil {
				tt.setup(t, dir)
			}

			enrollment := tt.enrollment
			if enrollment == (commandOutput{}) {
				enrollment = commandOutput{output: enrolledOutput}
			}

			prefs := tt.prefs
			if prefs == nil {
				prefs = []managedPreference{installOnLaunch}
			}

			assert.Equal(t, tt.expected, managedPreferenceLabels(t, dir, enrollment, prefs...))
		})
	}
}
