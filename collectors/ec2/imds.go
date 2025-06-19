package ec2

import (
	"bytes"
	"context"
	"io"

	"emperror.dev/errors"
	"github.com/aws/aws-sdk-go-v2/config"
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

func NewIMDSDefaultConfig(ctx context.Context) (IMDSClient, error) {
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, errors.WrapIf(err, "could not get config for EC2 client")
	}

	return NewIMDSClient(imds.NewFromConfig(cfg)), nil
}

func (c *imdsClient) GetMetadataContent(ctx context.Context, path string) string {
	response, err := c.client.GetMetadata(ctx, &imds.GetMetadataInput{Path: path})
	if err != nil {
		return ""
	}

	content, err := c.readContent(response.Content)
	if err != nil {
		return ""
	}

	return string(content)
}

func (c *imdsClient) GetDynamicMetadataContent(ctx context.Context, path string) ([]byte, error) {
	response, err := c.client.GetDynamicData(ctx, &imds.GetDynamicDataInput{
		Path: path,
	})
	if err != nil {
		return nil, err
	}

	return c.readContent(response.Content)
}

func (c *imdsClient) readContent(reader io.ReadCloser) ([]byte, error) {
	buff := new(bytes.Buffer)
	if _, err := buff.ReadFrom(reader); err != nil {
		return nil, err
	}

	return buff.Bytes(), nil
}
