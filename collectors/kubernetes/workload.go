package kubernetes

import (
	"regexp"
	"strconv"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const podTemplateHashLabel = "pod-template-hash"

func Workload(pod *corev1.Pod) (string, string) {
	return WorkloadAt(pod, time.Now())
}

func WorkloadAt(pod *corev1.Pod, now time.Time) (string, string) {
	owner, ok := controller(pod.GetOwnerReferences())
	if !ok {
		return "pod", pod.GetName()
	}

	kind := strings.ToLower(owner.Kind)

	switch kind {
	case "replicaset":
		if hash := pod.GetLabels()[podTemplateHashLabel]; hash != "" {
			if name, ok := strings.CutSuffix(owner.Name, "-"+hash); ok {
				return "deployment", name
			}
		}
	case "job":
		if name, ok := cronJob(owner.Name, now); ok {
			return "cronjob", name
		}
	case "node":
		return "static", strings.TrimSuffix(pod.GetName(), "-"+owner.Name)
	}

	return kind, owner.Name
}

// cronJobRun matches the Job names a CronJob creates:
// <cronjob>-<scheduled time in minutes since the epoch>.
var cronJobRun = regexp.MustCompile(`^(.+)-(\d{8})$`)

// cronJobRunAge is the most a run's scheduled time may differ from now.
const cronJobRunAge = 366 * 24 * time.Hour

func cronJob(job string, now time.Time) (string, bool) {
	m := cronJobRun.FindStringSubmatch(job)
	if m == nil {
		return "", false
	}

	minutes, err := strconv.ParseInt(m[2], 10, 64)
	if err != nil {
		return "", false
	}

	if d := now.Sub(time.Unix(minutes*60, 0)); d.Abs() > cronJobRunAge {
		return "", false
	}

	return m[1], true
}

func controller(owners []metav1.OwnerReference) (metav1.OwnerReference, bool) {
	for _, o := range owners {
		if o.Controller != nil && *o.Controller {
			return o, true
		}
	}

	if len(owners) > 0 {
		return owners[0], true
	}

	return metav1.OwnerReference{}, false
}
