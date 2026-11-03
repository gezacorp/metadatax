package procfs_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gezacorp/metadatax"
	"github.com/gezacorp/metadatax/collectors/procfs"
)

func TestInnermostUnit(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		path string
		want procfs.CgroupUnit
	}{
		{
			name: "system service",
			path: "/system.slice/nginx.service",
			want: procfs.CgroupUnit{
				Unit:    "nginx.service",
				Manager: "system",
				Kind:    procfs.KindService,
				Name:    "nginx.service",
			},
		},
		{
			name: "template instance folds",
			path: "/system.slice/system-getty.slice/getty@tty1.service",
			want: procfs.CgroupUnit{
				Unit:    "getty@tty1.service",
				Manager: "system",
				Kind:    procfs.KindService,
				Name:    "getty@.service",
			},
		},
		{
			name: "snap app under the user manager",
			path: "/user.slice/user-501.slice/user@501.service/app.slice/snap.google-cloud-cli.kubectl-0f9dcfea-afc5-41a7-9b32-937297529053.scope",
			want: procfs.CgroupUnit{
				Unit:    "snap.google-cloud-cli.kubectl-0f9dcfea-afc5-41a7-9b32-937297529053.scope",
				Manager: "user",
				Kind:    procfs.KindSnap,
				Name:    "google-cloud-cli.kubectl",
				Snap:    "google-cloud-cli",
				SnapApp: "kubectl",
			},
		},
		{
			name: "snap service",
			path: "/system.slice/snap.lxd.daemon.service",
			want: procfs.CgroupUnit{
				Unit:    "snap.lxd.daemon.service",
				Manager: "system",
				Kind:    procfs.KindSnap,
				Name:    "lxd.daemon",
				Snap:    "lxd",
				SnapApp: "daemon",
			},
		},
		{
			name: "desktop app scope with a launcher",
			path: "/user.slice/user-1000.slice/user@1000.service/app.slice/app-gnome-org.mozilla.firefox-4242.scope",
			want: procfs.CgroupUnit{
				Unit:     "app-gnome-org.mozilla.firefox-4242.scope",
				Manager:  "user",
				Kind:     procfs.KindApp,
				Name:     "org.mozilla.firefox",
				Launcher: "gnome",
			},
		},
		{
			name: "desktop app service with escaped dashes",
			path: "/user.slice/user-1000.slice/user@1000.service/app.slice/app-kde-dev.zed.Zed\\x2dPreview@a1b2.service",
			want: procfs.CgroupUnit{
				Unit:     "app-kde-dev.zed.Zed\\x2dPreview@a1b2.service",
				Manager:  "user",
				Kind:     procfs.KindApp,
				Name:     "dev.zed.Zed-Preview",
				Launcher: "kde",
			},
		},
		{
			name: "desktop app ID with escaped UTF-8",
			path: "/user.slice/user-1000.slice/user@1000.service/app.slice/app-gnome-org.example.Caf\\xc3\\xa9-77.scope",
			want: procfs.CgroupUnit{
				Unit:     "app-gnome-org.example.Caf\\xc3\\xa9-77.scope",
				Manager:  "user",
				Kind:     procfs.KindApp,
				Name:     "org.example.Café",
				Launcher: "gnome",
			},
		},
		{
			name: "tmux pane",
			path: "/user.slice/user-501.slice/user@501.service/tmux-spawn-12b36f24-2f17-478a-88ad-57317c27410c.scope",
			want: procfs.CgroupUnit{
				Unit:    "tmux-spawn-12b36f24-2f17-478a-88ad-57317c27410c.scope",
				Manager: "user",
				Kind:    procfs.KindTmux,
			},
		},
		{
			name: "terminal tab",
			path: "/user.slice/user-1000.slice/user@1000.service/app.slice/app-org.gnome.Terminal.slice/vte-spawn-2f1b0e4a-9c3d-4f5e-8a7b-6c5d4e3f2a1b.scope",
			want: procfs.CgroupUnit{
				Unit:    "vte-spawn-2f1b0e4a-9c3d-4f5e-8a7b-6c5d4e3f2a1b.scope",
				Manager: "user",
				Kind:    procfs.KindTerminal,
			},
		},
		{
			name: "systemd-run service",
			path: "/system.slice/run-r2add671c054348119c70a0ab12489c8a.service",
			want: procfs.CgroupUnit{
				Unit:    "run-r2add671c054348119c70a0ab12489c8a.service",
				Manager: "system",
				Kind:    procfs.KindTransient,
			},
		},
		{
			name: "systemd-run user scope",
			path: "/user.slice/user-501.slice/user@501.service/app.slice/run-r01737f14a959466aa36ef952fec9d709.scope",
			want: procfs.CgroupUnit{
				Unit:    "run-r01737f14a959466aa36ef952fec9d709.scope",
				Manager: "user",
				Kind:    procfs.KindTransient,
			},
		},
		{
			name: "systemd-run service of older and newer systemd",
			path: "/system.slice/run-u42.service",
			want: procfs.CgroupUnit{
				Unit:    "run-u42.service",
				Manager: "system",
				Kind:    procfs.KindTransient,
			},
		},
		{
			name: "systemd-run service named by pid and invocation",
			path: "/system.slice/run-p4242-i4243.service",
			want: procfs.CgroupUnit{
				Unit:    "run-p4242-i4243.service",
				Manager: "system",
				Kind:    procfs.KindTransient,
			},
		},
		{
			name: "D-Bus activated user service",
			path: "/user.slice/user-1000.slice/user@1000.service/app.slice/dbus-:1.2-org.freedesktop.portal.Desktop@0.service",
			want: procfs.CgroupUnit{
				Unit:    "dbus-:1.2-org.freedesktop.portal.Desktop@0.service",
				Manager: "user",
				Kind:    procfs.KindService,
				Name:    "org.freedesktop.portal.Desktop",
			},
		},
		{
			name: "login session",
			path: "/user.slice/user-501.slice/session-5.scope",
			want: procfs.CgroupUnit{
				Unit:    "session-5.scope",
				Manager: "system",
				Kind:    procfs.KindSession,
			},
		},
		{
			name: "docker container",
			path: "/system.slice/docker-f96b2842b994fe5bc5ab557c5b57467beb095f69143c31c54c075607e6ee2a7a.scope",
			want: procfs.CgroupUnit{
				Unit:             "docker-f96b2842b994fe5bc5ab557c5b57467beb095f69143c31c54c075607e6ee2a7a.scope",
				Manager:          "system",
				Kind:             procfs.KindContainer,
				ContainerUnit:    "docker-f96b2842b994fe5bc5ab557c5b57467beb095f69143c31c54c075607e6ee2a7a.scope",
				ContainerRuntime: "docker",
			},
		},
		{
			name: "kubernetes container",
			path: "/kubepods.slice/kubepods-burstable.slice/kubepods-burstable-pod1234.slice/cri-containerd-0a1b2c3d.scope",
			want: procfs.CgroupUnit{
				Unit:             "cri-containerd-0a1b2c3d.scope",
				Manager:          "system",
				Kind:             procfs.KindContainer,
				ContainerUnit:    "cri-containerd-0a1b2c3d.scope",
				ContainerRuntime: "cri-containerd",
			},
		},
		{
			name: "service of a systemd inside a docker container",
			path: "/system.slice/docker-0a1b2c3d.scope/system.slice/nginx.service",
			want: procfs.CgroupUnit{
				Unit:             "nginx.service",
				Manager:          "system",
				Kind:             procfs.KindService,
				Name:             "nginx.service",
				ContainerUnit:    "docker-0a1b2c3d.scope",
				ContainerRuntime: "docker",
			},
		},
		{
			name: "service in an LXD or Incus system container",
			path: "/lxc.payload.web1/system.slice/nginx.service",
			want: procfs.CgroupUnit{
				Unit:             "nginx.service",
				Manager:          "system",
				Kind:             procfs.KindService,
				Name:             "nginx.service",
				ContainerUnit:    "lxc.payload.web1",
				ContainerRuntime: "lxc",
			},
		},
		{
			name: "LXD container without a systemd",
			path: "/lxc.payload.alpine",
			want: procfs.CgroupUnit{
				Unit:             "lxc.payload.alpine",
				Manager:          "system",
				Kind:             procfs.KindContainer,
				ContainerUnit:    "lxc.payload.alpine",
				ContainerRuntime: "lxc",
			},
		},
		{
			name: "service in a systemd-nspawn container",
			path: "/machine.slice/systemd-nspawn@build.service/payload/system.slice/sshd.service",
			want: procfs.CgroupUnit{
				Unit:             "sshd.service",
				Manager:          "system",
				Kind:             procfs.KindService,
				Name:             "sshd.service",
				ContainerUnit:    "systemd-nspawn@build.service",
				ContainerRuntime: "nspawn",
			},
		},
		{
			name: "user manager itself",
			path: "/user.slice/user-501.slice/user@501.service/init.scope",
			want: procfs.CgroupUnit{
				Unit:    "init.scope",
				Manager: "user",
				Kind:    procfs.KindInit,
			},
		},
		{
			name: "other scope",
			path: "/machine.slice/machine-qemu\\x2d1\\x2dweb.scope",
			want: procfs.CgroupUnit{
				Unit:    "machine-qemu\\x2d1\\x2dweb.scope",
				Manager: "system",
				Kind:    procfs.KindScope,
				Name:    "machine-qemu\\x2d1\\x2dweb.scope",
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, ok := procfs.InnermostUnit(tc.path)
			require.True(t, ok)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestInnermostUnit_NoUnit(t *testing.T) {
	t.Parallel()

	for _, path := range []string{"", "/", "/kubepods/burstable/pod1/abc", "/user.slice"} {
		_, ok := procfs.InnermostUnit(path)
		assert.False(t, ok, path)
	}
}

func TestGetMetadata_CgroupUnit(t *testing.T) {
	t.Parallel()

	const path = "/user.slice/user-501.slice/user@501.service/app.slice/snap.httpie.http-b7062341-5808-46e1-8d69-f613cf5297a9.scope"

	md, err := procfs.New(
		procfs.CollectorWithProcessInfoFunc(func(context.Context, int32) (procfs.ProcessInfo, error) {
			return &processInfo{cgroup: path}, nil
		}),
		procfs.WithForceHasProcFS(),
	).GetMetadata(metadatax.ContextWithPID(context.Background(), 1234))
	require.NoError(t, err)

	labels := md.GetLabels()
	assert.Equal(t, []string{path}, labels["process:cgroup"])
	assert.Equal(t, []string{"snap.httpie.http-b7062341-5808-46e1-8d69-f613cf5297a9.scope"}, labels["process:cgroup:unit"])
	assert.Equal(t, []string{"user"}, labels["process:cgroup:manager"])
	assert.Equal(t, []string{"snap"}, labels["process:cgroup:kind"])
	assert.Equal(t, []string{"httpie.http"}, labels["process:cgroup:name"])
	assert.Equal(t, []string{"httpie"}, labels["process:cgroup:snap"])
	assert.Equal(t, []string{"http"}, labels["process:cgroup:snap-app"])
	assert.NotContains(t, labels, "process:cgroup:container:runtime")
}
