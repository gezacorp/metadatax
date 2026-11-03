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

// Workload is the kind and name of what runs pod, read from the pod alone:
// the Deployment of a ReplicaSet named <deployment>-<pod-template-hash>,
// the CronJob of a Job named like one of its recent runs, the pod's
// controller otherwise, the name a static pod has without its node's, or
// the pod itself when nothing owns it.
func Workload(pod *corev1.Pod) (string, string) {
	return WorkloadAt(pod, time.Now())
}

// WorkloadAt is Workload with now as the time a CronJob's run is compared
// with.
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

// cronJobRun is the name a CronJob gives the Job of a run: its own name and
// the scheduled time in minutes since the Unix epoch. The pod's owner is
// the Job; that the CronJob owns the Job only the API can tell, so the name
// stands in for it.
var cronJobRun = regexp.MustCompile(`^(.+)-(\d{8})$`)

// cronJobRunAge is how far from now a run's scheduled time may be. It keeps
// out Jobs named with a date or a Unix time in seconds, which read as
// minutes decades away.
const cronJobRunAge = 366 * 24 * time.Hour

// cronJob is the CronJob whose run job is, when its name is one of a run
// scheduled within cronJobRunAge of now.
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
