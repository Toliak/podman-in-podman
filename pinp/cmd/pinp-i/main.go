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
	sub := ""
	if idx >= 0 {
		sub = args[idx]
	}
	if sub == "check" {
		return pinp.RunCheckCommand(env, args[idx+1:])
	}
	if sub == "run" || sub == "create" {
		tail := args[idx+1:]
		if value, start, end, found := pinp.FindUserns(tail); found {
			if value != "keep-id" {
				return fmt.Errorf("conflicting arguments already supplied: --userns=%s (unsupported: pinp needs its own uid maps for nested podman); set PINP_PASSTHROUGH=1 to bypass injection", value)
			}
			tail = pinp.StripUserns(tail, start, end)
			args = append(args[:idx+1:idx+1], tail...)
		}
		if conflicts := pinp.Conflicts(tail); len(conflicts) > 0 {
			return fmt.Errorf("conflicting arguments already supplied: %s; set PINP_PASSTHROUGH=1 to bypass injection", strings.Join(conflicts, ", "))
		}
		args = pinp.InjectAt(args, idx, pinp.InnerRunArgs(env))
	}
	if env.Explain {
		if sub == "build" {
			pinp.MaybeBuildSpike(env)
		}
		return pinp.Explain(env.Podman, args)
	}
	if sub == "build" {
		pinp.MaybeBuildSpike(env)
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
