package main

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"pinp/internal/pinp"
)

func main() {
	if err := run(); err != nil {
		if !errors.Is(err, pinp.ErrCheckFailed) {
			fmt.Fprintln(os.Stderr, "pinp-i:", err)
		}
		os.Exit(1)
	}
}

func run() error {
	env, err := pinp.LoadEnv()
	if err != nil {
		return err
	}
	args := os.Args[1:]
	idx, err := pinp.FindSubcommand(args)
	if err != nil && !env.Passthrough {
		return err
	}
	sub := ""
	if idx >= 0 {
		sub = args[idx]
	}

	// pinp-only subcommands are dispatched before the passthrough short-circuit
	// so PINP_PASSTHROUGH=1 still reaches them. They never inject argv, exec
	// podman, or emit the PINP_ECHO line. The pinp- prefix is reserved: a typo
	// is rejected with a clear error rather than forwarded to podman.
	switch pinp.PinpCommand(sub) {
	case pinp.CommandCheck:
		return pinp.RunCheckCommand(env, args[idx+1:])
	case pinp.CommandPrepare:
		return pinp.RunPrepareCommand(env, args[idx+1:])
	}
	if pinp.IsReservedPinpName(sub) {
		return fmt.Errorf("unknown pinp subcommand %q; known commands: %s", sub, strings.Join(pinp.KnownPinpCommands(), ", "))
	}
	if env.Passthrough {
		if env.Explain {
			return pinp.Explain(env.Podman, args)
		}
		return pinp.Exec(env.Podman, args, env.Echo)
	}
	if sub == "run" || sub == "create" {
		tail := args[idx+1:]
		var keep pinp.KeepID
		userns := false
		if value, start, end, found := pinp.FindUserns(tail); found {
			userns = true
			keep, err = pinp.ParseKeepID(value, env.UID, env.GID, 0)
			if err != nil {
				return err
			}
			tail = pinp.StripUserns(tail, start, end)
			args = append(args[:idx+1:idx+1], tail...)
		}
		if conflicts := pinp.Conflicts(tail); len(conflicts) > 0 {
			return fmt.Errorf("conflicting arguments already supplied: %s; set PINP_PASSTHROUGH=1 to bypass injection", strings.Join(conflicts, ", "))
		}
		var extra []string
		if cfg, err := pinp.LoadEffectiveConfig(env); err == nil && env.InjectConfigArgs {
			extra = append(extra, pinp.ConfigArgs(cfg, args, sub)...)
		}
		// No --userns flag keeps the static inner maps (semantically
		// uid=0,gid=0); an explicit keep-id target is derived from L1's
		// /etc/sub{u,g}id. An explicit PINP_INNER_UIDMAPS override wins verbatim.
		if userns && !env.InnerMapsSet {
			uidRanges, uidErr := pinp.ParseSubIDFile(env.SubUIDFile, env.User)
			gidRanges, gidErr := pinp.ParseSubIDFile(env.SubGIDFile, env.User)
			if uidErr != nil || gidErr != nil || len(uidRanges) == 0 || len(gidRanges) == 0 {
				fmt.Fprintln(os.Stderr, "pinp-i: warning: cannot read L1 subordinate ID ranges; leaving the static inner uid maps (target-aware keep-id unavailable)")
				extra = append(extra, pinp.InnerRunArgs(env)...)
			} else {
				uidMaps, gidMaps, warnings, err := pinp.DeriveInnerRunMaps(uidRanges, gidRanges, keep)
				if err != nil {
					return err
				}
				for _, w := range warnings {
					fmt.Fprintln(os.Stderr, "pinp-i: warning:", w)
				}
				extra = append(extra, pinp.InnerMapsArgs(uidMaps, gidMaps)...)
			}
		} else {
			extra = append(extra, pinp.InnerRunArgs(env)...)
		}
		args = pinp.InjectAt(args, idx, extra)
	}
	if sub == "build" && idx >= 0 {
		if cfg, err := pinp.LoadEffectiveConfig(env); err == nil && env.InjectConfigArgs {
			args = pinp.InjectAt(args, idx, pinp.ConfigArgs(cfg, args, "build"))
		}
		if env.InjectBuildMaps {
			if conflicts := pinp.BuildConflicts(args); len(conflicts) > 0 {
				return fmt.Errorf("conflicting arguments already supplied: %s; set PINP_PASSTHROUGH=1 to bypass injection", strings.Join(conflicts, ", "))
			}
			uid, gid, warnings := pinp.BuildMaps(env)
			for _, w := range warnings {
				fmt.Fprintln(os.Stderr, "pinp-i: warning:", w)
			}
			args = pinp.InjectAt(args, idx, pinp.BuildMapArgs(uid, gid))
		}
	}
	if env.Explain {
		return pinp.Explain(env.Podman, args)
	}
	if !env.SkipChecks && (sub == "run" || sub == "create" || sub == "build") {
		report := pinp.RunChecks(env)
		if report.Summary.Fail > 0 {
			for _, check := range report.Checks {
				if check.Status == pinp.StatusFail {
					fmt.Fprintf(os.Stderr, "FAIL %s: %s\n", check.Name, strings.ReplaceAll(check.Detail, "\n", " "))
				}
			}
			return fmt.Errorf("%d preflight check(s) failed; set PINP_SKIP_CHECKS=1 to bypass", report.Summary.Fail)
		}
	}
	return pinp.Exec(env.Podman, args, env.Echo)
}
