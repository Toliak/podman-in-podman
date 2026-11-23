package pinp

import (
	"fmt"
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

func OuterRunArgs(env *Env, uidmaps, gidmaps []string) []string {
	args := []string{
		"--security-opt", "label=disable",
		"--device", DefaultFuseDevice,
		"-v", env.Store + ":" + SharedStorePath + env.StoreOpts,
	}
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
