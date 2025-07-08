package kubelet

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"emperror.dev/errors"
	corev1 "k8s.io/api/core/v1"

	"github.com/gezacorp/metadatax/collectors/kubernetes"
)

const (
	defaultAddress = "127.0.0.1:10250"
)

type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

type ClientOption func(*kubeletClient)

func WithHTTPClient(client HTTPClient) ClientOption {
	return func(c *kubeletClient) {
		c.httpClient = client
	}
}

func WithAccessToken(token string) ClientOption {
	return func(c *kubeletClient) {
		c.accessToken = token
	}
}

func WithAddress(address string) ClientOption {
	return func(c *kubeletClient) {
		c.address = address
	}
}

func WithCAPEMs(caPEMs []byte) ClientOption {
	return func(c *kubeletClient) {
		c.CAPEMs = caPEMs
	}
}

func WithClientCertPEM(pem []byte) ClientOption {
	return func(c *kubeletClient) {
		c.clientCertPEM = pem
	}
}

func WithSkipCertVerify() ClientOption {
	return func(c *kubeletClient) {
		c.skipCertVerify = true
	}
}

type kubeletClient struct {
	httpClient     HTTPClient
	accessToken    string
	address        string
	CAPEMs         []byte
	clientCertPEM  []byte
	skipCertVerify bool
}

func NewClient(opts ...ClientOption) (kubernetes.PodsGetter, error) {
	c := &kubeletClient{}

	for _, f := range opts {
		f(c)
	}

	if c.address == "" {
		if hn, err := kubernetes.NodeName(); err == nil {
			c.address = fmt.Sprintf("%s:10250", hn)
		} else {
			c.address = defaultAddress
		}
	}

	var err error
	if c.httpClient == nil {
		c.httpClient, err = c.getHTTPClient()
		if err != nil {
			return nil, err
		}
	}

	return c, nil
}

func (c *kubeletClient) GetPods(ctx context.Context) ([]corev1.Pod, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://"+c.address+"/pods", nil)
	if err != nil {
		return nil, errors.WrapIf(err, "could not instantiate http request")
	}

	if c.accessToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.accessToken)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, errors.WrapIf(err, "could not perform http request")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, errors.Errorf("non-200 response status: %s", resp.Status)
	}

	content, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.WrapIf(err, "could not read response")
	}

	var pods corev1.PodList
	if err := json.Unmarshal(content, &pods); err != nil {
		return nil, errors.WrapIf(err, "could not unmarshal response")
	}

	return pods.Items, nil
}

func (c *kubeletClient) getX509CertPool() *x509.CertPool {
	pool := x509.NewCertPool()
	pool.AppendCertsFromPEM(c.CAPEMs)

	return pool
}

func (c *kubeletClient) getHTTPClient() (HTTPClient, error) {
	tlsConfig := &tls.Config{
		RootCAs:            c.getX509CertPool(),
		InsecureSkipVerify: c.skipCertVerify,
	}

	if c.clientCertPEM != nil {
		clientCert, err := tls.X509KeyPair(c.clientCertPEM, c.clientCertPEM)
		if err != nil {
			return nil, errors.WrapIf(err, "could not parse x509 key pair")
		}
		tlsConfig.Certificates = append(tlsConfig.Certificates, clientCert)
	}

	return &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: tlsConfig,
		},
	}, nil
}
