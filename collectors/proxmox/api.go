package proxmox

import (
	"context"
	"crypto/tls"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	goproxmox "github.com/luthermonson/go-proxmox"
)

const defaultBaseURL = "https://localhost:8006/api2/json"

// guestInfoCacheTTL bounds how long a fetched GuestInfo (and the cluster/
// node-wide facts backing it) is reused across InspectContainer/InspectVM
// calls. Many processes typically belong to the same guest, and many guests
// share the same node, so each would otherwise repeat the same round trips
// independently. This only helps when one apiGuestInspector instance is
// reused across many GetMetadata calls (e.g. a resident daemon iterating
// over PIDs) - a one-shot-per-process CLI that builds a fresh inspector
// every run gets no benefit, since the cache dies with the process.
const guestInfoCacheTTL = 30 * time.Second

// macAddressRegex recognizes a MAC address appearing as a bare value in a
// Proxmox net device string, used to find the MAC in QEMU's
// "<model>=<mac>,bridge=...,..." shape, which has no explicit "mac=" key.
var macAddressRegex = regexp.MustCompile(`^([0-9A-Fa-f]{2}:){5}[0-9A-Fa-f]{2}$`)

// GuestInfo is the normalized set of Proxmox-level facts fetched from the
// API for a single VM/CT, decoupled from the underlying client library's
// types so callers (and tests) don't need to depend on it directly.
type GuestInfo struct {
	Name         string
	Tags         []string
	OSType       string
	Cores        int
	Memory       int // MB
	Unprivileged bool
	Status       string
	Description  string
	Pool         string
	Digest       string
	Lock         string
	HAState      string
	Template     bool
	OnBoot       bool
	Protection   bool
	MACAddress   string
	Bridge       string
	IPAddresses  []string

	// Node describes the Proxmox host this guest runs on. It comes free
	// with the API call used to fetch the guest itself, so it's always
	// populated alongside GuestInfo rather than needing a separate fetch.
	Node NodeInfo
}

// NodeInfo describes the Proxmox VE host a guest runs on.
type NodeInfo struct {
	PVEVersion       string
	KernelVersion    string
	CPUModel         string
	CPUCores         int
	CPUSockets       int
	MemoryTotalBytes uint64
	Uptime           uint64

	// ID is the node's corosync/cluster membership ID (0 for a standalone
	// node's only member). It's only unique within a single cluster - two
	// independent standalone nodes both report 0 - so it's not a good
	// candidate for a global node identity, just cluster-relative ordering.
	// nil when it couldn't be resolved (e.g. no cluster access), which is
	// distinct from a real ID of 0.
	ID *int

	// SSLFingerprint is the SHA256 fingerprint of the node's pveproxy TLS
	// certificate. It's generated once at node setup and stays stable
	// across reboots, making it the closest thing Proxmox exposes to a
	// stable, globally-unique node identifier - unlike ID above.
	SSLFingerprint string
}

// GuestInspector fetches Proxmox-level metadata for a guest given its node
// and VMID, mirroring the shape of the docker collector's ContainerInspector.
type GuestInspector interface {
	InspectContainer(ctx context.Context, node string, vmid int) (*GuestInfo, error)
	InspectVM(ctx context.Context, node string, vmid int) (*GuestInfo, error)
}

// guestCacheKey identifies a single guest's cached GuestInfo. A comparable
// struct instead of a formatted string, so there's no risk of a delimiter
// collision between an unusual node name and a different (type, vmid) pair.
type guestCacheKey struct {
	resourceType string
	node         string
	vmid         int
}

type apiGuestInspector struct {
	client *goproxmox.Client

	guestCache   *ttlCache[guestCacheKey, *GuestInfo]
	cluster      *ttlValue[*goproxmox.Cluster]
	nodeStatuses *ttlValue[goproxmox.NodeStatuses]
	resources    *ttlCache[string, goproxmox.ClusterResources] // keyed by resourceType ("lxc"/"qemu")
}

