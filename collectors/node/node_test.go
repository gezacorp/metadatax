package node_test

import (
	"context"
	"testing"

	"github.com/shirou/gopsutil/v3/host"
	"github.com/stretchr/testify/assert"

	"github.com/gezacorp/metadatax"
	"github.com/gezacorp/metadatax/collectors/node"
)

func TestGetMetadata(t *testing.T) {
	t.Parallel()

	expectedLabels := map[string][]string{
		"node:hostname":            {"hostname"},
		"node:kernel:arch":         {"aarch64"},
		"node:kernel:version":      {"6.8.0-63-generic"},
		"node:os:type":             {"linux"},
		"node:os:version":          {"ubuntu-24.04"},
		"node:platform:family":     {"debian"},
		"node:platform:name":       {"ubuntu"},
		"node:platform:version":    {"24.04"},
		"node:uuid":                {"9c4f3853-4193-4700-ae6e-f23c61fd120c"},
		"node:virtualization:type": {"xen"},
		"node:virtualization:role": {"guest"},
	}

	collector := node.New(
		node.CollectorWithGetHostMetadataFunc(func(context.Context) (*host.InfoStat, error) {
			return &host.InfoStat{
				Hostname:             "hostname",
				OS:                   "linux",
				Platform:             "ubuntu",
				PlatformFamily:       "debian",
				PlatformVersion:      "24.04",
				HostID:               "9c4f3853-4193-4700-ae6e-f23c61fd120c",
				KernelVersion:        "6.8.0-63-generic",
				KernelArch:           "aarch64",
				VirtualizationSystem: "xen",
				VirtualizationRole:   "guest",
			}, nil
		}),
		node.CollectorWithMetadataContainerInitFunc(func() metadatax.MetadataContainer {
			return metadatax.New(
				metadatax.WithPrefix("node"),
			)
		}),
	)

	md, err := collector.GetMetadata(context.Background())
	assert.Nil(t, err)
	assert.Equal(t, expectedLabels, map[string][]string(md.GetLabels()))
}
