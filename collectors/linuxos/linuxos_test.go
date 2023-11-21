package linuxos_test

import (
	"context"
	"testing"

	"github.com/signalfx/golib/metadata/hostmetadata"
	"github.com/stretchr/testify/assert"

	"github.com/gezacorp/metadatax/collectors/linuxos"
)

func TestGetMetadata(t *testing.T) {
	t.Parallel()

	expectedLabels := map[string][]string{
		"linuxos:name":           {"os-name"},
		"linuxos:version":        {"linux-version"},
		"linuxos:kernel:release": {"linux-kernel-release"},
		"linuxos:kernel:version": {"kernel-version"},
	}

	collector := linuxos.New(linuxos.CollectorWithHostMetadataGetter(func() (*hostmetadata.OS, error) {
		return &hostmetadata.OS{
			HostOSName:        expectedLabels["linuxos:name"][0],
			HostLinuxVersion:  expectedLabels["linuxos:version"][0],
			HostKernelName:    "linux",
			HostKernelRelease: "kernel-release",
			HostKernelVersion: expectedLabels["linuxos:kernel:version"][0],
		}, nil
	}))

	md, err := collector.GetMetadata(context.Background())
	assert.Nil(t, err)
	assert.Equal(t, expectedLabels, map[string][]string(md.GetLabels()))
}
