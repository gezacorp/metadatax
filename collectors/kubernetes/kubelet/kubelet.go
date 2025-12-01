package kubelet

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"emperror.dev/errors"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/transport"

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

func WithAccessTokenFile(tokenFile string) ClientOption {
	return func(c *kubeletClient) {
		c.accessTokenFile = tokenFile
	}
}

func WithAddress(address string) ClientOption {
	return func(c *kubeletClient) {
		c.address = address
	}
}

func WithCAPEMs(caPEM []byte) ClientOption {
	return func(c *kubeletClient) {
		c.caPEM = caPEM
	}
}

func WithCAPEMFile(caPEMFile string) ClientOption {
	return func(c *kubeletClient) {
		c.caPEMFilePath = caPEMFile
	}
}

func WithClientCertPEM(pem []byte) ClientOption {
	return func(c *kubeletClient) {
		c.clientCertPEM = pem
	}
}

func WithClientCertPEMFile(pemFile string) ClientOption {
	return func(c *kubeletClient) {
		c.clientCertPEMFilePath = pemFile
	}
}

func WithClientKeyPEMFile(pemFile string) ClientOption {
	return func(c *kubeletClient) {
		c.clientKeyPEMFilePath = pemFile
	}
}

func WithSkipCertVerify() ClientOption {
	return func(c *kubeletClient) {
		c.skipCertVerify = true
	}
}

type kubeletClient struct {
	httpClient            HTTPClient
	accessToken           string
	accessTokenFile       string
	address               string
	caPEM                 []byte
	caPEMFilePath         string
	clientCertPEM         []byte
	clientCertPEMFilePath string
	clientKeyPEMFilePath  string
	skipCertVerify        bool

	caPEMFile         kubernetes.CachedFile
	clientCertPEMFile kubernetes.CachedFile
	clientKeyPEMFile  kubernetes.CachedFile
	tlsConfig         *tls.Config
}

func NewClient(opts ...ClientOption) (kubernetes.PodLister, error) {
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

	if c.caPEMFilePath != "" {
		if f, err := kubernetes.NewCachedFile(c.caPEMFilePath, 5*time.Second); err != nil {
			return nil, errors.WithStackIf(err)
		} else {
			c.caPEMFile = f
		}
	}

	if c.clientCertPEMFilePath != "" {
		if c.clientKeyPEMFilePath == "" {
			return nil, errors.NewPlain("missing client certificate private key path")
		}

		if f, err := kubernetes.NewCachedFile(c.clientCertPEMFilePath, 5*time.Second); err != nil {
			return nil, errors.WithStackIf(err)
		} else {
			c.clientCertPEMFile = f
		}
	}

	if c.clientKeyPEMFilePath != "" {
		if c.clientCertPEMFilePath == "" {
			return nil, errors.NewPlain("missing client certificate path")
		}

		if f, err := kubernetes.NewCachedFile(c.clientKeyPEMFilePath, 5*time.Second); err != nil {
			return nil, errors.WithStackIf(err)
		} else {
			c.clientKeyPEMFile = f
		}
	}

	if c.httpClient == nil {
		if err := c.setHTTPClient(); err != nil {
			return nil, errors.WithStackIf(err)
		}
	}

	return c, nil
}

func (c *kubeletClient) GetPods(ctx context.Context) ([]corev1.Pod, error) {
	var err error

	httpClient, err := c.getHTTPClient()
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://"+c.address+"/pods", nil)
	if err != nil {
		return nil, errors.WrapIf(err, "could not instantiate http request")
	}

	if c.accessToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.accessToken)
	}

	resp, err := httpClient.Do(req)
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

func (c *kubeletClient) getCAPEM() ([]byte, bool, error) {
	if c.caPEMFile == nil && c.caPEM != nil {
		return c.caPEM, false, nil
	}

	if c.caPEMFile != nil {
		return c.caPEMFile.Content()
	}

	return nil, false, nil
}

func (c *kubeletClient) setHTTPClient() error {
	tlsConfig := &tls.Config{
		InsecureSkipVerify: c.skipCertVerify,
	}

	if c.clientCertPEMFile != nil && c.clientKeyPEMFile != nil {
		tlsConfig.GetClientCertificate = func(_ *tls.CertificateRequestInfo) (*tls.Certificate, error) {
			return kubernetes.NewCachedCertificate(c.clientCertPEMFile, c.clientKeyPEMFile).Certificate()
		}
	} else if c.clientCertPEM != nil {
		clientCert, err := tls.X509KeyPair(c.clientCertPEM, c.clientCertPEM)
		if err != nil {
			return errors.WrapIf(err, "could not parse x509 key pair")
		}
		tlsConfig.Certificates = append(tlsConfig.Certificates, clientCert)
	}

	tp := &http.Transport{
		TLSClientConfig: tlsConfig,
	}

	rt, err := transport.NewBearerAuthWithRefreshRoundTripper(c.accessToken, c.accessTokenFile, tp)
	if err != nil {
		return errors.WithStackIf(err)
	}

	c.tlsConfig = tlsConfig
	c.httpClient = &http.Client{
		Transport: rt,
	}

	return nil
}

func (c *kubeletClient) getHTTPClient() (HTTPClient, error) {
	if c.tlsConfig == nil {
		return c.httpClient, nil
	}

	caPEM, changed, err := c.getCAPEM()
	if err != nil {
		return nil, err
	}

	if changed {
		c.tlsConfig.RootCAs = x509.NewCertPool()
		c.tlsConfig.RootCAs.AppendCertsFromPEM(caPEM)
	}

	return c.httpClient, nil
}
