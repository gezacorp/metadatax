package azure

import (
	"context"
	"encoding/json"
	"io"
	"net/http"

	"emperror.dev/errors"
)

const (
	baseURL        = "http://169.254.169.254/metadata"
	defaultVersion = "2023-07-01"
)

type HTTPClient interface {
	Do(*http.Request) (*http.Response, error)
}

type metadataGetter struct {
	httpClient HTTPClient
	version    string
}

type MetadataGetterOption func(*metadataGetter)

func AzureMetadataGetterWithHTTPClient(httpClient HTTPClient) MetadataGetterOption {
	return func(g *metadataGetter) {
		g.httpClient = httpClient
	}
}

func AzureMetadataGetterWithVersion(version string) MetadataGetterOption {
	return func(g *metadataGetter) {
		g.version = version
	}
}

func NewAzureMetadataGetter(opts ...MetadataGetterOption) MetadataGetter {
	g := &metadataGetter{}

	for _, f := range opts {
		f(g)
	}

	if g.httpClient == nil {
		g.httpClient = &http.Client{}
	}

	if g.version == "" {
		g.version = defaultVersion
	}

	return g
}

func (g *metadataGetter) GetInstanceMetadata(ctx context.Context) (*AzureMetadataInstance, error) {
	content, err := g.getMetadata(ctx, "/instance")
	if err != nil {
		return nil, errors.WithStackIf(err)
	}

	var instance AzureMetadataInstance
	if err := json.Unmarshal(content, &instance); err != nil {
		return nil, errors.WithStackIf(err)
	}

	return &instance, nil
}

func (g *metadataGetter) GetLoadBalancerMetadata(ctx context.Context) (*AzureMetadataLoadBalancer, error) {
	content, err := g.getMetadata(ctx, "/loadbalancer")
	if err != nil {
		return nil, errors.WithStackIf(err)
	}

	var lb AzureMetadataLoadBalancer
	if err := json.Unmarshal(content, &lb); err != nil {
		return nil, errors.WithStackIf(err)
	}

	return &lb, nil
}

func (g *metadataGetter) getMetadata(ctx context.Context, path string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", baseURL+path, nil)
	if err != nil {
		return nil, errors.WrapIf(err, "could not instantiate http request")
	}
	req.Header.Add("Metadata", "True")

	q := req.URL.Query()
	q.Add("format", "json")
	q.Add("api-version", g.version)
	req.URL.RawQuery = q.Encode()

	resp, err := g.httpClient.Do(req)
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

	return content, nil
}
