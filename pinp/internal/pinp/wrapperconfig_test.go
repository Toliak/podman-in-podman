package pinp

import (
	"os"
	"path/filepath"
	"testing"
)

var wrapperEnvKeys = []string{
	"PINP_CONFIG", "PINP_PODMAN", "PINP_ECHO", "PINP_EXPLAIN",
	"PINP_PASSTHROUGH", "PINP_SKIP_CHECKS", "PINP_UID", "PINP_GID",
	"PINP_USER", "PINP_SUBUID_FILE", "PINP_SUBGID_FILE", "PINP_STORE",
	"PINP_STORE_OPTS", "PINP_FUSE_DEVICE", "PINP_SHARED_STORE",
	"PINP_NEWUIDMAP", "PINP_NEWGIDMAP", "PINP_INNER_UIDMAPS",
	"PINP_UID_MAP_FILE", "PINP_GID_MAP_FILE", "PINP_SETCAP",
	"PINP_USR_SHARE_CONTAINERS", "PINP_ETC_CONTAINERS", "PINP_HOME",
	"PINP_SELINUX", "PINP_SELINUXFS", "XDG_CONFIG_HOME", "HOME",
}

func isolateWrapperEnv(t *testing.T) {
	t.Helper()
	type saved struct {
		val string
		ok  bool
	}
	orig := make(map[string]saved, len(wrapperEnvKeys))
	for _, k := range wrapperEnvKeys {
		v, ok := os.LookupEnv(k)
		orig[k] = saved{v, ok}
		os.Unsetenv(k)
	}
	t.Cleanup(func() {
		for k, s := range orig {
			if s.ok {
				_ = os.Setenv(k, s.val)
			} else {
				_ = os.Unsetenv(k)
			}
		}
	})
}

func baseWrapperEnv(t *testing.T, dir string) {
	t.Helper()
	t.Setenv("PINP_HOME", dir)
	t.Setenv("HOME", dir)
	t.Setenv("PINP_USER", "tester")
	t.Setenv("PINP_UID", "1002")
	t.Setenv("PINP_GID", "1006")
}

func TestLoadEnvDefaults(t *testing.T) {
	isolateWrapperEnv(t)
	dir := t.TempDir()
	baseWrapperEnv(t, dir)
	env, err := LoadEnv()
	if err != nil {
		t.Fatalf("LoadEnv with no config: %v", err)
	}
	if env.Echo != true {
		t.Errorf("Echo default = %v, want true", env.Echo)
	}
	if env.SELinux != "auto" {
		t.Errorf("SELinux default = %q, want auto", env.SELinux)
	}
	if env.SELinuxFS != "/sys/fs/selinux" {
		t.Errorf("SELinuxFS default = %q", env.SELinuxFS)
	}
	if env.Podman != "podman" {
		t.Errorf("Podman default = %q", env.Podman)
	}
	if env.UID != 1002 || env.GID != 1006 {
		t.Errorf("uid/gid = %d/%d, want 1002/1006", env.UID, env.GID)
	}
	if env.UIDMapFile != DefaultUIDMapFile || env.GIDMapFile != DefaultGIDMapFile {
		t.Errorf("id-map file defaults = %q/%q, want %q/%q", env.UIDMapFile, env.GIDMapFile, DefaultUIDMapFile, DefaultGIDMapFile)
	}
	if env.Setcap != DefaultSetcap {
		t.Errorf("Setcap default = %q, want %q", env.Setcap, DefaultSetcap)
	}
}

