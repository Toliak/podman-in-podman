package pinp

import (
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	DefaultTotal        = 65536
	DefaultStoreOpts    = ":ro"
	DefaultFuseDevice   = "/dev/fuse"
	SharedStorePath     = "/var/lib/shared"
	ContainerRangeStart = 100000
	DefaultInnerUIDMaps = "0:0:1500 65534:1500:2"
	DefaultSubUIDFile   = "/etc/subuid"
	DefaultSubGIDFile   = "/etc/subgid"
	DefaultNewUIDMap    = "/usr/bin/newuidmap"
	DefaultNewGIDMap    = "/usr/bin/newgidmap"
	DefaultUIDMapFile   = "/proc/self/uid_map"
	DefaultGIDMapFile   = "/proc/self/gid_map"
	DefaultSetcap       = "setcap"
)

type Env struct {
	Podman           string
	Echo             bool
	Explain          bool
	Passthrough      bool
	SkipChecks       bool
	UID              int
	GID              int
	User             string
	SubUIDFile       string
	SubGIDFile       string
	Store            string
	StoreOpts        string
	FuseDevice       string
	SharedStore      string
	NewUIDMap        string
	NewGIDMap        string
	UIDMapFile       string
	GIDMapFile       string
	Setcap           string
	InnerMaps        []string
	UsrShareDir      string
	EtcDir           string
	ConfigHome       string
	SELinux          string
	SELinuxFS        string
	InjectConfigArgs bool
}

func boolEnv(name string) bool {
	v := strings.TrimSpace(os.Getenv(name))
	if v == "" {
		return false
	}
	if b, err := strconv.ParseBool(v); err == nil {
		return b
	}
	return v == "1"
}

func intEnv(name string, fallback int) (int, error) {
	v, ok := os.LookupEnv(name)
	if !ok || strings.TrimSpace(v) == "" {
		return fallback, nil
	}
	n, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil {
		return 0, fmt.Errorf("invalid %s=%q: %w", name, v, err)
	}
	return n, nil
}

func LoadEnv() (*Env, error) {
	cfg, err := loadWrapperConfig()
	if err != nil {
		return nil, err
	}
	uid, err := intVal("PINP_UID", cfg.Host.UID, os.Getuid())
	if err != nil {
		return nil, err
	}
	gid, err := intVal("PINP_GID", cfg.Host.GID, os.Getgid())
	if err != nil {
		return nil, err
	}
	home := strVal("PINP_HOME", cfg.Host.Home, "")
	if home == "" {
		home, err = os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("resolve home: %w", err)
		}
	}
	userName := strVal("PINP_USER", cfg.Host.User, "")
	if userName == "" {
		current, err := user.Current()
		if err != nil {
			return nil, fmt.Errorf("resolve user: %w", err)
		}
		userName = current.Username
	}
	configHome := filepath.Join(home, ".config", "containers")
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		configHome = filepath.Join(xdg, "containers")
	}
	selinux := strVal("PINP_SELINUX", cfg.Host.SELinux, "auto")
	switch selinux {
	case "auto", "on", "off":
	default:
		return nil, fmt.Errorf("invalid PINP_SELINUX=%q: must be one of auto, on, off", selinux)
	}
	return &Env{
		Podman:           strVal("PINP_PODMAN", cfg.Wrapper.Podman, "podman"),
		Echo:             boolVal("PINP_ECHO", cfg.Wrapper.Echo, true),
		Explain:          boolVal("PINP_EXPLAIN", cfg.Wrapper.Explain, false),
		Passthrough:      boolVal("PINP_PASSTHROUGH", cfg.Wrapper.Passthrough, false),
		SkipChecks:       boolVal("PINP_SKIP_CHECKS", cfg.Wrapper.SkipChecks, false),
		UID:              uid,
		GID:              gid,
		User:             userName,
		SubUIDFile:       strVal("PINP_SUBUID_FILE", cfg.Host.SubUIDFile, DefaultSubUIDFile),
		SubGIDFile:       strVal("PINP_SUBGID_FILE", cfg.Host.SubGIDFile, DefaultSubGIDFile),
		Store:            strVal("PINP_STORE", cfg.Host.Store, filepath.Join(home, ".local/share/containers/storage")),
		StoreOpts:        strVal("PINP_STORE_OPTS", cfg.Host.StoreOpts, DefaultStoreOpts),
		FuseDevice:       strVal("PINP_FUSE_DEVICE", cfg.Host.FuseDevice, DefaultFuseDevice),
		SharedStore:      strVal("PINP_SHARED_STORE", cfg.Host.SharedStore, SharedStorePath),
		NewUIDMap:        strVal("PINP_NEWUIDMAP", cfg.Host.NewUIDMap, DefaultNewUIDMap),
		NewGIDMap:        strVal("PINP_NEWGIDMAP", cfg.Host.NewGIDMap, DefaultNewGIDMap),
		UIDMapFile:       strVal("PINP_UID_MAP_FILE", cfg.Host.UIDMapFile, DefaultUIDMapFile),
		GIDMapFile:       strVal("PINP_GID_MAP_FILE", cfg.Host.GIDMapFile, DefaultGIDMapFile),
		Setcap:           strVal("PINP_SETCAP", cfg.Host.Setcap, DefaultSetcap),
		InnerMaps:        strings.Fields(strVal("PINP_INNER_UIDMAPS", cfg.L1.InnerUIDMaps, DefaultInnerUIDMaps)),
		UsrShareDir:      strVal("PINP_USR_SHARE_CONTAINERS", cfg.Host.UsrShareContainers, "/usr/share/containers"),
		EtcDir:           strVal("PINP_ETC_CONTAINERS", cfg.Host.EtcContainers, "/etc/containers"),
		SELinux:          selinux,
		SELinuxFS:        strVal("PINP_SELINUXFS", cfg.Host.SELinuxFS, "/sys/fs/selinux"),
		ConfigHome:       configHome,
		InjectConfigArgs: boolVal("PINP_CONFIG_ARGS", cfg.L1.ConfigArgs, true),
	}, nil
}
