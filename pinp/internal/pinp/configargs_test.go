package pinp

import "testing"

func fullEffectiveConfig() *EffectiveConfig {
	cfg := &EffectiveConfig{}
	cfg.Containers.Containers.NetNS = "host"
	cfg.Containers.Containers.IPCNs = "host"
	cfg.Containers.Containers.UTSNs = "host"
	cfg.Containers.Containers.CgroupNS = "host"
	cfg.Containers.Containers.PIDNS = "host"
	cfg.Containers.Containers.Cgroups = "enabled"
	cfg.Containers.Containers.LogDriver = "k8s-file"
	cfg.Containers.Engine.Runtime = "crun"
	cfg.Containers.Engine.CgroupManager = "cgroupfs"
	cfg.Containers.Engine.EventsLogger = "file"
	cfg.Storage.Storage.Driver = "overlay"
	cfg.Storage.Storage.Options.Overlay.MountProgram = "/usr/bin/fuse-overlayfs"
	return cfg
}

func equalSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func containsToken(args []string, token string) bool {
	for _, a := range args {
		if a == token {
			return true
		}
	}
	return false
}

func TestHasFlag(t *testing.T) {
	cases := []struct {
		name  string
		args  []string
		names []string
		want  bool
	}{
		{"exact-match", []string{"--network", "host"}, []string{"--network"}, true},
		{"equals-form", []string{"--network=bridge"}, []string{"--network"}, true},
		{"no-prefix-match", []string{"--network-alias", "x"}, []string{"--network"}, false},
		{"alias-match", []string{"--net", "host"}, []string{"--network", "--net"}, true},
		{"before-subcommand", []string{"--network", "bridge", "run", "img"}, []string{"--network"}, true},
		{"absent", []string{"run", "img"}, []string{"--network"}, false},
		{"value-token-not-name", []string{"host"}, []string{"--network"}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := HasFlag(c.args, c.names...); got != c.want {
				t.Fatalf("HasFlag(%v, %v) = %v, want %v", c.args, c.names, got, c.want)
			}
		})
	}
}

func TestConfigArgs(t *testing.T) {
	wantRun := []string{
		"--network", "host",
		"--ipc", "host",
		"--uts", "host",
		"--cgroupns", "host",
		"--pid", "host",
		"--cgroups", "enabled",
		"--log-driver", "k8s-file",
		"--runtime", "crun",
		"--cgroup-manager", "cgroupfs",
		"--events-backend", "file",
		"--storage-driver", "overlay",
		"--storage-opt", "overlay.mount_program=/usr/bin/fuse-overlayfs",
	}
	wantBuild := []string{
		"--network", "host",
		"--ipc", "host",
		"--uts", "host",
		"--cgroupns", "host",
		"--pid", "host",
		"--runtime", "crun",
		"--cgroup-manager", "cgroupfs",
		"--events-backend", "file",
		"--storage-driver", "overlay",
		"--storage-opt", "overlay.mount_program=/usr/bin/fuse-overlayfs",
	}

	t.Run("fully-configured-empty", func(t *testing.T) {
		if got := ConfigArgs(fullEffectiveConfig(), nil, "run"); len(got) != 0 {
			t.Fatalf("ConfigArgs(full, run) = %v, want empty", got)
		}
	})

	t.Run("all-empty-run", func(t *testing.T) {
		got := ConfigArgs(&EffectiveConfig{}, nil, "run")
		if !equalSlices(got, wantRun) {
			t.Fatalf("ConfigArgs(empty, run) = %v, want %v", got, wantRun)
		}
		if len(got) != 24 {
			t.Fatalf("ConfigArgs(empty, run) has %d tokens, want 24", len(got))
		}
	})

	t.Run("all-empty-build", func(t *testing.T) {
		got := ConfigArgs(&EffectiveConfig{}, nil, "build")
		if !equalSlices(got, wantBuild) {
			t.Fatalf("ConfigArgs(empty, build) = %v, want %v", got, wantBuild)
		}
		if len(got) != 20 {
			t.Fatalf("ConfigArgs(empty, build) has %d tokens, want 20", len(got))
		}
		if containsToken(got, "--cgroups") || containsToken(got, "--log-driver") {
			t.Fatalf("ConfigArgs(empty, build) must not contain --cgroups or --log-driver: %v", got)
		}
	})

	t.Run("only-netns-set", func(t *testing.T) {
		cfg := &EffectiveConfig{}
		cfg.Containers.Containers.NetNS = "host"
		got := ConfigArgs(cfg, nil, "run")
		if !equalSlices(got, wantRun[2:]) {
			t.Fatalf("ConfigArgs(netns, run) = %v, want %v", got, wantRun[2:])
		}
		if len(got) != 22 {
			t.Fatalf("ConfigArgs(netns, run) has %d tokens (11 settings), want 22", len(got))
		}
		if containsToken(got, "--network") {
			t.Fatalf("ConfigArgs(netns, run) must not inject --network: %v", got)
		}
	})

	t.Run("caller-network-suppresses", func(t *testing.T) {
		got := ConfigArgs(&EffectiveConfig{}, []string{"run", "--network", "bridge", "img"}, "run")
		if containsToken(got, "--network") {
			t.Fatalf("caller --network bridge must suppress --network: %v", got)
		}
		if !equalSlices(got, wantRun[2:]) {
			t.Fatalf("ConfigArgs with --network = %v, want %v", got, wantRun[2:])
		}
	})

	t.Run("caller-net-alias-suppresses", func(t *testing.T) {
		got := ConfigArgs(&EffectiveConfig{}, []string{"run", "--net", "bridge", "img"}, "run")
		if containsToken(got, "--network") {
			t.Fatalf("caller --net must suppress --network: %v", got)
		}
	})

	t.Run("caller-runtime-equals-suppresses", func(t *testing.T) {
		got := ConfigArgs(&EffectiveConfig{}, []string{"run", "--runtime=runc", "img"}, "run")
		if containsToken(got, "--runtime") {
			t.Fatalf("caller --runtime=runc must suppress --runtime: %v", got)
		}
		if len(got) != 22 {
			t.Fatalf("ConfigArgs with --runtime=runc has %d tokens, want 22", len(got))
		}
	})
}
