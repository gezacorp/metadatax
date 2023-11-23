package ec2

import (
	"bytes"
	"context"
	"io"

	"github.com/aws/aws-sdk-go-v2/feature/ec2/imds"
)

type imdsClient struct {
	client *imds.Client
}

func NewIMDSClient(client *imds.Client) IMDSClient {
	return &imdsClient{
		client: client,
	}
}

func (c *imdsClient) GetMetadataContent(ctx context.Context, path string) string {
	response, err := c.client.GetMetadata(ctx, &imds.GetMetadataInput{Path: path})
	if err != nil {
		return ""
	}

	return c.readContent(response.Content)
}

func (c *imdsClient) readContent(reader io.ReadCloser) string {
	buff := new(bytes.Buffer)
	if _, err := buff.ReadFrom(reader); err != nil {
		return ""
	}

	return buff.String()
}
