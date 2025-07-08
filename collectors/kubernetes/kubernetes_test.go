package kubernetes_test

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"

	"github.com/gezacorp/metadatax"
	"github.com/gezacorp/metadatax/collectors/kubernetes"
)

type kubeletClient struct{}

func (c *kubeletClient) GetPods(ctx context.Context) ([]corev1.Pod, error) {
	file, err := os.Open("testdata/pods.json")
	if err != nil {
		return nil, err
	}

	content, err := io.ReadAll(file)
	if err != nil {
		return nil, err
	}

	var pods corev1.PodList
	if err := json.Unmarshal(content, &pods); err != nil {
		return nil, err
	}

	return pods.Items, nil
}

type podResolver struct{}

func (r *podResolver) GetPodAndContainerID(pid int32) (string, string, error) {
	return "5831c41b-55ba-4e82-9c6e-2d3ad9d8bfe9", "2ce296b740c37b0793e7c95761b32f6a26d8b98b3c0e4e7d5a6032f71520ecad", nil
}

func TestGetMetadata(t *testing.T) {
	t.Parallel()

	expectedLabels := map[string][]string{
		"kubernetes:annotation:kubernetes.io/config.seen":   {"2023-11-23T16:37:13.953323037Z"},
		"kubernetes:annotation:kubernetes.io/config.source": {"api"},
		"kubernetes:container:image:id":                     {"docker.io/rancher/mirrored-metrics-server@sha256:c2dfd72bafd6406ed306d9fbd07f55c496b004293d13d3de88a4567eacc36558"},
		"kubernetes:container:name":                         {"metrics-server"},
		"kubernetes:label:k8s-app":                          {"metrics-server"},
		"kubernetes:label:pod-template-hash":                {"648b5df564"},
		"kubernetes:node:name":                              {"lima-k3s"},
		"kubernetes:pod:ephemeral-image:count":              {"0"},
		"kubernetes:pod:image:count":                        {"1"},
		"kubernetes:pod:image:id":                           {"docker.io/rancher/mirrored-metrics-server@sha256:c2dfd72bafd6406ed306d9fbd07f55c496b004293d13d3de88a4567eacc36558"},
		"kubernetes:pod:image:name":                         {"rancher/mirrored-metrics-server:v0.6.3"},
		"kubernetes:pod:init-image:count":                   {"0"},
		"kubernetes:pod:name":                               {"metrics-server-648b5df564-drsb2"},
		"kubernetes:pod:namespace":                          {"kube-system"},
		"kubernetes:pod:owner:name":                         {"metrics-server-648b5df564"},
		"kubernetes:pod:owner:kind":                         {"replicaset"},
		"kubernetes:pod:owner:kind-with-version":            {"apps/v1/replicaset"},
		"kubernetes:pod:serviceaccount":                     {"metrics-server"},
	}

	collector := kubernetes.New(
		kubernetes.WithPodsGetter(&kubeletClient{}),
		kubernetes.WithPodResolver(&podResolver{}),
	)

	md, err := collector.GetMetadata(metadatax.ContextWithPID(context.Background(), 1))
	assert.Nil(t, err)

	assert.Equal(t, expectedLabels, map[string][]string(md.GetLabels()))
}
