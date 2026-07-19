package systemd_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/gezacorp/metadatax"
	"github.com/gezacorp/metadatax/collectors/systemd"
)

type unitNameGetter struct {
	unitName string
	err      error
}

func (g *unitNameGetter) GetUnitNameForPID(pid int) (string, error) {
	return g.unitName, g.err
}

type unitPropertiesGetter struct {
	properties map[string]any
	err        error
}

func (g *unitPropertiesGetter) GetUnitProperties(ctx context.Context, unitName string) (map[string]any, error) {
	return g.properties, g.err
}

func TestGetMetadata(t *testing.T) {
	t.Parallel()

	collector := systemd.New(
		systemd.WithForceHasSystemd(),
		systemd.CollectorWithExtractENVs(),
		systemd.WithUnitNameGetter(&unitNameGetter{unitName: "nginx.service"}),
		systemd.WithUnitPropertiesGetter(&unitPropertiesGetter{
			properties: map[string]any{
				"Description":            "The nginx HTTP and reverse proxy server",
				"Type":                   "notify",
				"LoadState":              "loaded",
				"ActiveState":            "active",
				"SubState":               "running",
				"UnitFileState":          "enabled",
				"Result":                 "success",
				"FragmentPath":           "/usr/lib/systemd/system/nginx.service",
				"Slice":                  "system.slice",
				"InvocationID":           []byte{0xde, 0xad, 0xbe, 0xef},
				"MainPID":                uint32(4321),
				"NRestarts":              uint32(2),
				"ExecMainStartTimestamp": uint64(1717200000000000),
				"Environment":            []string{"NGINX_VERSION=1.25.3", "PATH=/usr/local/sbin"},
				"User":                   "www-data",
				"Group":                  "www-data",
				"DynamicUser":            false,
				"CapabilityBoundingSet":  uint64(0x3fffffffff),
				"ExecStart": []any{
					[]any{
						"/usr/sbin/nginx",
						[]string{"/usr/sbin/nginx", "-g", "daemon on;"},
					},
				},
			},
		}),
	)

	expectedLabels := map[string][]string{
		"systemd:unit":                    {"nginx.service"},
		"systemd:description":             {"The nginx HTTP and reverse proxy server"},
		"systemd:type":                    {"notify"},
		"systemd:load-state":              {"loaded"},
		"systemd:active-state":            {"active"},
		"systemd:sub-state":               {"running"},
		"systemd:unit-file-state":         {"enabled"},
		"systemd:result":                  {"success"},
		"systemd:fragment-path":           {"/usr/lib/systemd/system/nginx.service"},
		"systemd:slice":                   {"system.slice"},
		"systemd:invocation-id":           {"deadbeef"},
		"systemd:main-pid":                {"4321"},
		"systemd:restarts":                {"2"},
		"systemd:started-at":              {"2024-06-01T00:00:00Z"},
		"systemd:env:NGINX_VERSION":       {"1.25.3"},
		"systemd:env:PATH":                {"/usr/local/sbin"},
		"systemd:user":                    {"www-data"},
		"systemd:group":                   {"www-data"},
		"systemd:dynamic-user":            {"false"},
		"systemd:capability-bounding-set": {"0x3fffffffff"},
		"systemd:exec-start":              {"/usr/sbin/nginx -g daemon on;"},
	}

	ctx := metadatax.ContextWithPID(context.Background(), 1234)

	md, err := collector.GetMetadata(ctx)
	assert.NoError(t, err)
	assert.Equal(t, expectedLabels, map[string][]string(md.GetLabels()))
}

func TestGetMetadataNoUnit(t *testing.T) {
	t.Parallel()

	collector := systemd.New(
		systemd.WithForceHasSystemd(),
		systemd.WithUnitNameGetter(&unitNameGetter{unitName: ""}),
		systemd.WithSkipOnSoftError(),
	)

	ctx := metadatax.ContextWithPID(context.Background(), 1234)

	md, err := collector.GetMetadata(ctx)
	assert.NoError(t, err)
	assert.Empty(t, md.GetLabels())
}

func TestGetMetadataEnvNormalizesCaseAndMergesUnderOneKey(t *testing.T) {
	t.Parallel()

	// Env var names are uppercased so case variants of the same logical
	// variable (e.g. http_proxy/HTTP_PROXY, a common cross-tool
	// compatibility pattern) are reported under one canonical label; if
	// they carry different values, AddLabel's normal multi-value behavior
	// surfaces both rather than silently picking one.
	collector := systemd.New(
		systemd.WithForceHasSystemd(),
		systemd.CollectorWithExtractENVs(),
		systemd.WithUnitNameGetter(&unitNameGetter{unitName: "nginx.service"}),
		systemd.WithUnitPropertiesGetter(&unitPropertiesGetter{
			properties: map[string]any{
				"Environment": []string{"http_proxy=http://proxy1", "HTTP_PROXY=http://proxy2"},
			},
		}),
	)

	ctx := metadatax.ContextWithPID(context.Background(), 1234)

	md, err := collector.GetMetadata(ctx)
	assert.NoError(t, err)

	labels := md.GetLabels()
	assert.ElementsMatch(t, []string{"http://proxy1", "http://proxy2"}, labels["systemd:env:HTTP_PROXY"])
}

func TestGetMetadataNoSystemd(t *testing.T) {
	t.Parallel()

	collector := systemd.New(
		systemd.WithUnitNameGetter(&unitNameGetter{unitName: "nginx.service"}),
	)

	ctx := metadatax.ContextWithPID(context.Background(), 1234)

	md, err := collector.GetMetadata(ctx)
	assert.NoError(t, err)
	assert.Empty(t, md.GetLabels())
}
