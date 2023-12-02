package gcp

import (
	"bytes"
	"context"
	"os"
	"strconv"
	"strings"

	"github.com/gezacorp/metadatax"
)

const (
	name = "gcp"
)

type collector struct {
	metadataGetter MetadataGetter
	onGoogle       bool
	mdcontainer    metadatax.MetadataContainer
}

type CollectorOption func(*collector)

type MetadataGetter interface {
	GetInstanceMetadata(ctx context.Context) (*GCPMetadataInstance, error)
}

func CollectorWithForceOnGoogle() CollectorOption {
	return func(c *collector) {
		c.onGoogle = true
	}
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
		c.metadataGetter = NewGCPMetadataGetter()
	}

	return c
}

func (c *collector) GetMetadata(ctx context.Context) (metadatax.MetadataContainer, error) {
	if !c.isOnGoogle() {
		return c.mdcontainer, nil
	}

	instance, err := c.metadataGetter.GetInstanceMetadata(ctx)
	if err != nil {
		return c.mdcontainer, err
	}

	getters := []func(metadatax.MetadataContainer, *GCPMetadataInstance){
		c.base,
		c.placement,
		c.scheduling,
		c.network,
		c.serviceaccount,
	}

	for _, f := range getters {
		f(c.mdcontainer, instance)
	}

	return c.mdcontainer, nil
}

func (c *collector) base(md metadatax.MetadataContainer, instance *GCPMetadataInstance) {
	md.AddLabel("id", strconv.Itoa(int(instance.ID)))
	md.AddLabel("name", instance.Name)
	md.AddLabel("cpu-platform", instance.CPUPlatform)

	attributes := metadatax.ConvertMapStringToLabels(instance.Attributes)
	// filter out ssh public keys
	delete(attributes, "ssh-keys")
	md.Segment("attributes").AddLabels(attributes)

	for _, tag := range instance.Tags {
		md.AddLabel("tag", tag)
	}

	if p := strings.Split(instance.Image, "/"); len(p) == 5 {
		md.Segment("image").
			AddLabel("project", p[1]).
			AddLabel("name", p[4])

	}

	if p := strings.Split(instance.MachineType, "/"); len(p) == 4 {
		md.Segment("machine").
			AddLabel("project", p[1]).
			AddLabel("type", p[3])
	}
}

func (c *collector) placement(md metadatax.MetadataContainer, instance *GCPMetadataInstance) {
	if p := strings.Split(instance.Zone, "/"); len(p) == 4 {
		var region string
		if i := strings.LastIndex(p[3], "-"); i > 0 {
			region = p[3][:i]
		}
		md.Segment("placement").
			AddLabel("project", p[1]).
			AddLabel("zone", p[3]).
			AddLabel("region", region)
	}
}

func (c *collector) scheduling(md metadatax.MetadataContainer, instance *GCPMetadataInstance) {
	md.Segment("scheduling").
		AddLabel("automatic-restart", strings.ToLower(instance.Scheduling.AutomaticRestart)).
		AddLabel("onHostMaintenance", strings.ToLower(instance.Scheduling.OnHostMaintenance)).
		AddLabel("preemptible", strings.ToLower(instance.Scheduling.Preemptible))
}

func (c *collector) network(md metadatax.MetadataContainer, instance *GCPMetadataInstance) {
	nmd := md.Segment("network")
	for _, iface := range instance.NetworkInterfaces {
		nmd.AddLabel("mac", iface.Mac)

		for _, ac := range iface.AccessConfigs {
			if strings.Contains(iface.IP, ":") {
				nmd.AddLabel("public-ipv6", ac.ExternalIP)
			} else {
				nmd.AddLabel("public-ipv4", ac.ExternalIP)
			}
		}

		if strings.Contains(iface.IP, ":") {
			nmd.AddLabel("private-ipv6", iface.IP)
		} else {
			nmd.AddLabel("private-ipv4", iface.IP)
		}
	}
}

func (c *collector) serviceaccount(md metadatax.MetadataContainer, instance *GCPMetadataInstance) {
	samd := md.Segment("serviceaccount")
	for name, sa := range instance.ServiceAccounts {
		sanmd := samd.Segment(name)
		for _, alias := range sa.Aliases {
			sanmd.AddLabel("alias", alias)
		}
		sanmd.AddLabel("email", sa.Email)
		for _, scope := range sa.Scopes {
			sanmd.AddLabel("scope", scope)
		}
	}
}

func (c *collector) isOnGoogle() bool {
	if c.onGoogle {
		return true
	}

	data, err := os.ReadFile("/sys/class/dmi/id/product_name")
	if err != nil {
		return false
	}

	return bytes.Contains(data, []byte("Google"))
}