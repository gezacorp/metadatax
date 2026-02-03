package kubernetes_test

import (
	"context"
	_ "embed"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"

	"github.com/gezacorp/metadatax"
	"github.com/gezacorp/metadatax/collectors/kubernetes"
)

type kubeletClient struct{}

//go:embed testdata/pods.json
var testPodsJSON []byte

func (c *kubeletClient) GetPods(ctx context.Context) ([]corev1.Pod, error) {
	var pods corev1.PodList
	if err := json.Unmarshal(testPodsJSON, &pods); err != nil {
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
		"kubernetes:pod:init-image:count":                   {"1"},
		"kubernetes:pod:init-image:name":                    {"golang:1.24.0-alpine"},
		"kubernetes:pod:name":                               {"metrics-server-648b5df564-drsb2"},
		"kubernetes:pod:namespace":                          {"kube-system"},
		"kubernetes:pod:owner:name":                         {"metrics-server-648b5df564"},
		"kubernetes:pod:owner:kind":                         {"replicaset"},
		"kubernetes:pod:owner:kind-with-version":            {"apps/v1/replicaset"},
		"kubernetes:pod:serviceaccount":                     {"metrics-server"},
	}

	collector := kubernetes.New(
		kubernetes.WithPodLister(&kubeletClient{}),
		kubernetes.WithPodResolver(&podResolver{}),
	)

	md, err := collector.GetMetadata(metadatax.ContextWithPID(context.Background(), 1))
	assert.Nil(t, err)

	assert.Equal(t, expectedLabels, map[string][]string(md.GetLabels()))
}

type initContainerPodResolver struct{}

func (r *initContainerPodResolver) GetPodAndContainerID(pid int32) (string, string, error) {
	return "5831c41b-55ba-4e82-9c6e-2d3ad9d8bfe9", "fa7b84119285652b6a5391a67629f5c116ccb042e0cacc6605d95dd139360fa4", nil
}

type failedContainerPodResolver struct{}

func (r *failedContainerPodResolver) GetPodAndContainerID(pid int32) (string, string, error) {
	return "83cf03c7-a39a-482a-8b8a-fe3cf1b09e48", "1598284aa5aa67d2ea9c8229b42a0d524136b029532e6729f95f3d1ef42984f5", nil
}

func TestGetMetadataForInitContainer(t *testing.T) {
	t.Parallel()

	expectedLabels := map[string][]string{
		"kubernetes:annotation:kubernetes.io/config.seen":   {"2023-11-23T16:37:13.953323037Z"},
		"kubernetes:annotation:kubernetes.io/config.source": {"api"},
		"kubernetes:container:image:id":                     {"docker.io/library/golang@sha256:2d40d4fc278dad38be0777d5e2a88a2c6dee51b0b29c97a764fc6c6a11ca893c"},
		"kubernetes:container:name":                         {"alpine"},
		"kubernetes:label:k8s-app":                          {"metrics-server"},
		"kubernetes:label:pod-template-hash":                {"648b5df564"},
		"kubernetes:node:name":                              {"lima-k3s"},
		"kubernetes:pod:ephemeral-image:count":              {"0"},
		"kubernetes:pod:image:count":                        {"1"},
		"kubernetes:pod:image:id":                           {"docker.io/rancher/mirrored-metrics-server@sha256:c2dfd72bafd6406ed306d9fbd07f55c496b004293d13d3de88a4567eacc36558"},
		"kubernetes:pod:image:name":                         {"rancher/mirrored-metrics-server:v0.6.3"},
		"kubernetes:pod:init-image:count":                   {"1"},
		"kubernetes:pod:init-image:name":                    {"golang:1.24.0-alpine"},
		"kubernetes:pod:name":                               {"metrics-server-648b5df564-drsb2"},
		"kubernetes:pod:namespace":                          {"kube-system"},
		"kubernetes:pod:owner:name":                         {"metrics-server-648b5df564"},
		"kubernetes:pod:owner:kind":                         {"replicaset"},
		"kubernetes:pod:owner:kind-with-version":            {"apps/v1/replicaset"},
		"kubernetes:pod:serviceaccount":                     {"metrics-server"},
	}

	collector := kubernetes.New(
		kubernetes.WithPodLister(&kubeletClient{}),
		kubernetes.WithPodResolver(&initContainerPodResolver{}),
	)

	md, err := collector.GetMetadata(metadatax.ContextWithPID(context.Background(), 1))
	assert.Nil(t, err)

	assert.Equal(t, expectedLabels, map[string][]string(md.GetLabels()))
}

func TestGetMetadataForFailedContainer(t *testing.T) {
	t.Parallel()

	collector := kubernetes.New(
		kubernetes.WithPodLister(&kubeletClient{}),
		kubernetes.WithPodResolver(&failedContainerPodResolver{}),
	)

	_, err := collector.GetMetadata(metadatax.ContextWithPID(context.Background(), 1))
	assert.EqualErrorf(t, err, "could not get pod context after timeout: pod context not found", "error message %s")

}
