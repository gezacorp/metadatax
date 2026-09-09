package kubernetes_test

import (
	"context"
	_ "embed"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"

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
		"kubernetes:container:probe:liveness:path":          {"/livez"},
		"kubernetes:container:probe:liveness:port":          {"10250"},
		"kubernetes:container:probe:liveness:port-name":     {"https"},
		"kubernetes:container:probe:liveness:scheme":        {"https"},
		"kubernetes:container:probe:liveness:type":          {"httpget"},
		"kubernetes:container:probe:readiness:path":         {"/readyz"},
		"kubernetes:container:probe:readiness:port":         {"10250"},
		"kubernetes:container:probe:readiness:port-name":    {"https"},
		"kubernetes:container:probe:readiness:scheme":       {"https"},
		"kubernetes:container:probe:readiness:type":         {"httpget"},
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

const (
	probesPodUID         = "e2fca7a4-1d0f-4c6a-9b45-3f18a6c6b0a1"
	probesContainerID    = "8f2d9f3ab0f24c0d9f34c1c2c5e6f7a8b9c0d1e2f3a4b5c6d7e8f9a0b1c2d3e4"
	execProbePodUID      = "b1a7c9d3-6e52-4f18-8a0b-2c4d6e8f0a1b"
	execProbeContainerID = "1a2b3c4d5e6f708192a3b4c5d6e7f8091a2b3c4d5e6f708192a3b4c5d6e7f809"
)

type probesPodLister struct{}

func (l *probesPodLister) GetPods(ctx context.Context) ([]corev1.Pod, error) {
	grpcService := "grpc.health.v1.Health"

	return []corev1.Pod{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "probes",
				Namespace: "default",
				UID:       types.UID(probesPodUID),
			},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{
						Name:  "app",
						Image: "app:latest",
						Ports: []corev1.ContainerPort{
							{
								Name:          "admin",
								ContainerPort: 9090,
							},
						},
						LivenessProbe: &corev1.Probe{
							ProbeHandler: corev1.ProbeHandler{
								TCPSocket: &corev1.TCPSocketAction{
									Port: intstr.FromString("admin"),
									Host: "127.0.0.1",
								},
							},
						},
						ReadinessProbe: &corev1.Probe{
							ProbeHandler: corev1.ProbeHandler{
								GRPC: &corev1.GRPCAction{
									Port:    8080,
									Service: &grpcService,
								},
							},
						},
						StartupProbe: &corev1.Probe{
							ProbeHandler: corev1.ProbeHandler{
								HTTPGet: &corev1.HTTPGetAction{
									Path: "/startup",
									Port: intstr.FromInt(8081),
								},
							},
						},
					},
				},
			},
			Status: corev1.PodStatus{
				ContainerStatuses: []corev1.ContainerStatus{
					{
						Name:        "app",
						ContainerID: "containerd://" + probesContainerID,
					},
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "exec-probes",
				Namespace: "default",
				UID:       types.UID(execProbePodUID),
			},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{
						Name:  "app",
						Image: "app:latest",
						LivenessProbe: &corev1.Probe{
							ProbeHandler: corev1.ProbeHandler{
								Exec: &corev1.ExecAction{
									Command: []string{"sh", "-c", "curl --fail http://localhost:8080/healthz"},
								},
							},
						},
					},
				},
			},
			Status: corev1.PodStatus{
				ContainerStatuses: []corev1.ContainerStatus{
					{
						Name:        "app",
						ContainerID: "containerd://" + execProbeContainerID,
					},
				},
			},
		},
	}, nil
}

type probesPodResolver struct{}

func (r *probesPodResolver) GetPodAndContainerID(pid int32) (string, string, error) {
	return probesPodUID, probesContainerID, nil
}

