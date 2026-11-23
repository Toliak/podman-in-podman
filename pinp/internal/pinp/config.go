package pinp

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"
)

type ContainersConfig struct {
	Containers struct {
		NetNS          string   `toml:"netns"`
		IPCNs          string   `toml:"ipcns"`
		UTSNs          string   `toml:"utsns"`
		PIDNS          string   `toml:"pidns"`
		CgroupNS       string   `toml:"cgroupns"`
		Cgroups        string   `toml:"cgroups"`
		UserNS         string   `toml:"userns"`
		LogDriver      string   `toml:"log_driver"`
		Volumes        []string `toml:"volumes"`
		DefaultSysctls []string `toml:"default_sysctls"`
	} `toml:"containers"`
	Engine struct {
		CgroupManager string `toml:"cgroup_manager"`
		EventsLogger  string `toml:"events_logger"`
		Runtime       string `toml:"runtime"`
	} `toml:"engine"`
}

type StorageConfig struct {
	Storage struct {
		Driver  string `toml:"driver"`
		Options struct {
			AdditionalImageStores []string `toml:"additionalimagestores"`
			Overlay               struct {
				MountProgram string `toml:"mount_program"`
				MountOpt     string `toml:"mountopt"`
			} `toml:"overlay"`
		} `toml:"options"`
	} `toml:"storage"`
}

type EffectiveConfig struct {
	Containers ContainersConfig
	Storage    StorageConfig
	Sources    []string
}

type ConfigSetting struct {
	Key   string
	Want  string
	Flags []string
	Build bool
	Get   func(*EffectiveConfig) string
}

var ConfigSettings = []ConfigSetting{
	{Key: "netns", Want: "host", Flags: []string{"--network", "host"}, Build: true, Get: func(c *EffectiveConfig) string { return c.Containers.Containers.NetNS }},
	{Key: "ipcns", Want: "host", Flags: []string{"--ipc", "host"}, Build: true, Get: func(c *EffectiveConfig) string { return c.Containers.Containers.IPCNs }},
	{Key: "utsns", Want: "host", Flags: []string{"--uts", "host"}, Build: true, Get: func(c *EffectiveConfig) string { return c.Containers.Containers.UTSNs }},
	{Key: "cgroupns", Want: "host", Flags: []string{"--cgroupns", "host"}, Build: true, Get: func(c *EffectiveConfig) string { return c.Containers.Containers.CgroupNS }},
	{Key: "pidns", Want: "host", Flags: []string{"--pid", "host"}, Build: true, Get: func(c *EffectiveConfig) string { return c.Containers.Containers.PIDNS }},
	{Key: "cgroups", Want: "enabled", Flags: []string{"--cgroups", "enabled"}, Build: false, Get: func(c *EffectiveConfig) string { return c.Containers.Containers.Cgroups }},
	{Key: "log_driver", Want: "k8s-file", Flags: []string{"--log-driver", "k8s-file"}, Build: false, Get: func(c *EffectiveConfig) string { return c.Containers.Containers.LogDriver }},
	{Key: "runtime", Want: "crun", Flags: []string{"--runtime", "crun"}, Build: true, Get: func(c *EffectiveConfig) string { return c.Containers.Engine.Runtime }},
	{Key: "cgroup_manager", Want: "cgroupfs", Flags: []string{"--cgroup-manager", "cgroupfs"}, Build: true, Get: func(c *EffectiveConfig) string { return c.Containers.Engine.CgroupManager }},
	{Key: "events_logger", Want: "file", Flags: []string{"--events-backend", "file"}, Build: true, Get: func(c *EffectiveConfig) string { return c.Containers.Engine.EventsLogger }},
	{Key: "driver", Want: "overlay", Flags: []string{"--storage-driver", "overlay"}, Build: true, Get: func(c *EffectiveConfig) string { return c.Storage.Storage.Driver }},
	{Key: "mount_program", Want: "/usr/bin/fuse-overlayfs", Flags: []string{"--storage-opt", "overlay.mount_program=/usr/bin/fuse-overlayfs"}, Build: true, Get: func(c *EffectiveConfig) string { return c.Storage.Storage.Options.Overlay.MountProgram }},
}

