package metadatax

import (
	"context"
	"io"
	"net/http"

	"emperror.dev/errors"
)

type HTTPClient interface {
	Do(*http.Request) (*http.Response, error)
}

func SendHTTPGetRequest(ctx context.Context, httpClient HTTPClient, req *http.Request) ([]byte, error) {
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

	return content, nil
}