func TestGetMetadataForProbeTypes(t *testing.T) {
	t.Parallel()

	expectedLabels := map[string][]string{
		"kubernetes:container:name":                     {"app"},
		"kubernetes:container:probe:liveness:type":      {"tcpsocket"},
		"kubernetes:container:probe:liveness:host":      {"127.0.0.1"},
		"kubernetes:container:probe:liveness:port-name": {"admin"},
		"kubernetes:container:probe:liveness:port":      {"9090"},
		"kubernetes:container:probe:readiness:type":     {"grpc"},
		"kubernetes:container:probe:readiness:port":     {"8080"},
		"kubernetes:container:probe:readiness:service":  {"grpc.health.v1.Health"},
		"kubernetes:container:probe:startup:type":       {"httpget"},
		"kubernetes:container:probe:startup:path":       {"/startup"},
		"kubernetes:container:probe:startup:port":       {"8081"},
		"kubernetes:container:probe:startup:scheme":     {"http"},
		"kubernetes:pod:ephemeral-image:count":          {"0"},
		"kubernetes:pod:image:count":                    {"1"},
		"kubernetes:pod:image:name":                     {"app:latest"},
		"kubernetes:pod:init-image:count":               {"0"},
		"kubernetes:pod:name":                           {"probes"},
		"kubernetes:pod:namespace":                      {"default"},
	}

	collector := kubernetes.New(
		kubernetes.WithPodLister(&probesPodLister{}),
		kubernetes.WithPodResolver(&probesPodResolver{}),
	)

	md, err := collector.GetMetadata(metadatax.ContextWithPID(context.Background(), 1))
	assert.Nil(t, err)

	assert.Equal(t, expectedLabels, map[string][]string(md.GetLabels()))
}

type execProbePodResolver struct{}

func (r *execProbePodResolver) GetPodAndContainerID(pid int32) (string, string, error) {
	return execProbePodUID, execProbeContainerID, nil
}

func TestGetMetadataForExecProbe(t *testing.T) {
	t.Parallel()

	collector := kubernetes.New(
		kubernetes.WithPodLister(&probesPodLister{}),
		kubernetes.WithPodResolver(&execProbePodResolver{}),
	)

	md, err := collector.GetMetadata(metadatax.ContextWithPID(context.Background(), 1))
	assert.Nil(t, err)

	labels := md.GetLabels()

	assert.Equal(t, []string{"exec"}, labels["kubernetes:container:probe:liveness:type"])
	for _, key := range []string{"path", "port", "port-name", "scheme", "host"} {
		assert.NotContains(t, labels, "kubernetes:container:probe:liveness:"+key)
	}
}

const (
	staleCachePodUID         = "c3a1f2b4-9e21-4a6b-8c3d-1a2b3c4d5e6f"
	staleCacheOldContainerID = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	staleCacheNewContainerID = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
)

// staleCachePodLister simulates a pod list that is one generation behind reality: its first
// response (the one that gets cached) still carries the previous container instance's ID, even
// though the container count already matches what the pod spec expects. Only a forced refresh
// (skipCache=true, i.e. a retry) returns the pod list with the current container ID.
type staleCachePodLister struct {
	calls int
}

func (l *staleCachePodLister) GetPods(ctx context.Context) ([]corev1.Pod, error) {
	l.calls++

	containerID := staleCacheOldContainerID
	if l.calls > 1 {
		containerID = staleCacheNewContainerID
	}

	return []corev1.Pod{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "app",
				Namespace: "default",
				UID:       types.UID(staleCachePodUID),
			},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{
						Name:  "app",
						Image: "app:latest",
					},
				},
			},
			Status: corev1.PodStatus{
				ContainerStatuses: []corev1.ContainerStatus{
					{
						Name:        "app",
						ContainerID: "containerd://" + containerID,
					},
				},
			},
		},
	}, nil
}

type staleCachePodResolver struct{}

func (r *staleCachePodResolver) GetPodAndContainerID(pid int32) (string, string, error) {
	return staleCachePodUID, staleCacheNewContainerID, nil
}

// TestGetMetadataRetriesWhenCachedPodHasStaleContainerID reproduces a race where the collector's
// cached pod list still reflects the previous container instance (e.g. right after a restart):
// the container count matches what the pod spec expects, but the container ID the process
// resolver reports isn't among the cached statuses. getPodContext must not report "found" in
// that case - doing so skips the retry-with-fresh-pod-list path entirely, and the container
// segment is silently dropped from the metadata (AddLabel drops empty values, so a zero-value
// container never even shows up as an empty field - the whole "kubernetes:container:*" segment
// just vanishes).
func TestGetMetadataRetriesWhenCachedPodHasStaleContainerID(t *testing.T) {
	t.Parallel()

	collector := kubernetes.New(
		kubernetes.WithPodLister(&staleCachePodLister{}),
		kubernetes.WithPodResolver(&staleCachePodResolver{}),
	)

	md, err := collector.GetMetadata(metadatax.ContextWithPID(context.Background(), 1))
	assert.NoError(t, err)

	labels := md.GetLabels()
	assert.Equal(t, []string{"app"}, labels["kubernetes:pod:name"], "pod segment should be populated from the cached pod")
	assert.Equal(t, []string{"app"}, labels["kubernetes:container:name"], "container segment must be populated by retrying with a fresh pod list, not silently dropped")
}
