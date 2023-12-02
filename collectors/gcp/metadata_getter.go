package gcp

import (
	"context"
	"encoding/json"
	"io"
	"net/http"

	"emperror.dev/errors"
)

const (
	baseURL = "http://metadata.google.internal/computeMetadata/v1/"
)

type HTTPClient interface {
	Do(*http.Request) (*http.Response, error)
}

type metadataGetter struct {
	httpClient HTTPClient
}

type MetadataGetterOption func(*metadataGetter)

func GCPMetadataGetterWithHTTPClient(httpClient HTTPClient) MetadataGetterOption {
	return func(g *metadataGetter) {
		g.httpClient = httpClient
	}
}

func NewGCPMetadataGetter(opts ...MetadataGetterOption) MetadataGetter {
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
