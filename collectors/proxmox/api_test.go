package proxmox

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeData wraps v in the {"data": ...} envelope every Proxmox API
// response uses, matching what the go-proxmox client unmarshals.
func writeData(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"data": v})
}

// fakeServerCounters tracks how many times each cluster-wide endpoint was
// hit, so tests can assert on cache-sharing across distinct guests.
type fakeServerCounters struct {
	clusterStatus    int32
	clusterResources int32
	nodes            int32
}

// newFakeProxmoxServer serves the minimal set of endpoints InspectVM needs
// for vmid 200, counting requests to the VM status endpoint and every
// cluster-wide endpoint so tests can assert on cache behavior.
func newFakeProxmoxServer(t *testing.T, vmStatus, vmConfig map[string]any) (*httptest.Server, *int32) {
	t.Helper()

	server, vmFetches, _ := newFakeProxmoxServerMulti(t, map[int]fakeVM{200: {status: vmStatus, config: vmConfig}})

	return server, vmFetches
}

type fakeVM struct {
	status map[string]any
	config map[string]any
}

// newFakeProxmoxServerMulti is newFakeProxmoxServer generalized to serve
// several VMIDs behind one node, for tests that need to inspect more than
// one guest.
func newFakeProxmoxServerMulti(t *testing.T, vms map[int]fakeVM) (*httptest.Server, *int32, *fakeServerCounters) {
	t.Helper()

	var vmFetches int32
	counters := &fakeServerCounters{}

	mux := http.NewServeMux()
	mux.HandleFunc("/api2/json/nodes/proxmox/status", func(w http.ResponseWriter, r *http.Request) {
		writeData(w, map[string]any{
			"pveversion": "pve-manager/8.4.19/a68fb383814bb1e6",
			"kversion":   "Linux 6.8.12-30-pve",
			"cpuinfo":    map[string]any{"model": "test-cpu", "cores": 2, "sockets": 1},
			"memory":     map[string]any{"total": 1073741824},
			"uptime":     12345,
		})
	})
	for vmid, vm := range vms {
		path := fmt.Sprintf("/api2/json/nodes/proxmox/qemu/%d/status/current", vmid)
		mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
			atomic.AddInt32(&vmFetches, 1)
			writeData(w, vm.status)
		})
		mux.HandleFunc(fmt.Sprintf("/api2/json/nodes/proxmox/qemu/%d/config", vmid), func(w http.ResponseWriter, r *http.Request) {
			writeData(w, vm.config)
		})
		mux.HandleFunc(fmt.Sprintf("/api2/json/nodes/proxmox/qemu/%d/agent/network-get-interfaces", vmid), func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "no agent", http.StatusInternalServerError)
		})
	}
	mux.HandleFunc("/api2/json/cluster/status", func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&counters.clusterStatus, 1)
		writeData(w, []any{})
	})
	mux.HandleFunc("/api2/json/cluster/resources", func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&counters.clusterResources, 1)
		writeData(w, []any{})
	})
	mux.HandleFunc("/api2/json/nodes", func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&counters.nodes, 1)
		writeData(w, []any{})
	})

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	return server, &vmFetches, counters
}

func TestInspectVMPopulatesMemoryAndTemplate(t *testing.T) {
	server, _ := newFakeProxmoxServer(t,
		map[string]any{"name": "myvm", "status": "running"},
		map[string]any{"memory": 4096, "template": 1, "cores": 2, "ostype": "l26"},
	)

	inspector := NewAPIGuestInspector(server.URL+"/api2/json", "user@pam!test", "secret")

	info, err := inspector.InspectVM(context.Background(), "proxmox", 200)
	require.NoError(t, err)

	assert.Equal(t, 4096, info.Memory)
	assert.True(t, info.Template)
	assert.Equal(t, 2, info.Cores)
}

func TestInspectVMCachesResultWithinTTL(t *testing.T) {
	server, vmFetches := newFakeProxmoxServer(t,
		map[string]any{"name": "myvm", "status": "running"},
		map[string]any{"memory": 2048, "cores": 1},
	)

	inspector := NewAPIGuestInspector(server.URL+"/api2/json", "user@pam!test", "secret")

	_, err := inspector.InspectVM(context.Background(), "proxmox", 200)
	require.NoError(t, err)
	_, err = inspector.InspectVM(context.Background(), "proxmox", 200)
	require.NoError(t, err)

	assert.EqualValues(t, 1, atomic.LoadInt32(vmFetches), "second call within TTL should be served from cache, not refetched")
}

func TestFirstNetDevicePicksNumericallyLowestIndex(t *testing.T) {
	raw, ok := firstNetDevice(map[string]string{
		"net10": "bridge=vmbr10,hwaddr=AA:AA:AA:AA:AA:AA",
		"net2":  "bridge=vmbr2,hwaddr=BB:BB:BB:BB:BB:BB",
	})
	require.True(t, ok)
	assert.Equal(t, "bridge=vmbr2,hwaddr=BB:BB:BB:BB:BB:BB", raw)
}

func TestFirstNetDevicePrefersNet0(t *testing.T) {
	raw, ok := firstNetDevice(map[string]string{
		"net1": "bridge=vmbr1",
		"net0": "bridge=vmbr0",
	})
	require.True(t, ok)
	assert.Equal(t, "bridge=vmbr0", raw)
}

func TestFirstNetDeviceEmpty(t *testing.T) {
	_, ok := firstNetDevice(nil)
	assert.False(t, ok)
}

func TestClusterWideDataSharedAcrossDistinctGuests(t *testing.T) {
	server, _, counters := newFakeProxmoxServerMulti(t, map[int]fakeVM{
		200: {status: map[string]any{"name": "vm200", "status": "running"}, config: map[string]any{}},
		201: {status: map[string]any{"name": "vm201", "status": "running"}, config: map[string]any{}},
	})

	inspector := NewAPIGuestInspector(server.URL+"/api2/json", "user@pam!test", "secret")

	_, err := inspector.InspectVM(context.Background(), "proxmox", 200)
	require.NoError(t, err)
	_, err = inspector.InspectVM(context.Background(), "proxmox", 201)
	require.NoError(t, err)

	assert.EqualValues(t, 1, atomic.LoadInt32(&counters.clusterStatus), "cluster/status should be fetched once and reused across distinct guests")
	assert.EqualValues(t, 1, atomic.LoadInt32(&counters.clusterResources), "cluster/resources should be fetched once and reused across distinct guests")
	assert.EqualValues(t, 1, atomic.LoadInt32(&counters.nodes), "nodes should be fetched once and reused across distinct guests")
}