// NewAPIGuestInspector builds a GuestInspector backed by the Proxmox VE
// REST API. baseURL defaults to "https://localhost:8006/api2/json" (the
// node's own API, reachable without going over the network) when empty.
// Proxmox's default install uses a self-signed certificate, so TLS
// verification is skipped unless httpClient is overridden via opts.
func NewAPIGuestInspector(baseURL, tokenID, tokenSecret string, opts ...goproxmox.Option) GuestInspector {
	if baseURL == "" {
		baseURL = defaultBaseURL
	}

	allOpts := []goproxmox.Option{
		goproxmox.WithAPIToken(tokenID, tokenSecret),
		goproxmox.WithHTTPClient(&http.Client{
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec // PVE's default cert is self-signed; see doc comment.
			},
		}),
	}
	allOpts = append(allOpts, opts...)

	return &apiGuestInspector{
		client:       goproxmox.NewClient(baseURL, allOpts...),
		guestCache:   newTTLCache[guestCacheKey, *GuestInfo](),
		cluster:      &ttlValue[*goproxmox.Cluster]{},
		nodeStatuses: &ttlValue[goproxmox.NodeStatuses]{},
		resources:    newTTLCache[string, goproxmox.ClusterResources](),
	}
}

func (a *apiGuestInspector) InspectContainer(ctx context.Context, node string, vmid int) (*GuestInfo, error) {
	key := guestCacheKey{resourceType: "lxc", node: node, vmid: vmid}
	return a.guestCache.getOrFetch(key, guestInfoCacheTTL, func() (*GuestInfo, error) {
		return a.fetchContainer(ctx, node, vmid)
	})
}

func (a *apiGuestInspector) InspectVM(ctx context.Context, node string, vmid int) (*GuestInfo, error) {
	key := guestCacheKey{resourceType: "qemu", node: node, vmid: vmid}
	return a.guestCache.getOrFetch(key, guestInfoCacheTTL, func() (*GuestInfo, error) {
		return a.fetchVM(ctx, node, vmid)
	})
}

func (a *apiGuestInspector) fetchContainer(ctx context.Context, node string, vmid int) (*GuestInfo, error) {
	n, err := a.client.Node(ctx, node)
	if err != nil {
		return nil, err
	}

	container, err := n.Container(ctx, vmid)
	if err != nil {
		return nil, err
	}

	info := &GuestInfo{
		Name:   container.Name,
		Tags:   splitTags(container.Tags),
		Status: container.Status,
		Node:   nodeInfoFrom(n),
	}

	if cfg := container.ContainerConfig; cfg != nil {
		info.OSType = cfg.OSType
		info.Cores = cfg.Cores
		info.Unprivileged = bool(cfg.Unprivileged)
		info.Description = cfg.Description
		info.Digest = cfg.Digest
		info.Lock = cfg.Lock
		info.Template = bool(cfg.Template)
		info.OnBoot = bool(cfg.OnBoot)
		info.Protection = bool(cfg.Protection)

		if cfg.Memory != nil {
			info.Memory = *cfg.Memory
		}

		info.MACAddress, info.Bridge = parseNetDevice(cfg.Nets)
	}

	a.enrich(ctx, "lxc", node, vmid, info, func() ([]string, error) {
		ifaces, err := container.Interfaces(ctx)
		if err != nil {
			return nil, err
		}
		return ipsFromContainerInterfaces(ifaces), nil
	})

	return info, nil
}

