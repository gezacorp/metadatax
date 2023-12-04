package sysfsdmi

import (
	"context"
	"os"
	"strings"

	"github.com/gezacorp/metadatax"
)

const (
	name = "sysfsdmi"

	basePath = "/sys/class/dmi/id"
)

type collector struct {
	hasSysfs       bool
	sysFSDMIClient SysFSDMIClient

	mdContainerInitFunc func() metadatax.MetadataContainer
}

type CollectorOption func(*collector)

type SysFSDMIClient interface {
	GetContent(key string) string
}

func CollectorWithSysFSDMIClient(sysFSDMIClient SysFSDMIClient) CollectorOption {
	return func(c *collector) {
		c.sysFSDMIClient = sysFSDMIClient
	}
}

func CollectorWithMetadataContainerInitFunc(fn func() metadatax.MetadataContainer) CollectorOption {
	return func(c *collector) {
		c.mdContainerInitFunc = fn
	}
}

func CollectorWithForceHasSysFS() CollectorOption {
	return func(c *collector) {
		c.hasSysfs = true
	}
}

func New(opts ...CollectorOption) metadatax.Collector {
	c := &collector{}

	for _, f := range opts {
		f(c)
	}

	if c.mdContainerInitFunc == nil {
		c.mdContainerInitFunc = func() metadatax.MetadataContainer {
			return metadatax.New(metadatax.WithPrefix(name))
		}
	}

	if c.sysFSDMIClient == nil {
		c.sysFSDMIClient = c
	}

	return c
}

func (c *collector) GetMetadata(ctx context.Context) (metadatax.MetadataContainer, error) {
	md := c.mdContainerInitFunc()

	if !c.hasSysFS() {
		return md, nil
	}

	getters := []func(metadatax.MetadataContainer){
		c.bios,
		c.chassis,
		c.product,
	}

	for _, f := range getters {
		f(md)
	}

	return md, nil
}

func (c *collector) GetContent(key string) string {
	content, err := os.ReadFile(basePath + "/" + key)
	if err != nil {
		return ""
	}

	return strings.Trim(string(content), "\n")
}

func (c *collector) bios(md metadatax.MetadataContainer) {
	md.Segment("bios").
		AddLabel("date", c.sysFSDMIClient.GetContent("bios_date")).
		AddLabel("release", c.sysFSDMIClient.GetContent("bios_release")).
		AddLabel("vendor", c.sysFSDMIClient.GetContent("bios_vendor")).
		AddLabel("version", c.sysFSDMIClient.GetContent("bios_version"))
}

func (c *collector) chassis(md metadatax.MetadataContainer) {
	md.Segment("chassis").
		AddLabel("asset-tag", c.sysFSDMIClient.GetContent("chassis_asset_tag")).
		AddLabel("serial", c.sysFSDMIClient.GetContent("chassis_serial")).
		AddLabel("type", c.sysFSDMIClient.GetContent("chassis_type")).
		AddLabel("vendor", c.sysFSDMIClient.GetContent("chassis_vendor")).
		AddLabel("version", c.sysFSDMIClient.GetContent("chassis_version"))
}

func (c *collector) product(md metadatax.MetadataContainer) {
	md.Segment("product").
		AddLabel("family", c.sysFSDMIClient.GetContent("product_family")).
		AddLabel("name", c.sysFSDMIClient.GetContent("product_name")).
		AddLabel("serial", c.sysFSDMIClient.GetContent("product_serial")).
		AddLabel("sku", c.sysFSDMIClient.GetContent("product_sku")).
		AddLabel("version", c.sysFSDMIClient.GetContent("product_version"))
}

func (c *collector) hasSysFS() bool {
	if c.hasSysfs {
		return true
	}

	if _, err := os.Stat(basePath); err != nil {
		return false
	}

	return true
}
