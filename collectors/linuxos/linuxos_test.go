package linuxos_test

import (
	"context"
	"testing"

	"github.com/signalfx/golib/metadata/hostmetadata"
	"github.com/stretchr/testify/assert"

	"github.com/gezacorp/metadatax"
	"github.com/gezacorp/metadatax/collectors/linuxos"
)

func TestGetMetadata(t *testing.T) {
	t.Parallel()

	expectedLabels := map[string][]string{
		"linux:name":           {"os-name"},
		"linux:version":        {"linux-version"},
		"linux:kernel:release": {"linux-kernel-release"},
		"linux:kernel:version": {"kernel-version"},
	}

	collector := linuxos.New(
		linuxos.CollectorWithGetHostMetadataFunc(func() (*hostmetadata.OS, error) {
			return &hostmetadata.OS{
				HostOSName:        expectedLabels["linux:name"][0],
				HostLinuxVersion:  expectedLabels["linux:version"][0],
				HostKernelName:    "linux",
				HostKernelRelease: "kernel-release",
				HostKernelVersion: expectedLabels["linux:kernel:version"][0],
			}, nil
		}),
		linuxos.CollectorWithMetadataContainerInitFunc(func() metadatax.MetadataContainer {
			return metadatax.New(
				metadatax.WithPrefix("linux"),
			)
		}),
	)

	md, err := collector.GetMetadata(context.Background())
	assert.Nil(t, err)
	assert.Equal(t, expectedLabels, map[string][]string(md.GetLabels()))
}
