package autoconfig

import (
	"os"

	"emperror.dev/errors"

	"github.com/gezacorp/metadatax/collectors/kubernetes"
	"github.com/gezacorp/metadatax/collectors/kubernetes/apiserver"
	"github.com/gezacorp/metadatax/collectors/kubernetes/kubelet"
)

var AutoConfigurationFailedErr = errors.NewPlain("auto configuration failed")

type SourceType string

const (
	KubeletSourceType   SourceType = "kubelet"
	APIServerSourceType SourceType = "apiserver"
)

type Provider string

const (
	K3sProvider      Provider = "k3s"
	KindProvider     Provider = "kind"
	MicroK8sProvider Provider = "microk8s"
	GKEProvider      Provider = "gke"
	EKSProvider      Provider = "eks"
)

type Config struct {
	Provider          Provider
	SourceType        SourceType
	KubeletConfigFile string
	KubeConfigFile    string
	CACertFile        string
	CertFile          string
	KeyFile           string
}

var configs = []Config{
	{
		Provider:   MicroK8sProvider,
		SourceType: KubeletSourceType,
		CACertFile: "/var/snap/microk8s/current/certs/kubelet.crt",
		CertFile:   "/var/snap/microk8s/current/certs/apiserver-kubelet-client.crt",
		KeyFile:    "/var/snap/microk8s/current/certs/apiserver-kubelet-client.key",
	},
	{
		Provider:       MicroK8sProvider,
		SourceType:     APIServerSourceType,
		KubeConfigFile: "/var/snap/microk8s/current/credentials/kubelet.config",
	},
	{
		Provider:          K3sProvider,
		SourceType:        KubeletSourceType,
		KubeletConfigFile: "/var/lib/rancher/k3s/agent/etc/kubelet.conf.d/00-k3s-defaults.conf",
		CACertFile:        "/var/lib/rancher/k3s/agent/serving-kubelet.crt",
		CertFile:          "/var/lib/rancher/k3s/agent/client-kubelet.crt",
		KeyFile:           "/var/lib/rancher/k3s/agent/client-kubelet.key",
	},
	{
		Provider:       K3sProvider,
		SourceType:     APIServerSourceType,
		KubeConfigFile: "/var/lib/rancher/k3s/agent/kubelet.kubeconfig",
	},
	{
		Provider:          KindProvider,
		SourceType:        KubeletSourceType,
		KubeletConfigFile: "/var/lib/kubelet/config.yaml",
		CACertFile:        "/var/lib/kubelet/pki/kubelet.crt",
		CertFile:          "/etc/kubernetes/pki/apiserver-kubelet-client.crt",
		KeyFile:           "/etc/kubernetes/pki/apiserver-kubelet-client.key",
	},
	{
		Provider:          GKEProvider,
		SourceType:        KubeletSourceType,
		KubeletConfigFile: "/home/kubernetes/kubelet-config.yaml",
		CACertFile:        "/etc/srv/kubernetes/pki/ca-certificates.crt",
		CertFile:          "/var/lib/kubelet/pki/kubelet-client.crt",
		KeyFile:           "/var/lib/kubelet/pki/kubelet-client.key",
	},
	{
		Provider:       EKSProvider,
		SourceType:     APIServerSourceType,
		KubeConfigFile: "/var/lib/kubelet/kubeconfig",
	},
}

func (c Config) Available() bool {
	switch c.SourceType {
	case APIServerSourceType:
		return everyFileExists(c.KubeConfigFile)
	case KubeletSourceType:
		return everyFileExists(c.CACertFile, c.CertFile, c.KeyFile)
	default:
		return false
	}
}

func PodLister() (kubernetes.PodLister, error) {
	var cfg Config
	for _, config := range configs {
		if config.Available() {
			cfg = config

			break
		}
	}

	switch cfg.SourceType {
	case APIServerSourceType:
		opts := []apiserver.ClientOption{
			apiserver.WithKubeconfig(cfg.KubeConfigFile),
		}

		return apiserver.NewClient(opts...)
	case KubeletSourceType:
		opts := []kubelet.ClientOption{}
		if content, err := os.ReadFile(cfg.CACertFile); err != nil {
			return nil, err
		} else {
			opts = append(opts, kubelet.WithCAPEMs(content))
		}

		certContent, err := os.ReadFile(cfg.CertFile)
		if err != nil {
			return nil, err
		}

		keyContent, err := os.ReadFile(cfg.KeyFile)
		if err != nil {
			return nil, err
		}

		opts = append(opts, kubelet.WithClientCertPEM(append(certContent, keyContent...)))

		return kubelet.NewClient(opts...)
	}

	return nil, errors.WithStackIf(AutoConfigurationFailedErr)
}

func everyFileExists(paths ...string) bool {
	for _, path := range paths {
		if !fileExists(path) {
			return false
		}
	}

	return true
}

func fileExists(path string) bool {
	_, err := os.Stat(path)

	return err == nil || !os.IsNotExist(err)
}
