package kubernetes

import (
	"context"
	"regexp"
	"strconv"
	"strings"

	"emperror.dev/errors"
	corev1 "k8s.io/api/core/v1"

	"github.com/gezacorp/metadatax"
)

const (
	name = "kubernetes"
)

var PodAndContainerIDNotFoundError = errors.Sentinel("could not find pod or container id")

type PodResolver interface {
	GetPodAndContainerID(pid int32) (string, string, error)
}

type PodLister interface {
	GetPods(ctx context.Context) ([]corev1.Pod, error)
}

type podContext struct {
	pod             corev1.Pod
	container       corev1.Container
	containerStatus corev1.ContainerStatus
}

type collector struct {
	podLister   PodLister
	podResolver PodResolver

	mdContainerInitFunc func() metadatax.MetadataContainer
	skipOnSoftError     bool
}

type CollectorOption func(*collector)

func WithPodLister(getter PodLister) CollectorOption {
	return func(c *collector) {
		c.podLister = getter
	}
}

func WithPodResolver(resolver PodResolver) CollectorOption {
	return func(c *collector) {
		c.podResolver = resolver
	}
}

func CollectorWithMetadataContainerInitFunc(fn func() metadatax.MetadataContainer) CollectorOption {
	return func(c *collector) {
		c.mdContainerInitFunc = fn
	}
}

func WithSkipOnSoftError() CollectorOption {
	return func(c *collector) {
		c.skipOnSoftError = true
	}
}

func New(opts ...CollectorOption) metadatax.Collector {
	c := &collector{}

	for _, f := range opts {
		f(c)
	}

	if c.podResolver == nil {
		c.podResolver = c
	}

	if c.mdContainerInitFunc == nil {
		c.mdContainerInitFunc = func() metadatax.MetadataContainer {
			return metadatax.New(metadatax.WithPrefix(name))
		}
	}

	return c
}

func (c *collector) GetMetadata(ctx context.Context) (metadatax.MetadataContainer, error) {
	md := c.mdContainerInitFunc()

	if c.podLister == nil {
		if c.skipOnSoftError {
			return md, nil
		}

		return md, errors.NewPlain("pod lister is not specified")
	}

	pid, found := metadatax.PIDFromContext(ctx)
	if !found {
		return nil, metadatax.PIDNotFoundError
	}

	podID, containerID, err := c.podResolver.GetPodAndContainerID(pid)
	if err != nil && !c.skipOnSoftError {
		return nil, errors.WithDetails(err, "pid", pid)
	}

	if podID == "" || containerID == "" {
		return md, nil
	}

	pods, err := c.podLister.GetPods(ctx)
	if err != nil {
		return nil, errors.WrapIf(err, "could not get pods")
	}

	podctx, found := c.getPodContext(podID, containerID, pods)
	if !found {
		return md, nil
	}

	getters := []func(podContext, metadatax.MetadataContainer){
		c.pod,
		c.container,
		c.labels,
		c.annotations,
		c.images,
	}

	for _, f := range getters {
		f(podctx, md)
	}

	return md, nil
}

func (c *collector) pod(podctx podContext, md metadatax.MetadataContainer) {
	pod := podctx.pod

	pmd := md.Segment("pod")
	pmd.AddLabel("name", pod.GetName()).
		AddLabel("namespace", pod.GetNamespace()).
		AddLabel("serviceaccount", pod.Spec.ServiceAccountName)

	omd := pmd.Segment("owner")
	for _, owner := range pod.GetOwnerReferences() {
		omd.AddLabel("kind", strings.ToLower(owner.Kind)).
			AddLabel("kind-with-version", strings.ToLower(owner.APIVersion)+"/"+strings.ToLower(owner.Kind)).
			AddLabel("name", owner.Name)
	}

	md.Segment("node").AddLabel("name", pod.Spec.NodeName)
}

