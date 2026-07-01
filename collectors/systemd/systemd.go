package systemd

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"emperror.dev/errors"

	"github.com/gezacorp/metadatax"
	"github.com/shirou/gopsutil/v4/process"
)

const (
	name = "systemd"

	basePath = "/run/systemd/system"
)

var ErrUnitNotFound = errors.Sentinel("could not find systemd unit for pid")

type UnitNameGetter interface {
	GetUnitNameForPID(pid int) (string, error)
}

type UnitPropertiesGetter interface {
	GetUnitProperties(ctx context.Context, unitName string) (map[string]any, error)
}

type unitNameGetterFunc func(pid int) (string, error)

func (f unitNameGetterFunc) GetUnitNameForPID(pid int) (string, error) {
	return f(pid)
}

type collector struct {
	unitNameGetter       UnitNameGetter
	unitPropertiesGetter UnitPropertiesGetter

	mdContainerInitFunc func() metadatax.MetadataContainer
	skipOnSoftError     bool
	extractEnvs         bool
	hasSystemd          bool
}

type CollectorOption func(*collector)

func WithUnitNameGetter(getter UnitNameGetter) CollectorOption {
	return func(c *collector) {
		c.unitNameGetter = getter
	}
}

func WithUnitPropertiesGetter(getter UnitPropertiesGetter) CollectorOption {
	return func(c *collector) {
		c.unitPropertiesGetter = getter
	}
}

func CollectorWithMetadataContainerInitFunc(fn func() metadatax.MetadataContainer) CollectorOption {
	return func(c *collector) {
		c.mdContainerInitFunc = fn
	}
}

func WithForceHasSystemd() CollectorOption {
	return func(c *collector) {
		c.hasSystemd = true
	}
}

func WithSkipOnSoftError() CollectorOption {
	return func(c *collector) {
		c.skipOnSoftError = true
	}
}

func CollectorWithExtractENVs() CollectorOption {
	return func(c *collector) {
		c.extractEnvs = true
	}
}

func New(opts ...CollectorOption) metadatax.Collector {
	c := &collector{}

	for _, f := range opts {
		f(c)
	}

	if c.unitNameGetter == nil {
		c.unitNameGetter = unitNameGetterFunc(GetUnitNameForPID)
	}

	if c.unitPropertiesGetter == nil {
		c.unitPropertiesGetter = dbusUnitPropertiesGetter{}
	}

	if c.mdContainerInitFunc == nil {
		c.mdContainerInitFunc = func() metadatax.MetadataContainer {
			return metadatax.New(metadatax.WithPrefix(name))
		}
	}

	return c
}

func (c *collector) GetMetadata(ctx context.Context) (metadatax.MetadataContainer, error) {
	md := c.mdContainerInitFunc()

	if !c.hasSystemFS() {
		return md, nil
	}

	pid, found := metadatax.PIDFromContext(ctx)
	if !found {
		return nil, metadatax.ErrPIDNotFound
	}

	unitName, err := c.unitNameGetter.GetUnitNameForPID(int(pid))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			if c.skipOnSoftError {
				return md, nil
			}

			return nil, process.ErrorProcessNotRunning
		}

		if c.skipOnSoftError {
			return md, nil
		}

		return nil, errors.WrapIfWithDetails(err, "could not get systemd unit for pid", "pid", pid)
	}

	if unitName == "" {
		if c.skipOnSoftError {
			return md, nil
		}

		return nil, errors.WithDetails(ErrUnitNotFound, "pid", pid)
	}

	md.AddLabel("unit", unitName)

	properties, err := c.unitPropertiesGetter.GetUnitProperties(ctx, unitName)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			if c.skipOnSoftError {
				return md, nil
			}

			return nil, process.ErrorProcessNotRunning
		}

		if c.skipOnSoftError {
			return md, nil
		}

		return nil, errors.WrapIfWithDetails(err, "could not get systemd unit properties", "unit", unitName)
	}

	getters := []func(map[string]any, metadatax.MetadataContainer){
		c.base,
		c.identity,
	}

	if c.extractEnvs {
		getters = append(getters, c.envs)
	}

	for _, f := range getters {
		f(properties, md)
	}

	return md, nil
}

