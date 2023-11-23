package docker_test

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"testing"

	"github.com/docker/docker/api/types"
	"github.com/stretchr/testify/assert"

	"github.com/gezacorp/metadatax"
	"github.com/gezacorp/metadatax/collectors/docker"
)

type containerIDGetter struct{}

func (g *containerIDGetter) GetContainerIDFromPID(pid int) (string, error) {
	return "test", nil
}

type metadataGetter struct{}

func (g *metadataGetter) ContainerInspect(ctx context.Context, containerID string) (types.ContainerJSON, error) {
	file, err := os.Open("testdata/container.json")
	if err != nil {
		return types.ContainerJSON{}, err
	}
	content, err := io.ReadAll(file)
	if err != nil {
		return types.ContainerJSON{}, err
	}

	var containerJSON types.ContainerJSON
	if err := json.Unmarshal(content, &containerJSON); err != nil {
		return types.ContainerJSON{}, err
	}

	return containerJSON, nil
}

func TestGetMetadata(t *testing.T) {
	t.Parallel()

	collector, err := docker.New(
		docker.WithMetadataGetter(&metadataGetter{}),
		docker.WithContainerIDGetter(&containerIDGetter{}),
	)
	assert.Nil(t, err)

	expectedLabels := map[string][]string{
		"docker:cmdline":           {"/docker-entrypoint.sh nginx -g daemon off;"},
		"docker:env:NGINX_VERSION": {"1.25.3"},
		"docker:env:NJS_VERSION":   {"0.8.2"},
		"docker:env:PATH":          {"/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"},
		"docker:env:PKG_RELEASE":   {"1~bookworm"},
		"docker:id":                {"3ac7ed50c6087bb468fd70d37a6e3ee8d5b554bcbde20bd83f9a9dfa14f0431e"},
		"docker:image:hash":        {"sha256:c20060033e06f882b0fbe2db7d974d72e0887a3be5e554efdb0dcf8d53512647"},
		"docker:image:name":        {"nginx"},
		"docker:label:maintainer":  {"NGINX Docker Maintainers <docker-maint@nginx.com>"},
		"docker:name":              {"awesome_sinoussi"},
		"docker:network:hostname":  {"3ac7ed50c608"},
		"docker:network:mode":      {"default"},
		"docker:port-binding":      {"8080/tcp"},
	}

	md, err := collector.GetMetadata(metadatax.ContextWithPID(context.Background(), 1))
	assert.Nil(t, err)

	assert.Equal(t, expectedLabels, map[string][]string(md.GetLabels()))
}
