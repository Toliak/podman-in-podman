package pinp

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

var globalValueFlags = map[string]bool{
	"--cdi-spec-dir":       true,
	"--cgroup-manager":     true,
	"--config":             true,
	"--conmon":             true,
	"-c":                   true,
	"--connection":         true,
	"--events-backend":     true,
	"--hooks-dir":          true,
	"--identity":           true,
	"--imagestore":         true,
	"--log-level":          true,
	"--module":             true,
	"--network-config-dir": true,
	"--out":                true,
	"--root":               true,
	"--runroot":            true,
	"--runtime":            true,
	"--runtime-flag":       true,
	"--ssh":                true,
	"--storage-driver":     true,
	"--storage-opt":        true,
	"--tls-ca":             true,
	"--tls-cert":           true,
	"--tls-details":        true,
	"--tls-key":            true,
	"--tmpdir":             true,
	"--url":                true,
	"--volumepath":         true,
}

func FindSubcommand(args []string) (int, error) {
	for i := 0; i < len(args); i++ {
		token := args[i]
		if token == "--" {
			if i+1 < len(args) {
				return i + 1, nil
			}
			return -1, fmt.Errorf("missing subcommand after --")
		}
		if token == "-" || !strings.HasPrefix(token, "-") {
			return i, nil
		}
		name := token
		hasValue := false
		if idx := strings.IndexByte(token, '='); idx >= 0 {
			name = token[:idx]
			hasValue = true
		}
		if strings.HasPrefix(name, "-c") && !strings.HasPrefix(name, "--") && len(name) > 2 {
			continue
		}
		if globalValueFlags[name] && !hasValue {
			if i+1 >= len(args) {
				return -1, fmt.Errorf("missing value for %s", name)
			}
			i++
		}
	}
	return -1, nil
}

func Conflicts(args []string) []string {
	var found []string
	for i := 0; i < len(args); i++ {
		token := args[i]
		name := token
		value := ""
		hasValue := false
		if idx := strings.IndexByte(token, '='); idx >= 0 {
			name = token[:idx]
			value = token[idx+1:]
			hasValue = true
		}
		switch name {
		case "--uidmap", "--gidmap", "--userns", "--subuidname", "--subgidname":
			found = append(found, name)
		case "--device":
			if !hasValue && i+1 < len(args) {
				value = args[i+1]
				i++
			}
			if strings.HasPrefix(value, DefaultFuseDevice) {
				found = append(found, "--device /dev/fuse")
			}
		case "-v", "--volume", "--mount":
			if !hasValue && i+1 < len(args) {
				value = args[i+1]
				i++
			}
			if strings.Contains(value, SharedStorePath) {
				found = append(found, name+" -> "+SharedStorePath)
			}
		}
	}
	return found
}

func InjectAt(args []string, idx int, extra []string) []string {
	out := make([]string, 0, len(args)+len(extra))
	out = append(out, args[:idx+1]...)
	out = append(out, extra...)
	out = append(out, args[idx+1:]...)
	return out
}

func SELinuxEnabled(env *Env) bool {
	switch env.SELinux {
	case "on":
		return true
	case "off":
		return false
	default:
		_, err := os.Stat(env.SELinuxFS)
		return err == nil
	}
}

func SecurityLabelConflict(args []string) string {
	for i := 0; i < len(args); i++ {
		token := args[i]
		name := token
		value := ""
		hasValue := false
		if idx := strings.IndexByte(token, '='); idx >= 0 {
			name = token[:idx]
			value = token[idx+1:]
			hasValue = true
		}
		if name != "--security-opt" {
			continue
		}
		if !hasValue && i+1 < len(args) {
			value = args[i+1]
		}
		if strings.HasPrefix(value, "label=") {
			return "--security-opt " + value
		}
	}
	return ""
}

func FindUserns(args []string) (value string, start, end int, found bool) {
	for i, token := range args {
		if token == "--userns" {
			if i+1 < len(args) {
				return args[i+1], i, i + 2, true
			}
			return "", i, i + 1, true
		}
		if strings.HasPrefix(token, "--userns=") {
			return strings.TrimPrefix(token, "--userns="), i, i + 1, true
		}
	}
	return "", 0, 0, false
}

func StripUserns(args []string, start, end int) []string {
	out := make([]string, 0, len(args)-(end-start))
	out = append(out, args[:start]...)
	out = append(out, args[end:]...)
	return out
}

// KeepID is the resolved target of a --userns=keep-id... value: the container
// UID/GID where the host user lands, and the size of the user namespace.
type KeepID struct {
	UID  int
	GID  int
	Size int
}

