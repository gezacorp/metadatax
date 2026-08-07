package docker

import (
	"context"
	"io"
	"os"
	"strings"

	"emperror.dev/errors"
	cerrdefs "github.com/containerd/errdefs"
	"github.com/docker/cli/cli/command"
	"github.com/docker/cli/cli/config"
	docker_ctx "github.com/docker/cli/cli/context/docker"
	"github.com/docker/cli/cli/context/store"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"

	"github.com/gezacorp/metadatax"
)

const (
	name = "docker"

	defaultSocketPath = "unix:///var/run/docker.sock"
)

var ContainerIDNotFoundError = errors.Sentinel("could not find container id for pid")

type collector struct {
	socketPath         string
	dockerClientOpts   []client.Opt
	containerInspector ContainerInspector
	containerIDGetter  ContainerIDGetter

	mdContainerInitFunc func() metadatax.MetadataContainer
	skipOnSoftError     bool
	hasDocker           *bool
}

type ContainerInspector interface {
	ContainerInspect(ctx context.Context, containerID string) (container.InspectResponse, error)
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

func WithContainerInspector(inspector ContainerInspector) CollectorOption {
	return func(c *collector) {
		c.containerInspector = inspector
	}
}

func WithContainerIDGetter(containerIDGetter ContainerIDGetter) CollectorOption {
	return func(c *collector) {
		c.containerIDGetter = containerIDGetter
	}
}

func CollectorWithMetadataContainerInitFunc(fn func() metadatax.MetadataContainer) CollectorOption {
	return func(c *collector) {
		c.mdContainerInitFunc = fn
	}
}

func WithSkipOnSoftError() CollectorOption {
	return func(c *collector) {
		c.skipOnSoftError = true
	}
}

func WithForceHasDocker() CollectorOption {
	return func(c *collector) {
		hasDocker := true
		c.hasDocker = &hasDocker
	}
}

func New(opts ...CollectorOption) metadatax.Collector {
	c := &collector{}

	for _, f := range opts {
		f(c)
	}

	if c.socketPath == "" {
		if host, err := GetCurrentContextHost(); err == nil {
			c.socketPath = host
		}
	}

	if c.socketPath == "" {
		c.socketPath = defaultSocketPath
	}

	if c.containerIDGetter == nil {
		c.containerIDGetter = c
	}

	if c.mdContainerInitFunc == nil {
		c.mdContainerInitFunc = func() metadatax.MetadataContainer {
			return metadatax.New(metadatax.WithPrefix(name))
		}
	}

	return c
}

func (c *collector) HasDocker() bool {
	if c.hasDocker != nil {
		return *c.hasDocker
	}

	ret := c.isSocketPathExists(c.socketPath)
	c.hasDocker = &ret

	return ret
}

func (c *collector) isSocketPathExists(path string) bool {
	path = strings.TrimPrefix(path, "unix://")

	if _, err := os.Open(path); errors.Is(err, os.ErrPermission) {
		return false
	}

	info, err := os.Stat(path)
	if err != nil {
		return false
	}

	return (info.Mode() & os.ModeSocket) != 0
}

func (c *collector) GetMetadata(ctx context.Context) (metadatax.MetadataContainer, error) {
	md := c.mdContainerInitFunc()

	if !c.HasDocker() {
		return md, nil
	}

	if c.containerInspector == nil {
		dockerClient, err := c.getDockerClient()
		if err != nil {
			return nil, errors.WrapIf(err, "could not get docker client")
		}

		c.containerInspector = &clientContainerInspector{client: dockerClient}
	}

	pid, found := metadatax.PIDFromContext(ctx)
	if !found {
		return nil, metadatax.ErrPIDNotFound
	}

	containerID, err := c.containerIDGetter.GetContainerIDFromPID(int(pid))
	if err != nil {
		if c.skipOnSoftError {
			return md, nil
		}

		return nil, errors.WrapIfWithDetails(err, "could not get cgroups from pid", "pid", pid)
	}

	if containerID == "" {
		if c.skipOnSoftError {
			return md, nil
		}

		return nil, errors.WithDetails(ContainerIDNotFoundError, "pid", pid)
	}

	containerJSON, err := c.containerInspector.ContainerInspect(ctx, containerID)
	if c.skipOnSoftError && cerrdefs.IsNotFound(err) {
		return md, nil
	}

	if err != nil {
		return nil, err
	}

	getters := []func(container.InspectResponse, metadatax.MetadataContainer){
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

func (c *collector) base(containerJSON container.InspectResponse, md metadatax.MetadataContainer) {
	md.AddLabel("id", containerJSON.ID)
	md.AddLabel("name", strings.TrimLeft(containerJSON.Name, "/"))
	md.AddLabel("cmdline", containerJSON.Path+" "+strings.Join(containerJSON.Args, " "))
}

func (c *collector) envs(containerJSON container.InspectResponse, md metadatax.MetadataContainer) {
	emd := md.Segment("env")
	for _, env := range containerJSON.Config.Env {
		if !strings.Contains(env, "=") {
			continue
		}
		p := strings.SplitN(env, "=", 2)
		emd.AddLabel(strings.ToUpper(p[0]), p[1])
	}
}

func (c *collector) labels(containerJSON container.InspectResponse, md metadatax.MetadataContainer) {
	lmd := md.Segment("label")
	for k, v := range containerJSON.Config.Labels {
		lmd.AddLabel(k, v)
	}
}

func (c *collector) image(containerJSON container.InspectResponse, md metadatax.MetadataContainer) {
	md.Segment("image").
		AddLabel("name", containerJSON.Config.Image).
		AddLabel("hash", containerJSON.Image)
}

func (c *collector) network(containerJSON container.InspectResponse, md metadatax.MetadataContainer) {
	nmd := md.Segment("network")
	nmd.AddLabel("mode", string(containerJSON.HostConfig.NetworkMode))
	nmd.AddLabel("hostname", containerJSON.Config.Hostname)
	for port := range containerJSON.HostConfig.PortBindings {
		md.AddLabel("port-binding", port.String())
	}
}

func (c *collector) getDockerClient() (*client.Client, error) {
	var opts []client.Opt
	if c.socketPath != "" {
		opts = append(opts, client.WithHost(c.socketPath))
	}

	if c.dockerClientOpts != nil {
		opts = append(opts, c.dockerClientOpts...)
	}

	return client.New(opts...)
}

type clientContainerInspector struct {
	client *client.Client
}

func (c *clientContainerInspector) ContainerInspect(ctx context.Context, containerID string) (container.InspectResponse, error) {
	result, err := c.client.ContainerInspect(ctx, containerID, client.ContainerInspectOptions{})
	if err != nil {
		return container.InspectResponse{}, err
	}

	return result.Container, nil
}

func GetCurrentContextHost() (string, error) {
	getContextName := func() string {
		cfg := config.LoadDefaultConfigFile(io.Discard)

		if ctxName := os.Getenv(command.EnvOverrideContext); ctxName != "" {
			return ctxName
		}

		if cfg != nil && cfg.CurrentContext != "" {
			return cfg.CurrentContext
		}

		return command.DefaultContextName
	}

	currentContextName := getContextName()
	if currentContextName == command.DefaultContextName {
		if host := os.Getenv(client.EnvOverrideHost); host != "" {
			return host, nil
		}

		return client.DefaultDockerHost, nil
	}

	contextStore := store.New(config.ContextStoreDir(), command.DefaultContextStoreConfig())

	contexts, err := contextStore.List()
	if err != nil {
		return "", err
	}

	for _, ctx := range contexts {
		if ctx.Name != currentContextName {
			continue
		}

		if me, err := docker_ctx.EndpointFromContext(ctx); err == nil {
			return me.Host, nil
		}
	}

	return "", nil
}
