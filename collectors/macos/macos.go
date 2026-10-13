package macos

import (
	"context"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"

	"emperror.dev/errors"

	"github.com/gezacorp/metadatax"
)

const (
	name = "macos"

	defaultCommandTimeout = 10 * time.Second
)

type collector struct {
	isDarwin bool

	getSysctlFunc       GetSysctlFunc
	getSysctlUint32Func GetSysctlUint32Func
	runCommand          CommandRunner
	commandTimeout      time.Duration

	mdContainerInitFunc func() metadatax.MetadataContainer
}

type CollectorOption func(*collector)

type (
	GetSysctlFunc       func(name string) (string, error)
	GetSysctlUint32Func func(name string) (uint32, error)
	// CommandRunner returns the standard output of a command, also on a non-zero exit status.
	CommandRunner func(ctx context.Context, name string, args ...string) ([]byte, error)
)

func CollectorWithGetSysctlFunc(fn GetSysctlFunc) CollectorOption {
	return func(c *collector) {
		c.getSysctlFunc = fn
	}
}

func CollectorWithGetSysctlUint32Func(fn GetSysctlUint32Func) CollectorOption {
	return func(c *collector) {
		c.getSysctlUint32Func = fn
	}
}

// CollectorWithCommandTimeout sets the maximum time a single command may run.
func CollectorWithCommandTimeout(timeout time.Duration) CollectorOption {
	return func(c *collector) {
		c.commandTimeout = timeout
	}
}

func CollectorWithCommandRunner(fn CommandRunner) CollectorOption {
	return func(c *collector) {
		c.runCommand = fn
	}
}

func CollectorWithMetadataContainerInitFunc(fn func() metadatax.MetadataContainer) CollectorOption {
	return func(c *collector) {
		c.mdContainerInitFunc = fn
	}
}

func CollectorWithForceDarwin() CollectorOption {
	return func(c *collector) {
		c.isDarwin = true
	}
}

func New(opts ...CollectorOption) metadatax.Collector {
	c := &collector{
		isDarwin: runtime.GOOS == "darwin",

		getSysctlFunc:       getSysctl,
		getSysctlUint32Func: getSysctlUint32,
		runCommand:          runCommand,
		commandTimeout:      defaultCommandTimeout,
	}

	for _, f := range opts {
		f(c)
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

	if !c.isDarwin {
		return md, nil
	}

	profile, err := c.getSystemProfile(ctx)
	if err != nil {
		return nil, err
	}
	hardware := profile.hardware()
	software := profile.software()

	// OS version, kernel release, architecture and hardware UUID are provided by the node collector
	md.AddLabel("build", c.sysctl("kern.osversion"))
	md.AddLabel("computer-name", software.LocalHostName)
	md.Segment("kernel").AddLabel("version", c.sysctl("kern.version"))

	hw := md.Segment("hardware")
	hw.AddLabel("serial", hardware.SerialNumber)
	// equals the hardware UUID on Intel Macs
	if !strings.EqualFold(hardware.ProvisioningUDID, hardware.PlatformUUID) {
		hw.AddLabel("provisioning-udid", hardware.ProvisioningUDID)
	}
	hw.Segment("model").
		AddLabel("name", hardware.MachineName).
		AddLabel("identifier", hardware.MachineModel).
		AddLabel("number", hardware.ModelNumber)
	hw.Segment("cpu").
		AddLabel("brand", c.sysctl("machdep.cpu.brand_string"))

	deviceType := DeviceTypeUnknown
	if vmmPresent, err := c.getSysctlUint32Func("kern.hv_vmm_present"); err == nil {
		isVirtual := vmmPresent == 1
		hw.AddLabel("virtual", strconv.FormatBool(isVirtual))
		deviceType = DetectDeviceType(hardware.MachineName, hardware.MachineModel, isVirtual)
	}
	md.Segment("device").AddLabel("type", string(deviceType))

	if enrollment, ok := c.getMDMEnrollment(ctx); ok {
		md.Segment("mdm").
			AddLabel("enrolled", strconv.FormatBool(enrollment.Enrolled)).
			AddLabel("dep", strconv.FormatBool(enrollment.DEP)).
			AddLabel("server", enrollment.ServerHost)
	}

	return md, nil
}

func (c *collector) sysctl(name string) string {
	value, err := c.getSysctlFunc(name)
	if err != nil {
		return ""
	}

	return value
}

func (c *collector) command(ctx context.Context, name string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, c.commandTimeout)
	defer cancel()

	return c.runCommand(ctx, name, args...)
}

func runCommand(ctx context.Context, name string, args ...string) ([]byte, error) {
	output, err := exec.CommandContext(ctx, name, args...).Output()

	return output, errors.WrapIfWithDetails(err, "could not run command", "command", name)
}