func (a *apiGuestInspector) fetchVM(ctx context.Context, node string, vmid int) (*GuestInfo, error) {
	n, err := a.client.Node(ctx, node)
	if err != nil {
		return nil, err
	}

	vm, err := n.VirtualMachine(ctx, vmid)
	if err != nil {
		return nil, err
	}

	info := &GuestInfo{
		Name:   vm.Name,
		Status: vm.Status,
		Lock:   vm.Lock,
		Node:   nodeInfoFrom(n),
	}

	if cfg := vm.VirtualMachineConfig; cfg != nil {
		info.Tags = splitTags(cfg.Tags)
		info.Description = cfg.Description
		info.Digest = cfg.Digest
		info.Memory = int(cfg.Memory)
		info.Template = bool(cfg.Template)
		info.OnBoot = bool(cfg.OnBoot)
		info.Protection = bool(cfg.Protection)
		info.MACAddress, info.Bridge = parseNetDevice(cfg.Nets)

		if cfg.OSType != nil {
			info.OSType = *cfg.OSType
		}
		if cfg.Cores != nil {
			info.Cores = *cfg.Cores
		}
	}

	a.enrich(ctx, "qemu", node, vmid, info, func() ([]string, error) {
		// Requires the QEMU guest agent to be installed and running inside
		// the VM; commonly unavailable, so a failure here is expected.
		ifaces, err := vm.AgentGetNetworkIFaces(ctx)
		if err != nil {
			return nil, err
		}
		return ipsFromAgentInterfaces(ifaces), nil
	})

	return info, nil
}

// enrich fills in the remaining best-effort fields (live IPs, pool/HA
// state, node cluster ID, node SSL fingerprint) concurrently, since none of
// these three lookups depend on each other or on anything but ctx/node/vmid.
func (a *apiGuestInspector) enrich(ctx context.Context, resourceType, node string, vmid int, info *GuestInfo, fetchIPs func() ([]string, error)) {
	var wg sync.WaitGroup
	var pool, haState, sslFingerprint string
	var nodeID *int
	var ips []string

	wg.Add(3)

	go func() {
		defer wg.Done()
		if got, err := fetchIPs(); err == nil {
			ips = got
		}
	}()

	go func() {
		defer wg.Done()
		pool, haState, nodeID = a.resolveClusterInfo(ctx, resourceType, vmid, node)
	}()

	go func() {
		defer wg.Done()
		sslFingerprint = a.nodeSSLFingerprint(ctx, node)
	}()

	wg.Wait()

	info.IPAddresses = ips
	info.Pool = pool
	info.HAState = haState
	info.Node.ID = nodeID
	info.Node.SSLFingerprint = sslFingerprint
}

func nodeInfoFrom(n *goproxmox.Node) NodeInfo {
	return NodeInfo{
		PVEVersion:       n.PVEVersion,
		KernelVersion:    n.Kversion,
		CPUModel:         n.CPUInfo.Model,
		CPUCores:         n.CPUInfo.Cores,
		CPUSockets:       n.CPUInfo.Sockets,
		MemoryTotalBytes: n.Memory.Total,
		Uptime:           n.Uptime,
	}
}

// getCluster returns the cluster status (client.Cluster's /cluster/status),
// cached across all guests/node lookups since it's the same call regardless
// of which guest or resource type is being inspected.
func (a *apiGuestInspector) getCluster(ctx context.Context) (*goproxmox.Cluster, error) {
	return a.cluster.getOrFetch(guestInfoCacheTTL, func() (*goproxmox.Cluster, error) {
		return a.client.Cluster(ctx)
	})
}

// getResources returns /cluster/resources for a resource type, cached
// across every guest of that type on the node instead of being refetched
// once per distinct guest inspected.
func (a *apiGuestInspector) getResources(ctx context.Context, cluster *goproxmox.Cluster, resourceType string) (goproxmox.ClusterResources, error) {
	return a.resources.getOrFetch(resourceType, guestInfoCacheTTL, func() (goproxmox.ClusterResources, error) {
		return cluster.Resources(ctx, resourceType)
	})
}

// getNodeStatuses returns the cluster-wide /nodes list, cached the same way
// as getCluster/getResources above.
func (a *apiGuestInspector) getNodeStatuses(ctx context.Context) (goproxmox.NodeStatuses, error) {
	return a.nodeStatuses.getOrFetch(guestInfoCacheTTL, func() (goproxmox.NodeStatuses, error) {
		return a.client.Nodes(ctx)
	})
}

