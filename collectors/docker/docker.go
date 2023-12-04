package docker

import (
	"context"
	"strings"

	"emperror.dev/errors"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"

	"github.com/gezacorp/metadatax"
)

const (
	name = "docker"
)

var (
	ContainerIDNotFoundError = errors.Sentinel("could not find container id for pid")
)

type collector struct {
	socketPath                    string
	dockerClientOpts              []client.Opt
	metadataGetter                MetadataGetter
	containerIDGetter             ContainerIDGetter
	ignoreMissingContainerIDError bool
	ignoreNoSuchContainerError    bool

	mdcontainerGetter func() metadatax.MetadataContainer
}

type MetadataGetter interface {
	ContainerInspect(ctx context.Context, containerID string) (types.ContainerJSON, error)
}

type ContainerIDGetter interface {
	GetContainerIDFromPID(pid int) (string, error)
}

type CollectorOption func(*collector)

func WithDockerClientOpts(opts ...client.Opt) CollectorOption {
	return func(c *collector) {
		c.dockerClientOpts = opts
	}
}

func WithSocketPath(socketPath string) CollectorOption {
	return func(c *collector) {
		c.socketPath = socketPath
	}
}

func WithMetadataGetter(metadataGetter MetadataGetter) CollectorOption {
	return func(c *collector) {
		c.metadataGetter = metadataGetter
	}
}

func WithContainerIDGetter(containerIDGetter ContainerIDGetter) CollectorOption {
	return func(c *collector) {
		c.containerIDGetter = containerIDGetter
	}
}

func WithIgnoreMissingContainerIDError() CollectorOption {
	return func(c *collector) {
		c.ignoreMissingContainerIDError = true
	}
}

func WithIgnoreNoSuchContainerError() CollectorOption {
	return func(c *collector) {
		c.ignoreNoSuchContainerError = true
	}
}

func CollectorWithMetadataContainerGetter(getter func() metadatax.MetadataContainer) CollectorOption {
	return func(c *collector) {
		c.mdcontainerGetter = getter
	}
}

func New(opts ...CollectorOption) (metadatax.Collector, error) {
	c := &collector{}

	for _, f := range opts {
		f(c)
	}

	if c.metadataGetter == nil {
		if dc, err := c.getDockerClient(); err != nil {
			return nil, errors.WrapIf(err, "could not get docker client")
		} else {
			c.metadataGetter = dc
		}
	}

	if c.containerIDGetter == nil {
		c.containerIDGetter = c
	}

	if c.mdcontainerGetter == nil {
		c.mdcontainerGetter = func() metadatax.MetadataContainer {
			return metadatax.New(metadatax.WithPrefix(name))
		}
	}

	return c, nil
}

func (c *collector) GetMetadata(ctx context.Context) (metadatax.MetadataContainer, error) {
	md := c.mdcontainerGetter()

	pid, found := metadatax.PIDFromContext(ctx)
	if !found {
		return nil, metadatax.PIDNotFoundError
	}

	containerID, err := c.containerIDGetter.GetContainerIDFromPID(int(pid))
	if err != nil {
		return nil, errors.WrapIfWithDetails(err, "could not get cgroups from pid", "pid", pid)
	}
	if containerID == "" {
		if c.ignoreMissingContainerIDError {
			return md, nil
		}

		return nil, errors.WithDetails(ContainerIDNotFoundError, "pid", pid)
	}

	containerJSON, err := c.metadataGetter.ContainerInspect(ctx, containerID)
	if c.ignoreNoSuchContainerError && client.IsErrNotFound(err) {
		return md, nil
	}
	if err != nil {
		return nil, err
	}

	getters := []func(types.ContainerJSON, metadatax.MetadataContainer){
		c.base,
		c.labels,
		c.envs,
		c.image,
		c.network,
	}

	for _, f := range getters {
		f(containerJSON, md)
	}

	return md, nil
}

func (c *collector) GetContainerIDFromPID(pid int) (string, error) {
	cgroups, err := GetCgroupsForPID(pid)
	if err != nil {
		return "", errors.WithStackIf(err)
	}

	return GetContainerIDFromCgroups(cgroups), nil
}

func (c *collector) base(containerJSON types.ContainerJSON, md metadatax.MetadataContainer) {
	md.AddLabel("id", containerJSON.ID)
	md.AddLabel("name", strings.TrimLeft(containerJSON.Name, "/"))
	md.AddLabel("cmdline", containerJSON.Path+" "+strings.Join(containerJSON.Args, " "))
}

func (c *collector) envs(containerJSON types.ContainerJSON, md metadatax.MetadataContainer) {
	emd := md.Segment("env")
	for _, env := range containerJSON.Config.Env {
		if !strings.Contains(env, "=") {
			continue
		}
		p := strings.SplitN(env, "=", 2)
		emd.AddLabel(strings.ToUpper(p[0]), p[1])
	}
}

func (c *collector) labels(containerJSON types.ContainerJSON, md metadatax.MetadataContainer) {
	lmd := md.Segment("label")
	for k, v := range containerJSON.Config.Labels {
		lmd.AddLabel(k, v)
	}
}

func (c *collector) image(containerJSON types.ContainerJSON, md metadatax.MetadataContainer) {
	md.Segment("image").
		AddLabel("name", containerJSON.Config.Image).
		AddLabel("hash", containerJSON.Image)
}

func (c *collector) network(containerJSON types.ContainerJSON, md metadatax.MetadataContainer) {
	nmd := md.Segment("network")
	nmd.AddLabel("mode", string(containerJSON.HostConfig.NetworkMode))
	nmd.AddLabel("hostname", containerJSON.Config.Hostname)
	for port := range containerJSON.HostConfig.PortBindings {
		md.AddLabel("port-binding", string(port))
	}
}

func (c *collector) getDockerClient() (*client.Client, error) {
	var opts []client.Opt
	if c.socketPath != "" {
		opts = append(opts, client.WithHost(c.socketPath))
	}
	opts = append(opts, client.WithAPIVersionNegotiation())

	if c.dockerClientOpts != nil {
		opts = append(opts, c.dockerClientOpts...)
	}

	return client.NewClientWithOpts(opts...)
}
