package proxmox

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gezacorp/metadatax"
)

func intPtr(v int) *int {
	return &v
}

// fakeGuestInspector is a test double for GuestInspector, mirroring the
// docker collector's fake ContainerInspector test pattern.
type fakeGuestInspector struct {
	containers map[string]*GuestInfo
	vms        map[string]*GuestInfo
}

func (f *fakeGuestInspector) InspectContainer(ctx context.Context, node string, vmid int) (*GuestInfo, error) {
	info, ok := f.containers[strconv.Itoa(vmid)]
	if !ok {
		return nil, os.ErrNotExist
	}

	return info, nil
}

func (f *fakeGuestInspector) InspectVM(ctx context.Context, node string, vmid int) (*GuestInfo, error) {
	info, ok := f.vms[strconv.Itoa(vmid)]
	if !ok {
		return nil, os.ErrNotExist
	}

	return info, nil
}

// setupHostFixture builds a fake HOST_PROC tree with a cgroup entry for pid,
// and a fake /etc/pve tree with just the "local" node symlink (node-name
// resolution stays file-based; guest and node details come from the
// GuestInspector).
func setupHostFixture(t *testing.T, pid int, cgroupContent string, node string) string {
	t.Helper()

	hostProc := t.TempDir()
	t.Setenv("HOST_PROC", hostProc)

	pidDir := filepath.Join(hostProc, strconv.Itoa(pid))
	require.NoError(t, os.MkdirAll(pidDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(pidDir, "cgroup"), []byte(cgroupContent), 0o644))
	// procfs.Proc.Cgroups() also reads the process's stat file; stub the minimum it needs.
	require.NoError(t, os.WriteFile(filepath.Join(pidDir, "stat"), []byte("1 (init) S 0 1 1 0 -1 4194560 0 0 0 0 0 0 0 0 20 0 1 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0"), 0o644))

	pveRoot := t.TempDir()
	if node != "" {
		require.NoError(t, os.MkdirAll(filepath.Join(pveRoot, "nodes", node), 0o755))
		require.NoError(t, os.Symlink(filepath.Join("nodes", node), filepath.Join(pveRoot, "local")))
	}

	return pveRoot
}

func TestCollectorLXCGuest(t *testing.T) {
	pid := 12069
	pveRoot := setupHostFixture(t, pid, "0::/lxc/100/ns/init.scope\n", "proxmox")

	inspector := &fakeGuestInspector{
		containers: map[string]*GuestInfo{
			"100": {
				Name:         "CT100",
				OSType:       "ubuntu",
				Cores:        4,
				Memory:       512,
				Unprivileged: true,
				Status:       "running",
				Tags:         []string{"web", "prod"},
				Description:  "web server",
				Pool:         "team-a",
				Digest:       "71e74eecfad417e845ba67aa8ccf00c82a33e2fe",
				Template:     false,
				OnBoot:       true,
				Protection:   false,
				MACAddress:   "BC:24:11:98:D1:B1",
				Bridge:       "vmbr0",
				IPAddresses:  []string{"10.1.200.140"},
				Node: NodeInfo{
					PVEVersion:       "pve-manager/8.4.19/a68fb383814bb1e6",
					KernelVersion:    "6.8.12-30-pve",
					CPUModel:         "Intel(R) Xeon(R) CPU E5-2630 0 @ 2.30GHz",
					CPUCores:         2,
					CPUSockets:       2,
					MemoryTotalBytes: 4054757376,
					ID:               intPtr(0),
					SSLFingerprint:   "BD:6C:A1:0F:C3:8A:DF:A5:54:A2:11:10:D3:09:01:40:58:DE:07:15:CF:CE:3C:87:CC:E2:C8:B6:8A:27:96:2D",
				},
			},
		},
	}

	collector := New(withPVERoot(pveRoot), WithGuestInspector(inspector))

	ctx := metadatax.ContextWithPID(context.Background(), int32(pid))
	md, err := collector.GetMetadata(ctx)
	require.NoError(t, err)

	labels := md.GetLabels()
	assert.Equal(t, []string{"100"}, labels["proxmox:guest:vmid"])
	assert.Equal(t, []string{"lxc"}, labels["proxmox:guest:type"])
	assert.Equal(t, []string{"proxmox"}, labels["proxmox:node:name"])
	assert.Equal(t, []string{"CT100"}, labels["proxmox:guest:name"])
	assert.Equal(t, []string{"ubuntu"}, labels["proxmox:guest:ostype"])
	assert.Equal(t, []string{"running"}, labels["proxmox:guest:status"])
	assert.Equal(t, []string{"4"}, labels["proxmox:guest:cores"])
	assert.Equal(t, []string{"512"}, labels["proxmox:guest:memory"])
	assert.Equal(t, []string{"true"}, labels["proxmox:guest:unprivileged"])
	assert.ElementsMatch(t, []string{"web", "prod"}, labels["proxmox:guest:tag"])
	assert.Equal(t, []string{"web server"}, labels["proxmox:guest:description"])
	assert.Equal(t, []string{"team-a"}, labels["proxmox:guest:pool"])
	assert.Equal(t, []string{"71e74eecfad417e845ba67aa8ccf00c82a33e2fe"}, labels["proxmox:guest:digest"])
	assert.Equal(t, []string{"false"}, labels["proxmox:guest:template"])
	assert.Equal(t, []string{"true"}, labels["proxmox:guest:onboot"])
	assert.Equal(t, []string{"false"}, labels["proxmox:guest:protection"])
	assert.Equal(t, []string{"BC:24:11:98:D1:B1"}, labels["proxmox:guest:mac"])
	assert.Equal(t, []string{"vmbr0"}, labels["proxmox:guest:bridge"])
	assert.Equal(t, []string{"10.1.200.140"}, labels["proxmox:guest:ip"])
	assert.Equal(t, []string{"pve-manager/8.4.19/a68fb383814bb1e6"}, labels["proxmox:node:pve-version"])
	assert.Equal(t, []string{"6.8.12-30-pve"}, labels["proxmox:node:kernel"])
	assert.Equal(t, []string{"0"}, labels["proxmox:node:id"])
	assert.Equal(t, []string{"BD:6C:A1:0F:C3:8A:DF:A5:54:A2:11:10:D3:09:01:40:58:DE:07:15:CF:CE:3C:87:CC:E2:C8:B6:8A:27:96:2D"}, labels["proxmox:node:ssl-fingerprint"])
	assert.Equal(t, []string{"Intel(R) Xeon(R) CPU E5-2630 0 @ 2.30GHz"}, labels["proxmox:node:cpu-model"])
	assert.Equal(t, []string{"2"}, labels["proxmox:node:cpu-cores"])
	assert.Equal(t, []string{"2"}, labels["proxmox:node:cpu-sockets"])
	assert.Equal(t, []string{"4054757376"}, labels["proxmox:node:memory-total"])
}

