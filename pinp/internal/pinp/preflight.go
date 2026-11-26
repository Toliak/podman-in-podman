package pinp

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/syndtr/gocapability/capability"
)

type Status string

const (
	StatusPass Status = "pass"
	StatusWarn Status = "warn"
	StatusFail Status = "fail"
)

type Check struct {
	Name   string `json:"name"`
	Status Status `json:"status"`
	Detail string `json:"detail"`
}

type Report struct {
	Checks  []Check `json:"checks"`
	Summary struct {
		Pass int `json:"pass"`
		Warn int `json:"warn"`
		Fail int `json:"fail"`
	} `json:"summary"`
}

func (r *Report) add(name string, status Status, detail string) {
	r.Checks = append(r.Checks, Check{Name: name, Status: status, Detail: detail})
	switch status {
	case StatusPass:
		r.Summary.Pass++
	case StatusWarn:
		r.Summary.Warn++
	case StatusFail:
		r.Summary.Fail++
	}
}

func (r *Report) addPath(name, path string, wantExecutable bool) {
	info, err := os.Stat(path)
	if err != nil {
		r.add(name, StatusFail, fmt.Sprintf("%s: %v", path, err))
		return
	}
	if wantExecutable && info.Mode()&0o111 == 0 {
		r.add(name, StatusFail, path+" is not executable")
		return
	}
	r.add(name, StatusPass, path)
}

func (r *Report) expectSetting(cfg *EffectiveConfig, s ConfigSetting) {
	got := s.Get(cfg)
	if got == "" {
		r.add("config "+s.Key, StatusWarn, "not set; pinp-i injects "+strings.Join(s.Flags, " "))
		return
	}
	if got != s.Want {
		r.add("config "+s.Key, StatusWarn, fmt.Sprintf("%q, expected %q", got, s.Want))
		return
	}
	r.add("config "+s.Key, StatusPass, got)
}

func hasString(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}

func formatRanges(ranges []Range) string {
	parts := make([]string, 0, len(ranges))
	for _, item := range ranges {
		parts = append(parts, fmt.Sprintf("%d:%d", item.Start, item.Size))
	}
	if len(parts) == 0 {
		return "none"
	}
	return strings.Join(parts, " ")
}

