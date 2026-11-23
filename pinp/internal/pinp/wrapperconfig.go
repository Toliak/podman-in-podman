package pinp

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

type wrapperConfig struct {
	Wrapper struct {
		Podman      *string `toml:"podman"`
		Echo        *bool   `toml:"echo"`
		Explain     *bool   `toml:"explain"`
		Passthrough *bool   `toml:"passthrough"`
		SkipChecks  *bool   `toml:"skip_checks"`
	} `toml:"wrapper"`
	Host struct {
		UID                *int    `toml:"uid"`
		GID                *int    `toml:"gid"`
		User               *string `toml:"user"`
		Home               *string `toml:"home"`
		SubUIDFile         *string `toml:"subuid_file"`
		SubGIDFile         *string `toml:"subgid_file"`
		Store              *string `toml:"store"`
		StoreOpts          *string `toml:"store_opts"`
		FuseDevice         *string `toml:"fuse_device"`
		SharedStore        *string `toml:"shared_store"`
		NewUIDMap          *string `toml:"newuidmap"`
		NewGIDMap          *string `toml:"newgidmap"`
		UsrShareContainers *string `toml:"usr_share_containers"`
		EtcContainers      *string `toml:"etc_containers"`
		SELinux            *string `toml:"selinux"`
		SELinuxFS          *string `toml:"selinuxfs"`
	} `toml:"host"`
	L1 struct {
		InnerUIDMaps *string `toml:"inner_uidmaps"`
	} `toml:"l1"`
}

func wrapperConfigPath() string {
	if v := os.Getenv("PINP_CONFIG"); v != "" {
		return v
	}
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "pinp", "config.toml")
	}
	home := os.Getenv("PINP_HOME")
	if home == "" {
		home = os.Getenv("HOME")
	}
	if home == "" {
		if h, err := os.UserHomeDir(); err == nil {
			home = h
		}
	}
	if home == "" {
		return ""
	}
	return filepath.Join(home, ".config", "pinp", "config.toml")
}

func loadWrapperConfig() (*wrapperConfig, error) {
	path := wrapperConfigPath()
	if path == "" {
		return &wrapperConfig{}, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &wrapperConfig{}, nil
		}
		return nil, fmt.Errorf("read wrapper config %s: %w", path, err)
	}
	var cfg wrapperConfig
	if _, err := toml.Decode(string(data), &cfg); err != nil {
		return nil, fmt.Errorf("decode wrapper config %s: %w", path, err)
	}
	return &cfg, nil
}

func strVal(envName string, cfg *string, def string) string {
	if v, ok := os.LookupEnv(envName); ok {
		return v
	}
	if cfg != nil {
		return *cfg
	}
	return def
}

func boolVal(envName string, cfg *bool, def bool) bool {
	if v, ok := os.LookupEnv(envName); ok && strings.TrimSpace(v) != "" {
		return boolEnv(envName)
	}
	if cfg != nil {
		return *cfg
	}
	return def
}

func intVal(envName string, cfg *int, def int) (int, error) {
	fallback := def
	if cfg != nil {
		fallback = *cfg
	}
	if v, ok := os.LookupEnv(envName); ok && strings.TrimSpace(v) != "" {
		return intEnv(envName, fallback)
	}
	return fallback, nil
}
