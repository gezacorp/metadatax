package ec2

import (
	"bytes"
	"context"
	"net/http"
	"os"
	"time"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/ec2/imds"
	"github.com/gezacorp/metadatax"
)

const (
	name = "ec2"
)

type IMDSClient interface {
	GetMetadataContent(ctx context.Context, path string) string
	GetDynamicMetadataContent(ctx context.Context, path string) ([]byte, error)
}

type collector struct {
	imdsClient IMDSClient
	onEC2      bool

	mdContainerInitFunc func() metadatax.MetadataContainer
}

type CollectorOption func(*collector)

func WithIMDSClient(client IMDSClient) CollectorOption {
	return func(c *collector) {
		c.imdsClient = client
	}
}

func WithForceOnEC2() CollectorOption {
	return func(c *collector) {
		c.onEC2 = true
	}
}

func CollectorWithMetadataContainerInitFunc(fn func() metadatax.MetadataContainer) CollectorOption {
	return func(c *collector) {
		c.mdContainerInitFunc = fn
	}
}

func New(opts ...CollectorOption) (metadatax.Collector, error) {
	c := &collector{}

	for _, f := range opts {
		f(c)
	}

	if c.imdsClient == nil {
		if ic, err := NewIMDSDefaultConfig(context.Background()); err != nil {
			return nil, err
		} else {
			c.imdsClient = ic
		}
	}

	if c.mdContainerInitFunc == nil {
		c.mdContainerInitFunc = func() metadatax.MetadataContainer {
			return metadatax.New(metadatax.WithPrefix(name))
		}
	}

	return c, nil
}

func (c *collector) GetMetadata(ctx context.Context) (metadatax.MetadataContainer, error) {
	md := c.mdContainerInitFunc()

	if !c.isOnEC2(ctx) {
		return md, nil
	}

	getters := []func(context.Context, metadatax.MetadataContainer){
		c.base,
		c.network,
		c.placement,
		c.services,
	}

	for _, f := range getters {
		f(ctx, md)
	}

	return md, nil
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

func (c *collector) isOnEC2(ctx context.Context) bool {
	if c.onEC2 {
		return true
	}

	// Check Xen hypervisor UUID starts with "ec2"
	hypervisorUUIDPath := "/sys/hypervisor/uuid"
	if fileExists(hypervisorUUIDPath) {
		if data, err := os.ReadFile(hypervisorUUIDPath); err == nil {
			if bytes.HasPrefix(bytes.ToLower(data), []byte("ec2")) {
				c.onEC2 = true
				return true
			}
		}
	}

	// Check DMI BIOS vendor for "Amazon"
	biosVendorPath := "/sys/class/dmi/id/bios_vendor"
	if fileExists(biosVendorPath) {
		if data, err := os.ReadFile(biosVendorPath); err == nil {
			if bytes.Contains(bytes.ToLower(data), []byte("amazon")) {
				c.onEC2 = true
				return true
			}
		}
	}

	// Check DMI BIOS version for "amazon"
	biosVersionPath := "/sys/class/dmi/id/bios_version"
	if fileExists(biosVersionPath) {
		if data, err := os.ReadFile(biosVersionPath); err == nil {
			if bytes.Contains(bytes.ToLower(data), []byte("amazon")) {
				c.onEC2 = true
				return true
			}
		}
	}

	// Check system vendor for "amazon"
	sysVendorPath := "/sys/class/dmi/id/sys_vendor"
	if fileExists(sysVendorPath) {
		if data, err := os.ReadFile(sysVendorPath); err == nil {
			if bytes.Contains(bytes.ToLower(data), []byte("amazon")) {
				c.onEC2 = true
				return true
			}
		}
	}

	// As a last resort, try to access the EC2 metadata service
	cfg, err := config.LoadDefaultConfig(ctx, config.WithHTTPClient(&http.Client{Timeout: 500 * time.Millisecond}))
	if err == nil {
		imdsClient := imds.NewFromConfig(cfg)
		iid, err := imdsClient.GetInstanceIdentityDocument(ctx, &imds.GetInstanceIdentityDocumentInput{})
		if err == nil && iid != nil && iid.InstanceID != "" {
			c.onEC2 = true
			return true
		}
	}

	return false
}

// fileExists checks if a file exists and is not a directory
func fileExists(filename string) bool {
	info, err := os.Stat(filename)
	if os.IsNotExist(err) {
		return false
	}
	return !info.IsDir()
}