func (c *collector) container(podctx podContext, md metadatax.MetadataContainer) {
	cmd := md.Segment("container")
	cmd.AddLabel("name", podctx.container.Name)
	cmd.Segment("image").AddLabel("id", podctx.containerStatus.ImageID)
}

func (c *collector) labels(podctx podContext, md metadatax.MetadataContainer) {
	lmd := md.Segment("label")

	for k, v := range podctx.pod.GetLabels() {
		lmd.AddLabel(k, v)
	}
}

func (c *collector) annotations(podctx podContext, md metadatax.MetadataContainer) {
	amd := md.Segment("annotation")

	for k, v := range podctx.pod.GetAnnotations() {
		amd.AddLabel(k, v)
	}
}

func (c *collector) images(podctx podContext, md metadatax.MetadataContainer) {
	const nameKey = "name"
	const countKey = "count"

	pod := podctx.pod
	pmd := md.Segment("pod")
	imd := pmd.Segment("image")

	for _, cs := range pod.Status.ContainerStatuses {
		imd.AddLabel("id", cs.ImageID)
	}

	imageCount := 0
	for _, c := range pod.Spec.Containers {
		imageCount++
		imd.AddLabel(nameKey, c.Image)
	}
	imd.AddLabel(countKey, strconv.Itoa(imageCount))

	imageCount = 0
	imd = pmd.Segment("init-image")
	for _, c := range pod.Spec.InitContainers {
		imageCount++
		imd.AddLabel(nameKey, c.Image)
	}
	imd.AddLabel(countKey, strconv.Itoa(imageCount))

	imageCount = 0
	imd = pmd.Segment("ephemeral-image")
	for _, c := range pod.Spec.EphemeralContainers {
		imageCount++
		imd.AddLabel(nameKey, c.Image)
	}
	imd.AddLabel(countKey, strconv.Itoa(imageCount))
}

func (c *collector) GetPodAndContainerID(pid int32) (string, string, error) {
	k8sPodContainerIDRegex := regexp.MustCompile(`([a-z0-9/.-]+)?([/-]pod)?((?i)[a-z0-9-_]{36}).*((?i)[a-z0-9]{64})`)

	cgroups, err := GetCgroupsForPID(int(pid))
	if err != nil {
		return "", "", errors.WrapIf(err, "could not get cgroups for pid")
	}

	for _, cgroup := range cgroups {
		if match := k8sPodContainerIDRegex.FindStringSubmatch(cgroup.Path); len(match) > 0 && len(match[3]) == 36 && len(match[4]) == 64 {
			return strings.ReplaceAll(match[3], "_", "-"), match[4], nil
		}
	}

	return "", "", PodAndContainerIDNotFoundError
}

func (c *collector) getPodContext(podID, containerID string, pods []corev1.Pod) (podContext, bool) {
	podContext := podContext{}

	for _, _pod := range pods {
		if string(_pod.GetUID()) == podID {
			podContext.pod = _pod
		}
	}

	if podContext.pod.GetName() == "" {
		return podContext, false
	}

	for _, _containerStatus := range podContext.pod.Status.ContainerStatuses {
		if strings.Contains(_containerStatus.ContainerID, containerID) {
			podContext.containerStatus = _containerStatus
			break
		}
	}

	for _, _container := range podContext.pod.Spec.Containers {
		if _container.Name == podContext.containerStatus.Name {
			podContext.container = _container
			break
		}
	}

	if podContext.container.Name == "" {
		for _, _container := range podContext.pod.Spec.InitContainers {
			if _container.Name == podContext.containerStatus.Name {
				podContext.container = _container
				break
			}
		}
	}

	if podContext.container.Name == "" {
		for _, c := range podContext.pod.Spec.EphemeralContainers {
			if c.Name == podContext.containerStatus.Name {
				podContext.container = corev1.Container(c.EphemeralContainerCommon)
				break
			}
		}
	}

	return podContext, true
}
