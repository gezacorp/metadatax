package proxmox

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strconv"

	"emperror.dev/errors"
	"github.com/prometheus/procfs"

	"github.com/gezacorp/metadatax"
)

const name = "proxmox"

// guestType identifies which kind of Proxmox guest a host-visible cgroup
// belongs to.
type guestType string

const (
	guestTypeLXC  guestType = "lxc"
	guestTypeQemu guestType = "qemu"
)

// lxcCgroupRegex matches the cgroup path Proxmox assigns an LXC container's
// processes to, confirmed live against a running container: "/lxc/100/ns/init.scope".
var lxcCgroupRegex = regexp.MustCompile(`/lxc/(\d+)(?:/|$)`)

// qemuCgroupRegex matches the cgroup path Proxmox assigns a QEMU VM's main
// process to. Proxmox's qemu-server launches VMs via "systemd-run --scope
// --slice=qemu -u <vmid>.scope", which is documented behavior but NOT
// verified live in this environment (no VM was running on the investigated
// node to confirm it).
var qemuCgroupRegex = regexp.MustCompile(`/qemu\.slice/(\d+)\.scope`)

var errNotAProxmoxGuest = errors.Sentinel("pid does not belong to a proxmox guest")

// collector runs on a Proxmox VE node (host). Given a PID, it resolves
// which VM/CT that PID belongs to via its cgroup path, then fetches guest
// and node metadata (tags, resource config, network, ...) through the
// Proxmox API via guestInspector.
type collector struct {
	pveRoot         string
	getProcFunc     func(pid int) (procfs.Proc, error)
	guestInspector  GuestInspector
	skipOnSoftError bool

	mdContainerInitFunc func() metadatax.MetadataContainer
}

type CollectorOption func(*collector)

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

// WithGuestInspector configures how guest/node metadata is fetched once a
// PID has been resolved to a (node, vmid, type). Use NewAPIGuestInspector
// with credentials to talk to the real Proxmox API; without it, only
// vmid/type/node are reported.
func WithGuestInspector(inspector GuestInspector) CollectorOption {
	return func(c *collector) {
		c.guestInspector = inspector
	}
}

// withPVERoot lets tests point node-name resolution at a fixture tree
// instead of the real /etc/pve. Unexported: production callers always use
// /etc/pve.
func withPVERoot(path string) CollectorOption {
	return func(c *collector) {
		c.pveRoot = path
	}
}

// New returns a collector meant to run on a Proxmox VE node (host). Given a
// PID, it resolves which VM/CT that PID belongs to via its cgroup path,
// then, if a GuestInspector is configured (see WithGuestInspector), fetches
// rich guest and node metadata through the Proxmox API.
//
// Without an explicit WithGuestInspector, New looks for PROXMOX_TOKEN_ID and
// PROXMOX_TOKEN_SECRET env vars (plus optional PROXMOX_BASE_URL) and builds
// one automatically via NewAPIGuestInspector when both are set, the same
// way docker.New auto-detects its socket path. If they're unset, only
// vmid/type/node-name are reported.
func New(opts ...CollectorOption) metadatax.Collector {
	c := &collector{
		pveRoot: "/etc/pve",
	}

	for _, f := range opts {
		f(c)
	}

	if c.getProcFunc == nil {
		c.getProcFunc = defaultGetProc
	}

	if c.guestInspector == nil {
		c.guestInspector = guestInspectorFromEnv()
	}

	if c.mdContainerInitFunc == nil {
		c.mdContainerInitFunc = func() metadatax.MetadataContainer {
			return metadatax.New(metadatax.WithPrefix(name))
		}
	}

	return c
}

// guestInspectorFromEnv builds a GuestInspector from PROXMOX_TOKEN_ID /
// PROXMOX_TOKEN_SECRET / PROXMOX_BASE_URL when the required token env vars
// are set, or returns nil otherwise.
func guestInspectorFromEnv() GuestInspector {
	tokenID := os.Getenv("PROXMOX_TOKEN_ID")
	tokenSecret := os.Getenv("PROXMOX_TOKEN_SECRET")
	if tokenID == "" || tokenSecret == "" {
		return nil
	}

	return NewAPIGuestInspector(os.Getenv("PROXMOX_BASE_URL"), tokenID, tokenSecret)
}

func defaultGetProc(pid int) (procfs.Proc, error) {
	hostProc := os.Getenv("HOST_PROC")
	if hostProc == "" {
		return procfs.NewProc(pid)
	}

	fs, err := procfs.NewFS(hostProc)
	if err != nil {
		return procfs.Proc{}, errors.WrapIf(err, "could not create a new procfs")
	}

	return fs.Proc(pid)
}

