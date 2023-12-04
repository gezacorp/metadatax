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
	content, err := g.getMetadata(ctx, "/instance/")
	if err != nil {
		return nil, errors.WithStackIf(err)
	}

	var instance GCPMetadataInstance
	if err := json.Unmarshal(content, &instance); err != nil {
		return nil, errors.WithStackIf(err)
	}

	return &instance, nil
}

func (g *metadataGetter) getMetadata(ctx context.Context, path string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", baseURL+path, nil)
	if err != nil {
		return nil, errors.WrapIf(err, "could not instantiate http request")
	}
	req.Header.Add("Metadata-Flavor", "Google")

	q := req.URL.Query()
	q.Add("recursive", "true")
	q.Add("alt", "json")
	req.URL.RawQuery = q.Encode()

	return metadatax.SendHTTPGetRequest(ctx, g.httpClient, req)
}
