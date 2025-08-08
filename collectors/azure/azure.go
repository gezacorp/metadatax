package azure

import (
	"bytes"
	"context"
	"os"

	"github.com/gezacorp/metadatax"
)

const (
	name = "azure"
)

type collector struct {
	onAzure              *bool
	getAzureMetadataFunc GetAzureMetadataFunc

	mdContainerInitFunc func() metadatax.MetadataContainer
}

type CollectorOption func(*collector)

type GetAzureMetadataFunc func(ctx context.Context) AzureMetadata

type AzureMetadata interface {
	GetInstanceMetadata(ctx context.Context) (*AzureMetadataInstance, error)
	GetLoadBalancerMetadata(ctx context.Context) (*AzureMetadataLoadBalancer, error)
}

func CollectorWithForceOnAzure() CollectorOption {
	return func(c *collector) {
		f := true
		c.onAzure = &f
	}
}

func CollectorWithGetAzureMetadataFunc(fn GetAzureMetadataFunc) CollectorOption {
	return func(c *collector) {
		c.getAzureMetadataFunc = fn
	}
}

func CollectorWithMetadataContainerInitFunc(fn func() metadatax.MetadataContainer) CollectorOption {
	return func(c *collector) {
		c.mdContainerInitFunc = fn
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

	if c.getAzureMetadataFunc == nil {
		c.getAzureMetadataFunc = func(ctx context.Context) AzureMetadata {
			return NewAzureMetadataClient()
		}
	}

	return c
}

func (c *collector) GetMetadata(ctx context.Context) (metadatax.MetadataContainer, error) {
	md := c.mdContainerInitFunc()

	if !c.isOnAzure() {
		return md, nil
	}

	data := c.getAzureMetadataFunc(ctx)

	instance, err := data.GetInstanceMetadata(ctx)
	if err != nil {
		return md, err
	}

	lb, err := data.GetLoadBalancerMetadata(ctx)
	if err != nil {
		return md, err
	}

	getters := []func(metadatax.MetadataContainer, *AzureMetadataInstance, *AzureMetadataLoadBalancer){
		c.base,
		c.placement,
		c.vm,
		c.tags,
		c.network,
	}

	for _, f := range getters {
		f(md, instance, lb)
	}

	return md, nil
}

func (c *collector) base(md metadatax.MetadataContainer, instance *AzureMetadataInstance, lb *AzureMetadataLoadBalancer) {
	md.AddLabel("name", instance.Compute.Name)
	md.AddLabel("ostype", instance.Compute.OsType)
	md.AddLabel("priority", instance.Compute.Priority)
	md.AddLabel("provider", instance.Compute.Provider)
	md.AddLabel("sku", instance.Compute.Sku)

	md.Segment("resourcegroup").AddLabel("name", instance.Compute.ResourceGroupName)
	md.Segment("subscription").AddLabel("id", instance.Compute.SubscriptionID)
}

func (c *collector) placement(md metadatax.MetadataContainer, instance *AzureMetadataInstance, lb *AzureMetadataLoadBalancer) {
	md.Segment("placement").
		AddLabel("location", instance.Compute.Location).
		AddLabel("groupid", instance.Compute.PlacementGroupID).
		AddLabel("zone", instance.Compute.Zone)
}

func (c *collector) vm(md metadatax.MetadataContainer, instance *AzureMetadataInstance, lb *AzureMetadataLoadBalancer) {
	md.Segment("vm").
		AddLabel("size", instance.Compute.VMSize).
		AddLabel("id", instance.Compute.VMID).
		AddLabel("publisher", instance.Compute.Publisher).
		AddLabel("version", instance.Compute.Version).
		AddLabel("offer", instance.Compute.Offer).
		Segment("scaleset").AddLabel("name", instance.Compute.VMScaleSetName)
}

func (c *collector) tags(md metadatax.MetadataContainer, instance *AzureMetadataInstance, lb *AzureMetadataLoadBalancer) {
	tmd := md.Segment("tag")
	for _, tag := range instance.Compute.TagsList {
		tmd.AddLabel(tag.Name, tag.Value)
	}
}

func (c *collector) network(md metadatax.MetadataContainer, instance *AzureMetadataInstance, lb *AzureMetadataLoadBalancer) {
	nmd := md.Segment("network")
	for _, iface := range instance.Network.Interface {
		nmd.AddLabel("mac", iface.MacAddress)

		for _, ip := range iface.IPv4.IPAddress {
			nmd.AddLabel("private-ipv4", ip.PrivateIpAddress)
			nmd.AddLabel("public-ipv4", ip.PublicIpAddress)
		}

		for _, ip := range iface.IPv6.IPAddress {
			nmd.AddLabel("private-ipv6", ip.PrivateIpAddress)
			nmd.AddLabel("public-ipv6", ip.PublicIpAddress)
		}
	}

	for _, ip := range lb.LoadBalancer.PublicIPAddresses {
		nmd.AddLabel("public-ipv4", ip.FrontendIpAddress)
	}
}

func (c *collector) isOnAzure() bool {
	if c.onAzure != nil {
		return *c.onAzure
	}

	isOnAzure := IsOnAzure()
	c.onAzure = &isOnAzure

	return isOnAzure
}

func IsOnAzure() bool {
	data, err := os.ReadFile("/sys/class/dmi/id/sys_vendor")
	if err != nil {
		return false
	}

	return bytes.Contains(data, []byte("Microsoft Corporation"))
}
