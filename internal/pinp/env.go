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
)

type Env struct {
	Podman        string
	Explain       bool
	Passthrough   bool
	SkipChecks    bool
	UID           int
	GID           int
	User          string
	SubUIDFile    string
	SubGIDFile    string
	Store         string
	StoreOpts     string
	FuseDevice    string
	SharedStore   string
	NewUIDMap     string
	NewGIDMap     string
	InnerMaps     []string
	UsrShareDir   string
	EtcDir        string
	ConfigHome    string
}

func lookup(name, fallback string) string {
	if v, ok := os.LookupEnv(name); ok {
		return v
	}
	return fallback
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
	uid, err := intEnv("PINP_UID", os.Getuid())
	if err != nil {
		return nil, err
	}
	gid, err := intEnv("PINP_GID", os.Getgid())
	if err != nil {
		return nil, err
	}
	home := lookup("PINP_HOME", "")
	if home == "" {
		home, err = os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("resolve home: %w", err)
		}
	}
	userName := lookup("PINP_USER", "")
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
	return &Env{
		Podman:        lookup("PINP_PODMAN", "podman"),
		Explain:       boolEnv("PINP_EXPLAIN"),
		Passthrough:   boolEnv("PINP_PASSTHROUGH"),
		SkipChecks:    boolEnv("PINP_SKIP_CHECKS"),
		UID:           uid,
		GID:           gid,
		User:          userName,
		SubUIDFile:    lookup("PINP_SUBUID_FILE", DefaultSubUIDFile),
		SubGIDFile:    lookup("PINP_SUBGID_FILE", DefaultSubGIDFile),
		Store:         lookup("PINP_STORE", filepath.Join(home, ".local/share/containers/storage")),
		StoreOpts:     lookup("PINP_STORE_OPTS", DefaultStoreOpts),
		FuseDevice:    lookup("PINP_FUSE_DEVICE", DefaultFuseDevice),
		SharedStore:   lookup("PINP_SHARED_STORE", SharedStorePath),
		NewUIDMap:     lookup("PINP_NEWUIDMAP", DefaultNewUIDMap),
		NewGIDMap:     lookup("PINP_NEWGIDMAP", DefaultNewGIDMap),
		InnerMaps:     strings.Fields(lookup("PINP_INNER_UIDMAPS", DefaultInnerUIDMaps)),
		UsrShareDir:   lookup("PINP_USR_SHARE_CONTAINERS", "/usr/share/containers"),
		EtcDir:        lookup("PINP_ETC_CONTAINERS", "/etc/containers"),
		ConfigHome:    configHome,
	}, nil
}