func loadContainersInto(path string, dst *ContainersConfig) error {
	var fresh ContainersConfig
	md, err := toml.DecodeFile(path, &fresh)
	if err != nil {
		return fmt.Errorf("decode %s: %w", path, err)
	}
	if md.IsDefined("containers", "netns") {
		dst.Containers.NetNS = fresh.Containers.NetNS
	}
	if md.IsDefined("containers", "ipcns") {
		dst.Containers.IPCNs = fresh.Containers.IPCNs
	}
	if md.IsDefined("containers", "utsns") {
		dst.Containers.UTSNs = fresh.Containers.UTSNs
	}
	if md.IsDefined("containers", "pidns") {
		dst.Containers.PIDNS = fresh.Containers.PIDNS
	}
	if md.IsDefined("containers", "cgroupns") {
		dst.Containers.CgroupNS = fresh.Containers.CgroupNS
	}
	if md.IsDefined("containers", "cgroups") {
		dst.Containers.Cgroups = fresh.Containers.Cgroups
	}
	if md.IsDefined("containers", "userns") {
		dst.Containers.UserNS = fresh.Containers.UserNS
	}
	if md.IsDefined("containers", "log_driver") {
		dst.Containers.LogDriver = fresh.Containers.LogDriver
	}
	if md.IsDefined("containers", "volumes") {
		dst.Containers.Volumes = fresh.Containers.Volumes
	}
	if md.IsDefined("containers", "default_sysctls") {
		dst.Containers.DefaultSysctls = fresh.Containers.DefaultSysctls
	}
	if md.IsDefined("engine", "cgroup_manager") {
		dst.Engine.CgroupManager = fresh.Engine.CgroupManager
	}
	if md.IsDefined("engine", "events_logger") {
		dst.Engine.EventsLogger = fresh.Engine.EventsLogger
	}
	if md.IsDefined("engine", "runtime") {
		dst.Engine.Runtime = fresh.Engine.Runtime
	}
	return nil
}

func loadStorageInto(path string, dst *StorageConfig) error {
	var fresh StorageConfig
	md, err := toml.DecodeFile(path, &fresh)
	if err != nil {
		return fmt.Errorf("decode %s: %w", path, err)
	}
	if md.IsDefined("storage", "driver") {
		dst.Storage.Driver = fresh.Storage.Driver
	}
	if md.IsDefined("storage", "options", "additionalimagestores") {
		dst.Storage.Options.AdditionalImageStores = fresh.Storage.Options.AdditionalImageStores
	}
	if md.IsDefined("storage", "options", "overlay", "mount_program") {
		dst.Storage.Options.Overlay.MountProgram = fresh.Storage.Options.Overlay.MountProgram
	}
	if md.IsDefined("storage", "options", "overlay", "mountopt") {
		dst.Storage.Options.Overlay.MountOpt = fresh.Storage.Options.Overlay.MountOpt
	}
	return nil
}

func confFiles(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []string
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".conf") {
			continue
		}
		out = append(out, filepath.Join(dir, entry.Name()))
	}
	sort.Strings(out)
	return out
}

func mainCandidates(env *Env, name string) []string {
	return []string{
		filepath.Join(env.UsrShareDir, name+".conf"),
		filepath.Join(env.EtcDir, name+".conf"),
		filepath.Join(env.ConfigHome, name+".conf"),
	}
}

func dropInDirs(env *Env, name string, uid int) []string {
	uidDir := fmt.Sprint(uid)
	return []string{
		filepath.Join(env.UsrShareDir, name+".conf.d"),
		filepath.Join(env.UsrShareDir, name+".rootless.conf.d"),
		filepath.Join(env.UsrShareDir, name+".rootless.conf.d", uidDir),
		filepath.Join(env.EtcDir, name+".conf.d"),
		filepath.Join(env.EtcDir, name+".rootless.conf.d"),
		filepath.Join(env.EtcDir, name+".rootless.conf.d", uidDir),
		filepath.Join(env.ConfigHome, name+".conf.d"),
	}
}

func mergedPaths(env *Env, name, overrideEnv, lastEnv string, uid int) []string {
	var paths []string
	if override := os.Getenv(overrideEnv); override != "" {
		paths = append(paths, override)
	} else {
		for _, path := range mainCandidates(env, name) {
			if _, err := os.Stat(path); err == nil {
				paths = append(paths, path)
			}
		}
		for _, dir := range dropInDirs(env, name, uid) {
			paths = append(paths, confFiles(dir)...)
		}
	}
	if last := os.Getenv(lastEnv); last != "" {
		paths = append(paths, last)
	}
	return paths
}

func LoadEffectiveConfig(env *Env) (*EffectiveConfig, error) {
	cfg := &EffectiveConfig{}
	for _, path := range mergedPaths(env, "containers", "CONTAINERS_CONF", "CONTAINERS_CONF_OVERRIDE", env.UID) {
		if err := loadContainersInto(path, &cfg.Containers); err != nil {
			return nil, err
		}
		cfg.Sources = append(cfg.Sources, path)
	}
	for _, path := range mergedPaths(env, "storage", "CONTAINERS_STORAGE_CONF", "", env.UID) {
		if err := loadStorageInto(path, &cfg.Storage); err != nil {
			return nil, err
		}
		cfg.Sources = append(cfg.Sources, path)
	}
	return cfg, nil
}
