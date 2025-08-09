package apiserver

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"

	"github.com/gezacorp/metadatax/collectors/kubernetes"
)

type ClientOption func(*apiServerClient)

type apiServerClient struct {
	c          client.Client
	nodeName   string
	kubeconfig string
}

func WithKubeconfig(path string) ClientOption {
	return func(c *apiServerClient) {
		c.kubeconfig = path
	}
}

func NewClient(opts ...ClientOption) (kubernetes.PodLister, error) {
	var err error

	c := &apiServerClient{}

	for _, o := range opts {
		o(c)
	}

	var cfg *rest.Config

	if c.kubeconfig != "" {
		cfg, err = clientcmd.BuildConfigFromFlags("", c.kubeconfig)
	} else {
		cfg, err = config.GetConfig()
	}

	if err != nil {
		return nil, err
	}

	if c.nodeName, err = kubernetes.NodeName(); err != nil {
		return nil, err
	}

	s := runtime.NewScheme()
	if err := scheme.AddToScheme(s); err != nil {
		return nil, err
	}

	c.c, err = client.New(cfg, client.Options{
		Scheme: s,
	})
	if err != nil {
		return nil, err
	}

	return c, nil
}

func (c *apiServerClient) GetPods(ctx context.Context) ([]corev1.Pod, error) {
	pods := &corev1.PodList{}

	if err := c.c.List(ctx, pods, client.MatchingFields(map[string]string{
		"spec.nodeName": c.nodeName,
	})); err != nil {
		return nil, err
	}

	return pods.Items, nil
}
