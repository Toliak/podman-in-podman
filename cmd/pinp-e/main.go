package main

import (
	"fmt"
	"os"
	"strings"

	"pinp/internal/pinp"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "pinp-e:", err)
		os.Exit(1)
	}
}

func run() error {
	env, err := pinp.LoadEnv()
	if err != nil {
		return err
	}
	args := os.Args[1:]
	if env.Passthrough {
		if env.Explain {
			return pinp.Explain(env.Podman, args)
		}
		return pinp.Exec(env.Podman, args)
	}
	idx, err := pinp.FindSubcommand(args)
	if err != nil {
		return err
	}
	if idx >= 0 {
		sub := args[idx]
		if sub == "run" || sub == "create" {
			if conflicts := pinp.Conflicts(args[idx+1:]); len(conflicts) > 0 {
				return fmt.Errorf("conflicting arguments already supplied: %s", strings.Join(conflicts, ", "))
			}
			uidRanges, err := pinp.ParseSubIDFile(env.SubUIDFile, env.User)
			if err != nil {
				return err
			}
			gidRanges, err := pinp.ParseSubIDFile(env.SubGIDFile, env.User)
			if err != nil {
				return err
			}
			if err := pinp.ValidateHostRanges(uidRanges, gidRanges); err != nil {
				return err
			}
			uidmaps, gidmaps := pinp.OuterMaps(env.UID, env.GID, pinp.DefaultTotal)
			args = pinp.InjectAt(args, idx, pinp.OuterRunArgs(env, uidmaps, gidmaps))
		}
	}
	if env.Explain {
		return pinp.Explain(env.Podman, args)
	}
	if _, err := os.Stat("/run/.containerenv"); err == nil {
		fmt.Fprintln(os.Stderr, "pinp-e: warning: running inside a container; pinp-e is meant for the host")
	}
	return pinp.Exec(env.Podman, args)
}