// resolveClusterInfo looks up a guest's resource pool and HA state from
// /cluster/resources, plus the node's corosync membership ID from
// /cluster/status. All three are optional/best-effort: any error or
// no-match is treated as "not set" rather than propagated, since a guest's
// core identity (fetched by fetchContainer/fetchVM) doesn't depend on any
// of them being present.
func (a *apiGuestInspector) resolveClusterInfo(ctx context.Context, resourceType string, vmid int, nodeName string) (pool string, haState string, nodeID *int) {
	cluster, err := a.getCluster(ctx)
	if err != nil {
		return "", "", nil
	}

	for _, n := range cluster.Nodes {
		if n.Name == nodeName {
			id := n.NodeID
			nodeID = &id
			break
		}
	}

	resources, err := a.getResources(ctx, cluster, resourceType)
	if err != nil {
		return "", "", nodeID
	}

	for _, r := range resources {
		if r.Type == resourceType && int(r.VMID) == vmid {
			return r.Pool, r.HAstate, nodeID
		}
	}

	return "", "", nodeID
}

// nodeSSLFingerprint looks up a node's pveproxy certificate fingerprint via
// the cluster-wide /nodes list (the only endpoint that exposes it).
func (a *apiGuestInspector) nodeSSLFingerprint(ctx context.Context, nodeName string) string {
	statuses, err := a.getNodeStatuses(ctx)
	if err != nil {
		return ""
	}

	for _, s := range statuses {
		if s.Node == nodeName {
			return s.SSLFingerprint
		}
	}

	return ""
}

// parseNetDevice extracts a MAC address and bridge name out of the first
// configured net device (net0, net1, ...). LXC devices spell the MAC as an
// explicit "hwaddr=" key; QEMU devices spell it as "<model>=<mac>" with no
// key of its own, so any comma-separated value that looks like a MAC is
// accepted as a fallback.
func parseNetDevice(nets map[string]string) (mac string, bridge string) {
	raw, ok := firstNetDevice(nets)
	if !ok {
		return "", ""
	}

	for part := range strings.SplitSeq(raw, ",") {
		key, value, hasKey := strings.Cut(part, "=")
		if !hasKey {
			continue
		}

		switch key {
		case "hwaddr":
			mac = value
		case "bridge":
			bridge = value
		default:
			if mac == "" && macAddressRegex.MatchString(value) {
				mac = value
			}
		}
	}

	return mac, bridge
}

func firstNetDevice(nets map[string]string) (string, bool) {
	if raw, ok := nets["net0"]; ok {
		return raw, true
	}

	// Fall back to the lowest-numbered device present, since net0 could in
	// principle be absent while a later index isn't. Compared numerically,
	// not lexicographically - Proxmox allows net0-net31, and string
	// comparison would put "net10" before "net2".
	lowestIndex := -1
	var lowestKey string
	for key := range nets {
		index, err := strconv.Atoi(strings.TrimPrefix(key, "net"))
		if err != nil {
			continue
		}

		if lowestIndex == -1 || index < lowestIndex {
			lowestIndex = index
			lowestKey = key
		}
	}

	if lowestKey == "" {
		return "", false
	}

	return nets[lowestKey], true
}

func ipsFromContainerInterfaces(ifaces goproxmox.ContainerInterfaces) []string {
	var ips []string
	for _, iface := range ifaces {
		if iface.Name == "lo" {
			continue
		}
		if ip, _, ok := strings.Cut(iface.Inet, "/"); ok && ip != "" {
			ips = append(ips, ip)
		}
	}

	return ips
}

func ipsFromAgentInterfaces(ifaces []*goproxmox.AgentNetworkIface) []string {
	var ips []string
	for _, iface := range ifaces {
		for _, addr := range iface.IPAddresses {
			if addr.IPAddressType == "ipv4" {
				ips = append(ips, addr.IPAddress)
			}
		}
	}

	return ips
}

func splitTags(tags string) []string {
	if tags == "" {
		return nil
	}

	var out []string
	for tag := range strings.SplitSeq(tags, ";") {
		if tag = strings.TrimSpace(tag); tag != "" {
			out = append(out, tag)
		}
	}

	return out
}