func TestCollectorQemuGuest(t *testing.T) {
	pid := 5555
	pveRoot := setupHostFixture(t, pid, "0::/qemu.slice/200.scope\n", "proxmox")

	inspector := &fakeGuestInspector{
		vms: map[string]*GuestInfo{
			"200": {
				Name:   "myvm",
				OSType: "l26",
				Status: "running",
			},
		},
	}

	collector := New(withPVERoot(pveRoot), WithGuestInspector(inspector))

	ctx := metadatax.ContextWithPID(context.Background(), int32(pid))
	md, err := collector.GetMetadata(ctx)
	require.NoError(t, err)

	labels := md.GetLabels()
	assert.Equal(t, []string{"200"}, labels["proxmox:guest:vmid"])
	assert.Equal(t, []string{"qemu"}, labels["proxmox:guest:type"])
	assert.Equal(t, []string{"myvm"}, labels["proxmox:guest:name"])
	_, hasUnprivileged := labels["proxmox:guest:unprivileged"]
	assert.False(t, hasUnprivileged, "unprivileged is an LXC-only concept")
}

func TestCollectorReturnsNilMetadataWhenInspectionFails(t *testing.T) {
	pid := 12069
	pveRoot := setupHostFixture(t, pid, "0::/lxc/100/ns/init.scope\n", "proxmox")

	// fakeGuestInspector has no entry for vmid 100, so InspectContainer
	// returns an error - simulating an API failure after vmid/type/node
	// were already resolved from the cheap, reliable cgroup+symlink lookup.
	collector := New(withPVERoot(pveRoot), WithGuestInspector(&fakeGuestInspector{}))

	ctx := metadatax.ContextWithPID(context.Background(), int32(pid))
	md, err := collector.GetMetadata(ctx)
	require.Error(t, err)
	assert.Nil(t, md)
}

func TestCollectorSkipOnSoftErrorSwallowsInspectionFailure(t *testing.T) {
	pid := 12069
	pveRoot := setupHostFixture(t, pid, "0::/lxc/100/ns/init.scope\n", "proxmox")

	collector := New(withPVERoot(pveRoot), WithGuestInspector(&fakeGuestInspector{}), WithSkipOnSoftError())

	ctx := metadatax.ContextWithPID(context.Background(), int32(pid))
	md, err := collector.GetMetadata(ctx)
	require.NoError(t, err)

	labels := md.GetLabels()
	assert.Equal(t, []string{"100"}, labels["proxmox:guest:vmid"])
}

func TestCollectorNotAGuestProcess(t *testing.T) {
	pid := 1
	pveRoot := setupHostFixture(t, pid, "0::/init.scope\n", "proxmox")

	collector := New(withPVERoot(pveRoot), WithGuestInspector(&fakeGuestInspector{}))

	ctx := metadatax.ContextWithPID(context.Background(), int32(pid))
	md, err := collector.GetMetadata(ctx)
	require.NoError(t, err)
	assert.Empty(t, md.GetLabels())
}

func TestCollectorNoInspectorConfigured(t *testing.T) {
	pid := 12069
	pveRoot := setupHostFixture(t, pid, "0::/lxc/100/ns/init.scope\n", "proxmox")

	collector := New(withPVERoot(pveRoot))

	ctx := metadatax.ContextWithPID(context.Background(), int32(pid))
	md, err := collector.GetMetadata(ctx)
	require.NoError(t, err)

	labels := md.GetLabels()
	assert.Equal(t, []string{"100"}, labels["proxmox:guest:vmid"])
	assert.Equal(t, []string{"lxc"}, labels["proxmox:guest:type"])
	_, hasName := labels["proxmox:guest:name"]
	assert.False(t, hasName)
}

func TestCollectorNoPID(t *testing.T) {
	collector := New()

	_, err := collector.GetMetadata(context.Background())
	assert.ErrorIs(t, err, metadatax.PIDNotFoundError)
}
