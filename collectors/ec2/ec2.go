package ec2

import (
	"context"

	"emperror.dev/errors"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/ec2/imds"

	"github.com/gezacorp/metadatax"
)

const (
	name = "ec2"
)

type IMDSClient interface {
	GetMetadataContent(ctx context.Context, path string) string
}

type collector struct {
	imdsClient  IMDSClient
	mdcontainer metadatax.MetadataContainer
}

type CollectorOption func(*collector)

func WithIMDSClient(client IMDSClient) CollectorOption {
	return func(c *collector) {
		c.imdsClient = client
	}
}

func CollectorWithMetadataContainer(mdcontainer metadatax.MetadataContainer) CollectorOption {
	return func(c *collector) {
		c.mdcontainer = mdcontainer
	}
}

func New(opts ...CollectorOption) (metadatax.Collector, error) {
	c := &collector{}

	for _, f := range opts {
		f(c)
	}

	if c.imdsClient == nil {
		cfg, err := config.LoadDefaultConfig(context.Background())
		if err != nil {
			return nil, errors.WrapIf(err, "could not get config for EC2 client")
		}

		c.imdsClient = NewIMDSClient(imds.NewFromConfig(cfg))
	}

	if c.mdcontainer == nil {
		c.mdcontainer = metadatax.New(metadatax.WithPrefix(name))
	}

	return c, nil
}

func (c *collector) GetMetadata(ctx context.Context) (metadatax.MetadataContainer, error) {
	getters := []func(context.Context, metadatax.MetadataContainer){
		c.base,
		c.network,
		c.placement,
		c.services,
	}

	for _, f := range getters {
		f(ctx, c.mdcontainer)
	}

	return c.mdcontainer, nil
}

func (c *collector) base(ctx context.Context, md metadatax.MetadataContainer) {
	md.AddLabel("security-groups", c.imdsClient.GetMetadataContent(ctx, "security-groups"))
	md.Segment("instance").
		AddLabel("id", c.imdsClient.GetMetadataContent(ctx, "instance-id")).
		AddLabel("type", c.imdsClient.GetMetadataContent(ctx, "instance-type"))
	md.Segment("ami").AddLabel("id", c.imdsClient.GetMetadataContent(ctx, "ami-id"))
	md.Segment("kernel").AddLabel("id", c.imdsClient.GetMetadataContent(ctx, "kernel-id"))
}

func (c *collector) network(ctx context.Context, md metadatax.MetadataContainer) {
	keys := []string{
		"hostname",
		"local-hostname",
		"public-hostname",
		"local-ipv4",
		"public-ipv4",
		"local-ipv6",
		"public-ipv6",
		"mac",
	}

	nmd := md.Segment("network")
	for _, key := range keys {
		nmd.AddLabel(key, c.imdsClient.GetMetadataContent(ctx, key))
	}
}

func (c *collector) placement(ctx context.Context, md metadatax.MetadataContainer) {
	keys := []string{
		"availability-zone",
		"availability-zone-id",
		"group-name",
		"host-id",
		"partition-number",
		"region",
	}

	pmd := md.Segment("placement")
	for _, key := range keys {
		pmd.AddLabel(key, c.imdsClient.GetMetadataContent(ctx, "placement/"+key))
	}
}

func (c *collector) services(ctx context.Context, md metadatax.MetadataContainer) {
	keys := []string{
		"domain",
		"partition",
	}

	smd := md.Segment("services")
	for _, key := range keys {
		smd.AddLabel(key, c.imdsClient.GetMetadataContent(ctx, "services/"+key))
	}
}