func (c *collector) GetMetadata(ctx context.Context) (metadatax.MetadataContainer, error) {
	md := c.mdContainerInitFunc()

	pid, found := metadatax.PIDFromContext(ctx)
	if !found {
		return nil, metadatax.PIDNotFoundError
	}

	vmid, gtype, err := c.resolveGuestFromCgroup(int(pid))
	if err != nil {
		if errors.Is(err, errNotAProxmoxGuest) {
			return md, nil
		}

		if c.skipOnSoftError {
			return md, nil
		}

		return nil, errors.WrapIfWithDetails(err, "could not resolve proxmox guest from cgroup", "pid", pid)
	}

	gmd := md.Segment("guest")
	gmd.AddLabel("vmid", vmid)
	gmd.AddLabel("type", string(gtype))

	node, err := localNodeName(c.pveRoot)
	if err == nil {
		md.Segment("node").AddLabel("name", node)
	}

	if node != "" && c.guestInspector != nil {
		if info, err := c.inspectGuest(ctx, node, gtype, vmid); err == nil {
			applyGuestInfo(gmd, gtype, info)
			applyNodeInfo(md.Segment("node"), info.Node)
		} else if !c.skipOnSoftError {
			return nil, errors.WrapIfWithDetails(err, "could not inspect proxmox guest", "node", node, "vmid", vmid)
		}
	}

	return md, nil
}

func (c *collector) inspectGuest(ctx context.Context, node string, gtype guestType, vmid string) (*GuestInfo, error) {
	vmidInt, err := strconv.Atoi(vmid)
	if err != nil {
		return nil, err
	}

	if gtype == guestTypeQemu {
		return c.guestInspector.InspectVM(ctx, node, vmidInt)
	}

	return c.guestInspector.InspectContainer(ctx, node, vmidInt)
}

func (c *collector) resolveGuestFromCgroup(pid int) (string, guestType, error) {
	proc, err := c.getProcFunc(pid)
	if err != nil {
		return "", "", errors.WrapIf(err, "could not get process info")
	}

	cgroups, err := proc.Cgroups()
	if err != nil {
		return "", "", errors.WrapIf(err, "could not get cgroups")
	}

	for _, cgroup := range cgroups {
		if match := lxcCgroupRegex.FindStringSubmatch(cgroup.Path); len(match) > 1 {
			return match[1], guestTypeLXC, nil
		}

		if match := qemuCgroupRegex.FindStringSubmatch(cgroup.Path); len(match) > 1 {
			return match[1], guestTypeQemu, nil
		}
	}

	return "", "", errNotAProxmoxGuest
}

// localNodeName resolves this node's name from the pmxcfs "local" symlink,
// e.g. /etc/pve/local -> nodes/proxmox.
func localNodeName(pveRoot string) (string, error) {
	target, err := os.Readlink(filepath.Join(pveRoot, "local"))
	if err != nil {
		return "", err
	}

	return filepath.Base(target), nil
}

func applyGuestInfo(gmd metadatax.MetadataContainer, gtype guestType, info *GuestInfo) {
	gmd.AddLabel("name", info.Name)
	gmd.AddLabel("ostype", info.OSType)
	gmd.AddLabel("status", info.Status)
	gmd.AddLabel("description", info.Description)
	gmd.AddLabel("pool", info.Pool)
	gmd.AddLabel("digest", info.Digest)
	gmd.AddLabel("lock", info.Lock)
	gmd.AddLabel("ha-state", info.HAState)
	gmd.AddLabel("mac", info.MACAddress)
	gmd.AddLabel("bridge", info.Bridge)

	if info.Cores > 0 {
		gmd.AddLabel("cores", strconv.Itoa(info.Cores))
	}

	if info.Memory > 0 {
		gmd.AddLabel("memory", strconv.Itoa(info.Memory))
	}

	if gtype == guestTypeLXC {
		gmd.AddLabel("unprivileged", strconv.FormatBool(info.Unprivileged))
	}

	gmd.AddLabel("template", strconv.FormatBool(info.Template))
	gmd.AddLabel("onboot", strconv.FormatBool(info.OnBoot))
	gmd.AddLabel("protection", strconv.FormatBool(info.Protection))

	if len(info.Tags) > 0 {
		tmd := gmd.Segment("tag")
		tmd.AddLabel("", info.Tags...)
	}

	if len(info.IPAddresses) > 0 {
		gmd.AddLabel("ip", info.IPAddresses...)
	}
}

func applyNodeInfo(nmd metadatax.MetadataContainer, info NodeInfo) {
	nmd.AddLabel("pve-version", info.PVEVersion)
	nmd.AddLabel("kernel", info.KernelVersion)
	nmd.AddLabel("cpu-model", info.CPUModel)
	nmd.AddLabel("ssl-fingerprint", info.SSLFingerprint)

	if info.ID != nil {
		nmd.AddLabel("id", strconv.Itoa(*info.ID))
	}

	if info.CPUCores > 0 {
		nmd.AddLabel("cpu-cores", strconv.Itoa(info.CPUCores))
	}

	if info.CPUSockets > 0 {
		nmd.AddLabel("cpu-sockets", strconv.Itoa(info.CPUSockets))
	}

	if info.MemoryTotalBytes > 0 {
		nmd.AddLabel("memory-total", strconv.FormatUint(info.MemoryTotalBytes, 10))
	}
}