func sameRanges(a, b []Range) bool {
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

func (r *Report) checkIDMapHelper(env *Env, name, path string, want capability.Cap, label string) {
	r.addPath(name+" binary", path, true)
	setuid, err := IsSetuid(path)
	if err != nil {
		r.add(name+" setuid", StatusWarn, err.Error())
	} else if setuid {
		r.add(name+" setuid", StatusFail, "binary is setuid-root; expected file capabilities only")
	} else {
		r.add(name+" setuid", StatusPass, "not setuid")
	}
	ok, detail := FileCapStatus(path, want, label)
	if ok {
		r.add(name+" capabilities", StatusPass, detail)
	} else {
		r.add(name+" capabilities", StatusFail, detail)
	}
}

func (r *Report) checkSubID(name, path, user string) []Range {
	ranges, err := ParseSubIDFile(path, user)
	if err != nil {
		r.add(name, StatusFail, err.Error())
		return nil
	}
	if len(ranges) == 0 {
		r.add(name, StatusFail, "no ranges for "+user)
		return nil
	}
	r.add(name, StatusPass, fmt.Sprintf("%s -> %s", user, formatRanges(ranges)))
	return ranges
}

func RunChecks(env *Env) Report {
	var r Report

	if _, err := ResolvePodman(env.Podman); err != nil {
		r.add("podman binary", StatusFail, err.Error())
	} else {
		r.add("podman binary", StatusPass, env.Podman)
	}

	info, infoErr := GetPodmanInfo(env.Podman)
	if infoErr != nil {
		r.add("podman info", StatusFail, infoErr.Error())
	} else {
		r.add("podman info", StatusPass, fmt.Sprintf("buildah=%s runtime=%s", info.Host.BuildahVersion, info.Host.OCIRuntime.Name))
		if info.Host.Security.Rootless {
			r.add("rootless", StatusPass, "true")
		} else {
			r.add("rootless", StatusFail, "podman is not rootless")
		}
		if info.Host.ServiceIsRemote {
			r.add("local podman", StatusFail, "serviceIsRemote=true")
		} else {
			r.add("local podman", StatusPass, "false")
		}
		if info.Store.GraphDriverName == "overlay" {
			r.add("graph driver", StatusPass, "overlay")
		} else {
			r.add("graph driver", StatusWarn, info.Store.GraphDriverName)
		}
		if info.Host.OCIRuntime.Name == "crun" {
			r.add("oci runtime", StatusPass, "crun")
		} else {
			r.add("oci runtime", StatusWarn, info.Host.OCIRuntime.Name)
		}
		if info.Host.CgroupsVersion == "v2" {
			r.add("cgroup version", StatusPass, "v2")
		} else {
			r.add("cgroup version", StatusWarn, info.Host.CgroupsVersion)
		}
		if info.Host.Security.SELinuxEnabled {
			r.add("selinux", StatusWarn, "enabled; pinp-e conditionally injects label=disable; set PINP_SELINUX=off to disable")
		} else {
			r.add("selinux", StatusPass, "disabled")
		}
		r.add("id mappings", StatusPass, fmt.Sprintf("uid=%d gid=%d extents", len(info.Host.IDMappings.UIDMap), len(info.Host.IDMappings.GIDMap)))
		r.add("graph root", StatusPass, info.Store.GraphRoot)
		if info.Host.OCIRuntime.Path != "" {
			r.addPath("runtime binary", info.Host.OCIRuntime.Path, true)
		}
	}

	if _, err := os.Stat("/run/.containerenv"); err == nil {
		r.add("inside container", StatusPass, "/run/.containerenv present")
	} else if marker := os.Getenv("container"); marker != "" {
		r.add("inside container", StatusPass, "container="+marker)
	} else {
		r.add("inside container", StatusWarn, "pinp-i is meant to run inside L1; no container marker found")
	}

	cfg, cfgErr := LoadEffectiveConfig(env)
	if cfgErr != nil {
		r.add("effective config", StatusFail, cfgErr.Error())
	} else {
		r.add("effective config", StatusPass, strings.Join(cfg.Sources, ", "))
		for _, s := range ConfigSettings {
			r.expectSetting(cfg, s)
		}
		if cfg.Containers.Containers.UserNS != "" {
			r.add("config userns", StatusWarn, fmt.Sprintf("containers.conf userns=%q would collide with injected --uidmap and is silently overridden", cfg.Containers.Containers.UserNS))
		}
		if v := os.Getenv("PODMAN_USERNS"); v != "" {
			r.add("env PODMAN_USERNS", StatusWarn, fmt.Sprintf("PODMAN_USERNS=%q would collide with injected --uidmap and is silently overridden", v))
		}
		if hasString(cfg.Storage.Storage.Options.AdditionalImageStores, SharedStorePath) {
			r.add("config additionalimagestores", StatusPass, SharedStorePath)
		} else {
			r.add("config additionalimagestores", StatusWarn, "not set; no podman CLI equivalent; mount /var/lib/shared in L1")
		}
		target, _ := ResolvePrepareTarget(env)
		storageConf := storageConfPath(env, target)
		dropIn := storageDropInPath(storageConf)
		if mounted, err := IsMountPoint(env.SharedStore); err != nil {
			r.add("storage.conf", StatusWarn, fmt.Sprintf("cannot read mount info: %v", err))
		} else if !mounted {
			r.add("storage.conf", StatusPass, env.SharedStore+" is not mounted; shared store disabled")
		} else if _, err := os.Stat(dropIn); err == nil {
			r.add("storage.conf", StatusPass, dropIn)
		} else if hasString(cfg.Storage.Storage.Options.AdditionalImageStores, env.SharedStore) {
			r.add("storage.conf", StatusPass, "additionalimagestores configured in effective config")
		} else {
			r.add("storage.conf", StatusWarn, fmt.Sprintf("mounted but not configured; run pinp-prepare --write (expected %s)", filepath.Clean(dropIn)))
		}
		mountProgram := cfg.Storage.Storage.Options.Overlay.MountProgram
		if mountProgram == "" {
			mountProgram = "/usr/bin/fuse-overlayfs"
		}
		r.addPath("fuse-overlayfs", mountProgram, true)
	}

	r.checkIDMapHelper(env, "newuidmap", env.NewUIDMap, capability.CAP_SETUID, "cap_setuid")
	r.checkIDMapHelper(env, "newgidmap", env.NewGIDMap, capability.CAP_SETGID, "cap_setgid")
	if _, err := exec.LookPath("crun"); err == nil {
		r.add("crun on PATH", StatusPass, "crun")
	} else {
		r.add("crun on PATH", StatusWarn, "crun not found on PATH")
	}
	if _, err := os.Stat(env.FuseDevice); err != nil {
		r.add("/dev/fuse", StatusFail, err.Error())
	} else {
		r.add("/dev/fuse", StatusPass, env.FuseDevice)
	}
	if shared, err := os.Stat(env.SharedStore); err != nil {
		r.add("shared store", StatusFail, fmt.Sprintf("%s: %v", env.SharedStore, err))
	} else if !shared.IsDir() {
		r.add("shared store", StatusFail, env.SharedStore+" is not a directory")
	} else {
		r.add("shared store", StatusPass, env.SharedStore)
	}

	uidRanges := r.checkSubID("subuid", env.SubUIDFile, env.User)
	gidRanges := r.checkSubID("subgid", env.SubGIDFile, env.User)
	status, detail := BuildReadiness(env, uidRanges, gidRanges)
	r.add("build readiness", status, detail)

	if raw, err := os.ReadFile("/proc/sys/user/max_user_namespaces"); err == nil {
		n, convErr := strconv.Atoi(strings.TrimSpace(string(raw)))
		if convErr != nil || n <= 0 {
			r.add("max_user_namespaces", StatusFail, strings.TrimSpace(string(raw)))
		} else {
			r.add("max_user_namespaces", StatusPass, strconv.Itoa(n))
		}
	} else {
		r.add("max_user_namespaces", StatusWarn, err.Error())
	}
	if raw, err := os.ReadFile("/proc/sys/kernel/unprivileged_userns_clone"); err == nil {
		if strings.TrimSpace(string(raw)) == "1" {
			r.add("unprivileged_userns_clone", StatusPass, "1")
		} else {
			r.add("unprivileged_userns_clone", StatusWarn, strings.TrimSpace(string(raw)))
		}
	} else {
		r.add("unprivileged_userns_clone", StatusWarn, "not present")
	}
	for _, idmap := range []struct{ name, path string }{
		{"uid_map", env.UIDMapFile},
		{"gid_map", env.GIDMapFile},
	} {
		raw, err := os.ReadFile(idmap.path)
		if err != nil {
			r.add(idmap.name, StatusWarn, err.Error())
			continue
		}
		r.add(idmap.name, StatusPass, strings.TrimSpace(string(raw)))
	}
	// Derived subordinate-ID info is meaningful only inside a nested L1 userns;
	// on a host identity map there is nothing to compare, so stay quiet.
	target, _ := ResolvePrepareTarget(env)
	for _, c := range []struct {
		label, mapFile, subFile string
	}{
		{"subuid consistency", env.UIDMapFile, env.SubUIDFile},
		{"subgid consistency", env.GIDMapFile, env.SubGIDFile},
	} {
		extents, err := ParseIDMapFile(c.mapFile)
		if err != nil || IsHostIdentityMap(extents) {
			continue
		}
		derived, _ := DeriveSubIDRanges(extents, 0)
		actual, _ := ParseSubIDFile(c.subFile, target.User)
		if !sameRanges(derived, actual) {
			r.add(c.label, StatusWarn, fmt.Sprintf("%s has %s; derived from %s is %s; refresh with pinp-i pinp-prepare", c.subFile, formatRanges(actual), c.mapFile, formatRanges(derived)))
		} else {
			r.add(c.label, StatusPass, formatRanges(derived))
		}
	}
	return r
}
