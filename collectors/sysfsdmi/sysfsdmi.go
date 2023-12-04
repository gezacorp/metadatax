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
	metadataGetter MetadataGetter
	hasSysfs       bool

	mdcontainerGetter func() metadatax.MetadataContainer
}

type CollectorOption func(*collector)

type MetadataGetter interface {
	GetContent(key string) string
}

func CollectorWithMetadataGetter(metadataGetter MetadataGetter) CollectorOption {
	return func(c *collector) {
		c.metadataGetter = metadataGetter
	}
}

func CollectorWithMetadataContainerGetter(getter func() metadatax.MetadataContainer) CollectorOption {
	return func(c *collector) {
		c.mdcontainerGetter = getter
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

	if c.mdcontainerGetter == nil {
		c.mdcontainerGetter = func() metadatax.MetadataContainer {
			return metadatax.New(metadatax.WithPrefix(name))
		}
	}

	if c.metadataGetter == nil {
		c.metadataGetter = c
	}

	return c
}

func (c *collector) GetMetadata(ctx context.Context) (metadatax.MetadataContainer, error) {
	md := c.mdcontainerGetter()

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
		AddLabel("date", c.metadataGetter.GetContent("bios_date")).
		AddLabel("release", c.metadataGetter.GetContent("bios_release")).
		AddLabel("vendor", c.metadataGetter.GetContent("bios_vendor")).
		AddLabel("version", c.metadataGetter.GetContent("bios_version"))
}

func (c *collector) chassis(md metadatax.MetadataContainer) {
	md.Segment("chassis").
		AddLabel("asset-tag", c.metadataGetter.GetContent("chassis_asset_tag")).
		AddLabel("serial", c.metadataGetter.GetContent("chassis_serial")).
		AddLabel("type", c.metadataGetter.GetContent("chassis_type")).
		AddLabel("vendor", c.metadataGetter.GetContent("chassis_vendor")).
		AddLabel("version", c.metadataGetter.GetContent("chassis_version"))
}

func (c *collector) product(md metadatax.MetadataContainer) {
	md.Segment("product").
		AddLabel("family", c.metadataGetter.GetContent("product_family")).
		AddLabel("name", c.metadataGetter.GetContent("product_name")).
		AddLabel("serial", c.metadataGetter.GetContent("product_serial")).
		AddLabel("sku", c.metadataGetter.GetContent("product_sku")).
		AddLabel("version", c.metadataGetter.GetContent("product_version"))
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
