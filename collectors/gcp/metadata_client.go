package gcp

import (
	"context"
	"encoding/json"
	"net/http"

	"emperror.dev/errors"

	"github.com/gezacorp/metadatax"
)

const (
	baseURL = "http://metadata.google.internal/computeMetadata/v1/"
)

type metadataGetter struct {
	httpClient metadatax.HTTPClient
}

type MetadataGetterOption func(*metadataGetter)

func GCPMetadataClientWithHTTPClient(httpClient metadatax.HTTPClient) MetadataGetterOption {
	return func(g *metadataGetter) {
		g.httpClient = httpClient
	}
}

func NewGCPMetadataClient(opts ...MetadataGetterOption) GCPMetadataClient {
	g := &metadataGetter{}

	for _, f := range opts {
		f(g)
	}

	if g.httpClient == nil {
		g.httpClient = &http.Client{}
	}

	return g
}

func (g *metadataGetter) GetInstanceMetadata(ctx context.Context) (*GCPMetadataInstance, error) {
	content, err := g.getMetadata(ctx, "/instance/", map[string]string{
		"recursive": "true",
		"alt":       "json",
	})
	if err != nil {
		return nil, errors.WithStackIf(err)
	}

	var instance GCPMetadataInstance
	if err := json.Unmarshal(content, &instance); err != nil {
		return nil, errors.WithStackIf(err)
	}

	return &instance, nil
}

func (g *metadataGetter) GetProjectMetadata(ctx context.Context) (*GCPProjectMetadata, error) {
	content, err := g.getMetadata(ctx, "/project/", map[string]string{
		"recursive": "true",
		"alt":       "json",
	})
	if err != nil {
		return nil, errors.WithStackIf(err)
	}

	var project GCPProjectMetadata
	if err := json.Unmarshal(content, &project); err != nil {
		return nil, errors.WithStackIf(err)
	}

	return &project, nil
}

func (g *metadataGetter) GetInstanceIdentityToken(ctx context.Context, audience string, format string) ([]byte, error) {
	return g.getMetadata(ctx, "/instance/service-accounts/default/identity", map[string]string{
		"audience": audience,
		"format":   format,
	})
}

func (g *metadataGetter) getMetadata(ctx context.Context, path string, queryValues map[string]string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", baseURL+path, nil)
	if err != nil {
		return nil, errors.WrapIf(err, "could not instantiate http request")
	}
	req.Header.Add("Metadata-Flavor", "Google")

	q := req.URL.Query()
	for k, v := range queryValues {
		q.Add(k, v)
	}
	req.URL.RawQuery = q.Encode()

	return metadatax.SendHTTPGetRequest(ctx, g.httpClient, req)
}
