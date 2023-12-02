package azure

import (
	"context"

	"github.com/gezacorp/metadatax"
)

const (
	name = "azure"
)

type collector struct {
	metadataGetter MetadataGetter
	mdcontainer    metadatax.MetadataContainer
}

type CollectorOption func(*collector)

type MetadataGetter interface {
	GetInstanceMetadata(ctx context.Context) (*AzureMetadataInstance, error)
	GetLoadBalancerMetadata(ctx context.Context) (*AzureMetadataLoadBalancer, error)
}

func CollectorWithMetadataGetter(metadataGetter MetadataGetter) CollectorOption {
	return func(c *collector) {
		c.metadataGetter = metadataGetter
	}
}

func CollectorWithMetadataContainer(mdcontainer metadatax.MetadataContainer) CollectorOption {
	return func(c *collector) {
		c.mdcontainer = mdcontainer
	}
}

func New(opts ...CollectorOption) metadatax.Collector {
	c := &collector{}

	for _, f := range opts {
		f(c)
	}

	if c.mdcontainer == nil {
		c.mdcontainer = metadatax.New(metadatax.WithPrefix(name))
	}

	if c.metadataGetter == nil {
		c.metadataGetter = NewAzureMetadataGetter()
	}

	return c
}

func (c *collector) GetMetadata(ctx context.Context) (metadatax.MetadataContainer, error) {
	instance, err := c.metadataGetter.GetInstanceMetadata(ctx)
	if err != nil {
		return c.mdcontainer, err
	}

	lb, err := c.metadataGetter.GetLoadBalancerMetadata(ctx)
	if err != nil {
		return c.mdcontainer, err
	}

	getters := []func(metadatax.MetadataContainer, *AzureMetadataInstance, *AzureMetadataLoadBalancer){
		c.base,
		c.placement,
		c.vm,
		c.tags,
		c.network,
	}

	for _, f := range getters {
		f(c.mdcontainer, instance, lb)
	}

	return c.mdcontainer, nil
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
	tmd := c.mdcontainer.Segment("tag")
	for _, tag := range instance.Compute.TagsList {
		tmd.AddLabel(tag.Name, tag.Value)
	}
}

func (c *collector) network(md metadatax.MetadataContainer, instance *AzureMetadataInstance, lb *AzureMetadataLoadBalancer) {
	nmd := c.mdcontainer.Segment("network")
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