func TestLoadEnvConfigAndEnvPrecedence(t *testing.T) {
	isolateWrapperEnv(t)
	dir := t.TempDir()
	t.Setenv("PINP_HOME", dir)
	t.Setenv("HOME", dir)
	cfgPath := filepath.Join(dir, "pinp.toml")
	content := `
[wrapper]
podman = "cfg-podman"
echo = false
explain = true
passthrough = true
skip_checks = true
[host]
uid = 1111
gid = 2222
user = "cfg-user"
store = "/cfg/store"
store_opts = ":z"
uid_map_file = "/cfg/uid_map"
gid_map_file = "/cfg/gid_map"
setcap = "/cfg/setcap"
selinux = "on"
selinuxfs = "/cfg/selinuxfs"
[l1]
inner_uidmaps = "0:0:10 65534:10:2"
`
	if err := os.WriteFile(cfgPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PINP_CONFIG", cfgPath)

	env, err := LoadEnv()
	if err != nil {
		t.Fatal(err)
	}
	checks := []struct {
		name string
		got  any
		want any
	}{
		{"Podman", env.Podman, "cfg-podman"},
		{"Echo", env.Echo, false},
		{"Explain", env.Explain, true},
		{"Passthrough", env.Passthrough, true},
		{"SkipChecks", env.SkipChecks, true},
		{"UID", env.UID, 1111},
		{"GID", env.GID, 2222},
		{"User", env.User, "cfg-user"},
		{"Store", env.Store, "/cfg/store"},
		{"StoreOpts", env.StoreOpts, ":z"},
		{"UIDMapFile", env.UIDMapFile, "/cfg/uid_map"},
		{"GIDMapFile", env.GIDMapFile, "/cfg/gid_map"},
		{"Setcap", env.Setcap, "/cfg/setcap"},
		{"SELinux", env.SELinux, "on"},
		{"SELinuxFS", env.SELinuxFS, "/cfg/selinuxfs"},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("config: %s = %v, want %v", c.name, c.got, c.want)
		}
	}
	if len(env.InnerMaps) != 2 || env.InnerMaps[0] != "0:0:10" || env.InnerMaps[1] != "65534:10:2" {
		t.Errorf("config: InnerMaps = %v", env.InnerMaps)
	}

	t.Setenv("PINP_PODMAN", "env-podman")
	t.Setenv("PINP_ECHO", "1")
	t.Setenv("PINP_UID", "9999")
	t.Setenv("PINP_SELINUX", "off")
	env2, err := LoadEnv()
	if err != nil {
		t.Fatal(err)
	}
	if env2.Podman != "env-podman" {
		t.Errorf("env: Podman = %q, want env-podman", env2.Podman)
	}
	if env2.Echo != true {
		t.Errorf("env: Echo = %v, want true", env2.Echo)
	}
	if env2.UID != 9999 {
		t.Errorf("env: UID = %d, want 9999", env2.UID)
	}
	if env2.SELinux != "off" {
		t.Errorf("env: SELinux = %q, want off", env2.SELinux)
	}
}

func TestLoadEnvMissingConfigSilent(t *testing.T) {
	isolateWrapperEnv(t)
	dir := t.TempDir()
	baseWrapperEnv(t, dir)
	t.Setenv("PINP_CONFIG", filepath.Join(dir, "absent.toml"))
	if _, err := LoadEnv(); err != nil {
		t.Fatalf("missing config must be a silent no-op, got: %v", err)
	}
}

func TestLoadEnvMalformedConfig(t *testing.T) {
	isolateWrapperEnv(t)
	dir := t.TempDir()
	baseWrapperEnv(t, dir)
	cfgPath := filepath.Join(dir, "bad.toml")
	if err := os.WriteFile(cfgPath, []byte("[wrapper\npodman = ???\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PINP_CONFIG", cfgPath)
	if _, err := LoadEnv(); err == nil {
		t.Fatal("expected error for malformed TOML")
	}
}

func TestLoadEnvBadValueType(t *testing.T) {
	isolateWrapperEnv(t)
	dir := t.TempDir()
	baseWrapperEnv(t, dir)
	cfgPath := filepath.Join(dir, "badtype.toml")
	if err := os.WriteFile(cfgPath, []byte("[host]\nuid = \"not-an-int\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PINP_CONFIG", cfgPath)
	if _, err := LoadEnv(); err == nil {
		t.Fatal("expected error for a bad value type")
	}
}

func TestLoadEnvInvalidSELinux(t *testing.T) {
	isolateWrapperEnv(t)
	dir := t.TempDir()
	baseWrapperEnv(t, dir)
	t.Setenv("PINP_SELINUX", "maybe")
	if _, err := LoadEnv(); err == nil {
		t.Fatal("expected error for invalid PINP_SELINUX")
	}
}

func TestLoadEnvInvalidSELinuxFromConfig(t *testing.T) {
	isolateWrapperEnv(t)
	dir := t.TempDir()
	baseWrapperEnv(t, dir)
	cfgPath := filepath.Join(dir, "pinp.toml")
	if err := os.WriteFile(cfgPath, []byte("[host]\nselinux = \"bogus\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PINP_CONFIG", cfgPath)
	if _, err := LoadEnv(); err == nil {
		t.Fatal("expected error for invalid selinux from config")
	}
}
