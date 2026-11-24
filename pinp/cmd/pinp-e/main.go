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
		return pinp.Exec(env.Podman, args, env.Echo)
	}
	idx, err := pinp.FindSubcommand(args)
	if err != nil {
		return err
	}
	if idx >= 0 {
		sub := args[idx]
		if sub == "run" || sub == "create" {
			tail := args[idx+1:]
			keep := pinp.DefaultKeepID()
			if value, start, end, found := pinp.FindUserns(tail); found {
				keep, err = pinp.ParseKeepID(value, env.UID, env.GID, pinp.DefaultTotal)
				if err != nil {
					return err
				}
				tail = pinp.StripUserns(tail, start, end)
				args = append(args[:idx+1:idx+1], tail...)
			}
			conflicts := pinp.Conflicts(tail)
			if pinp.SELinuxEnabled(env) {
				if c := pinp.SecurityLabelConflict(tail); c != "" {
					conflicts = append(conflicts, c)
				}
			}
			if len(conflicts) > 0 {
				return fmt.Errorf("conflicting arguments already supplied: %s; set PINP_PASSTHROUGH=1 to bypass injection", strings.Join(conflicts, ", "))
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
			pool := min(pinp.TotalSize(uidRanges), pinp.TotalSize(gidRanges))
			if err := pinp.ValidateKeepID(keep, env.UID, env.GID, pool); err != nil {
				return err
			}
			if (keep.UID == 0 || keep.GID == 0) && env.Store != "" {
				fmt.Fprintln(os.Stderr, "pinp-e: warning: the host user maps to container root (uid=0/gid=0), so the shared image store (/var/lib/shared) is root-owned inside L1 and a rootless inner container engine cannot use it; use bare --userns=keep-id (uid=$UID,gid=$GID) to keep the store usable, or run the L1 engine as an explicit keep-id:uid=U,gid=G target")
			}
			uidmaps, gidmaps := pinp.OuterMaps(env.UID, env.GID, keep.UID, keep.GID, keep.Size)
			args = pinp.InjectAt(args, idx, pinp.OuterRunArgs(env, uidmaps, gidmaps))
		}
	}
	if env.Explain {
		return pinp.Explain(env.Podman, args)
	}
	if _, err := os.Stat("/run/.containerenv"); err == nil {
		fmt.Fprintln(os.Stderr, "pinp-e: warning: running inside a container; pinp-e is meant for the host")
	}
	return pinp.Exec(env.Podman, args, env.Echo)
}