func (c *collector) base(properties map[string]any, md metadatax.MetadataContainer) {
	md.AddLabel("description", stringProperty(properties, "Description"))
	md.AddLabel("type", stringProperty(properties, "Type"))
	md.AddLabel("load-state", stringProperty(properties, "LoadState"))
	md.AddLabel("active-state", stringProperty(properties, "ActiveState"))
	md.AddLabel("sub-state", stringProperty(properties, "SubState"))
	md.AddLabel("unit-file-state", stringProperty(properties, "UnitFileState"))
	md.AddLabel("result", stringProperty(properties, "Result"))
	md.AddLabel("fragment-path", stringProperty(properties, "FragmentPath"))
	md.AddLabel("slice", stringProperty(properties, "Slice"))
	md.AddLabel("invocation-id", invocationIDProperty(properties))
	md.AddLabel("main-pid", uint32Property(properties, "MainPID"))
	md.AddLabel("restarts", uint32Property(properties, "NRestarts"))
	md.AddLabel("started-at", timestampProperty(properties, "ExecMainStartTimestamp"))
}

// identity adds unit-type-specific properties that describe what the unit
// is declared to run as, rather than how its current instance is running.
// These are meant to stay stable across restarts of the same unit and are
// intended for use as components of a workload identity.
func (c *collector) identity(properties map[string]any, md metadatax.MetadataContainer) {
	md.AddLabel("user", stringProperty(properties, "User"))
	md.AddLabel("group", stringProperty(properties, "Group"))
	md.AddLabel("dynamic-user", boolProperty(properties, "DynamicUser"))
	md.AddLabel("capability-bounding-set", capabilityBoundingSetProperty(properties))
	md.AddLabel("exec-start", execStartCommand(properties))
}

func (c *collector) envs(properties map[string]any, md metadatax.MetadataContainer) {
	envmd := md.Segment("env")

	envs, ok := properties["Environment"].([]string)
	if !ok {
		return
	}

	for _, env := range envs {
		if !strings.Contains(env, "=") {
			continue
		}
		parts := strings.SplitN(env, "=", 2)
		envmd.AddLabel(strings.ToUpper(parts[0]), parts[1])
	}
}

func stringProperty(properties map[string]any, name string) string {
	if v, ok := properties[name].(string); ok {
		return v
	}

	return ""
}

func uint32Property(properties map[string]any, name string) string {
	v, ok := properties[name].(uint32)
	if !ok {
		return ""
	}

	return strconv.FormatUint(uint64(v), 10)
}

func boolProperty(properties map[string]any, name string) string {
	v, ok := properties[name].(bool)
	if !ok {
		return ""
	}

	return strconv.FormatBool(v)
}

// capabilityBoundingSetProperty formats CapabilityBoundingSet, which systemd
// exposes as a raw uint64 capability bitmask, as hex. Decoding it into
// individual capability names would need a capability-number-to-name table;
// the raw mask is still useful as an identity component since it is stable
// for a given unit's declared configuration.
func capabilityBoundingSetProperty(properties map[string]any) string {
	v, ok := properties["CapabilityBoundingSet"].(uint64)
	if !ok {
		return ""
	}

	return fmt.Sprintf("0x%x", v)
}

// execStartCommand formats the ExecStart service property, an array of
// (path, argv, ...) tuples, into the command line of its first entry.
func execStartCommand(properties map[string]any) string {
	entries, ok := properties["ExecStart"].([]any)
	if !ok || len(entries) == 0 {
		return ""
	}

	entry, ok := entries[0].([]any)
	if !ok || len(entry) < 2 {
		return ""
	}

	if argv, ok := entry[1].([]string); ok && len(argv) > 0 {
		return strings.Join(argv, " ")
	}

	path, ok := entry[0].(string)
	if !ok {
		return ""
	}

	return path
}

func invocationIDProperty(properties map[string]any) string {
	v, ok := properties["InvocationID"].([]byte)
	if !ok || len(v) == 0 {
		return ""
	}

	return fmt.Sprintf("%x", v)
}

// timestampProperty formats a systemd timestamp property, which is expressed
// as microseconds since the Unix epoch (0 meaning "never").
func timestampProperty(properties map[string]any, name string) string {
	v, ok := properties[name].(uint64)
	if !ok || v == 0 {
		return ""
	}

	return time.UnixMicro(int64(v)).UTC().Format(time.RFC3339)
}

func (c *collector) hasSystemFS() bool {
	if c.hasSystemd {
		return true
	}

	v := HasSystemd()
	if v {
		c.hasSystemd = true
	}

	return v
}

func HasSystemd() bool {
	info, err := os.Stat(basePath)
	if err != nil {
		return false
	}

	return info.IsDir()
}