// DefaultKeepID is pinp-e's target when --userns is absent. It overrides
// podman's --userns=host default: the host user becomes container root while
// pinp still creates the userns and the 100000 anchor it needs for nested
// podman.
func DefaultKeepID() KeepID {
	return KeepID{UID: 0, GID: 0, Size: DefaultTotal}
}

// ParseKeepID parses a --userns value into a KeepID. hostUID/hostGID are the
// caller's real IDs (defaults for bare keep-id and for missing uid=/gid=
// options); total is the default userns size. An empty value is an error:
// podman treats --userns="" as an alias for --userns=host. Any non-keep-id
// value (host/auto/nomap/ns:/container:) is rejected with a passthrough hint.
func ParseKeepID(value string, hostUID, hostGID, total int) (KeepID, error) {
	if value == "" {
		return KeepID{}, fmt.Errorf("conflicting arguments already supplied: --userns= (empty value is an alias for --userns=host); set PINP_PASSTHROUGH=1 to bypass injection")
	}
	if value == "keep-id" {
		return KeepID{UID: hostUID, GID: hostGID, Size: total}, nil
	}
	if !strings.HasPrefix(value, "keep-id:") {
		return KeepID{}, fmt.Errorf("conflicting arguments already supplied: --userns=%s (unsupported: pinp needs its own uid maps for nested podman); set PINP_PASSTHROUGH=1 to bypass injection", value)
	}
	k := KeepID{UID: hostUID, GID: hostGID, Size: total}
	for _, opt := range strings.Split(strings.TrimPrefix(value, "keep-id:"), ",") {
		name, val, ok := strings.Cut(opt, "=")
		if !ok {
			return KeepID{}, fmt.Errorf("invalid --userns=%s: option %q needs a value", value, opt)
		}
		n, err := strconv.Atoi(val)
		if err != nil || n < 0 {
			return KeepID{}, fmt.Errorf("invalid --userns=%s: option %q has a bad value", value, opt)
		}
		switch name {
		case "uid":
			k.UID = n
		case "gid":
			k.GID = n
		case "size":
			k.Size = n
		default:
			return KeepID{}, fmt.Errorf("invalid --userns=%s: unknown keep-id option %q", value, name)
		}
	}
	return k, nil
}

func OuterRunArgs(env *Env, uidmaps, gidmaps []string) []string {
	args := make([]string, 0, 6+2*len(uidmaps)+2*len(gidmaps))
	if SELinuxEnabled(env) {
		args = append(args, "--security-opt", "label=disable")
	}
	args = append(args,
		"--device", DefaultFuseDevice,
		"-v", env.Store+":"+SharedStorePath+env.StoreOpts,
	)
	for _, m := range uidmaps {
		args = append(args, "--uidmap", m)
	}
	for _, m := range gidmaps {
		args = append(args, "--gidmap", m)
	}
	return args
}

func InnerRunArgs(env *Env) []string {
	args := make([]string, 0, 2*len(env.InnerMaps))
	for _, m := range env.InnerMaps {
		args = append(args, "--uidmap", m)
	}
	return args
}

// InnerMapsArgs renders separately derived uid and gid inner maps as repeated
// --uidmap/--gidmap flags. Explicit keep-id targets need both because the
// target UID and GID differ; the static fallback stays uidmap-only (podman
// mirrors it to the gid map).
func InnerMapsArgs(uidMaps, gidMaps []string) []string {
	args := make([]string, 0, 2*(len(uidMaps)+len(gidMaps)))
	for _, m := range uidMaps {
		args = append(args, "--uidmap", m)
	}
	for _, m := range gidMaps {
		args = append(args, "--gidmap", m)
	}
	return args
}

var flagAliases = map[string][]string{"--network": {"--net"}}

func HasFlag(args []string, names ...string) bool {
	for _, token := range args {
		name := token
		if idx := strings.IndexByte(token, '='); idx >= 0 {
			name = token[:idx]
		}
		for _, n := range names {
			if name == n {
				return true
			}
		}
	}
	return false
}

func ConfigArgs(cfg *EffectiveConfig, args []string, sub string) []string {
	var out []string
	for _, s := range ConfigSettings {
		if !s.Build && sub == "build" {
			continue
		}
		if s.Get(cfg) != "" {
			continue
		}
		names := append([]string{s.Flags[0]}, flagAliases[s.Flags[0]]...)
		if HasFlag(args, names...) {
			continue
		}
		out = append(out, s.Flags...)
	}
	return out
}
