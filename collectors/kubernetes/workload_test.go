package kubernetes_test

import (
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/gezacorp/metadatax/collectors/kubernetes"
)

func pod(name string, labels map[string]string, owners ...metav1.OwnerReference) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:            name,
			Labels:          labels,
			OwnerReferences: owners,
		},
		Spec: corev1.PodSpec{NodeName: "node-1"},
	}
}

func owner(kind, name string, controller bool) metav1.OwnerReference {
	return metav1.OwnerReference{Kind: kind, Name: name, Controller: &controller}
}

func TestWorkloadAt_CronJobRunsAndTimestampedJobs(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 11, 3, 12, 0, 0, 0, time.UTC)
	minutes := strconv.FormatInt(now.Unix()/60, 10)

	for _, tc := range []struct {
		job      string
		wantKind string
		wantName string
	}{
		{
			job:      "backup-" + minutes,
			wantKind: "cronjob",
			wantName: "backup",
		},
		{
			job:      "report-" + strconv.FormatInt(now.Add(-30*24*time.Hour).Unix()/60, 10),
			wantKind: "cronjob",
			wantName: "report",
		},
		{
			job:      "backup-20261103",
			wantKind: "job",
			wantName: "backup-20261103",
		},
		{
			job:      "db-migrate-1730612345",
			wantKind: "job",
			wantName: "db-migrate-1730612345",
		},
	} {
		kind, name := kubernetes.WorkloadAt(pod(tc.job+"-x7k2p", nil, owner("Job", tc.job, true)), now)
		assert.Equal(t, tc.wantKind, kind, tc.job)
		assert.Equal(t, tc.wantName, name, tc.job)
	}
}

func TestWorkload(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name     string
		pod      *corev1.Pod
		wantKind string
		wantName string
	}{
		{
			name:     "deployment through its replicaset",
			pod:      pod("api-7d9f8c6b5-x2x9q", map[string]string{"pod-template-hash": "7d9f8c6b5"}, owner("ReplicaSet", "api-7d9f8c6b5", true)),
			wantKind: "deployment",
			wantName: "api",
		},
		{
			name:     "replicaset of its own",
			pod:      pod("cache-abcde", nil, owner("ReplicaSet", "cache", true)),
			wantKind: "replicaset",
			wantName: "cache",
		},
		{
			name:     "replicaset whose hash does not match its name",
			pod:      pod("api-other-x2x9q", map[string]string{"pod-template-hash": "7d9f8c6b5"}, owner("ReplicaSet", "api-other", true)),
			wantKind: "replicaset",
			wantName: "api-other",
		},
		{
			name:     "statefulset",
			pod:      pod("db-0", map[string]string{"controller-revision-hash": "db-5f7b"}, owner("StatefulSet", "db", true)),
			wantKind: "statefulset",
			wantName: "db",
		},
		{
			name:     "daemonset",
			pod:      pod("agent-7xk2p", nil, owner("DaemonSet", "agent", true)),
			wantKind: "daemonset",
			wantName: "agent",
		},
		{
			name:     "controller owner wins",
			pod:      pod("x-1", nil, owner("ConfigMap", "cm", false), owner("Job", "migrate", true)),
			wantKind: "job",
			wantName: "migrate",
		},
		{
			name:     "job whose name ends in a short number",
			pod:      pod("migrate-v2-1234-x7k2p", nil, owner("Job", "migrate-v2-1234", true)),
			wantKind: "job",
			wantName: "migrate-v2-1234",
		},
		{
			name:     "static pod",
			pod:      pod("kube-apiserver-node-1", nil, owner("Node", "node-1", true)),
			wantKind: "static",
			wantName: "kube-apiserver",
		},
		{
			name:     "bare pod",
			pod:      pod("debug", nil),
			wantKind: "pod",
			wantName: "debug",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			kind, name := kubernetes.Workload(tc.pod)
			assert.Equal(t, tc.wantKind, kind)
			assert.Equal(t, tc.wantName, name)
		})
	}
}
