package pinp

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/pflag"
)

var ErrCheckFailed = errors.New("preflight checks failed")

func MaybeBuildSpike(env *Env) {
	target, _ := ResolvePrepareTarget(env)
	uidRanges, _ := ParseSubIDFile(env.SubUIDFile, target.User)
	gidRanges, _ := ParseSubIDFile(env.SubGIDFile, target.User)
	if len(uidRanges) > 1 || len(gidRanges) > 1 {
		fmt.Fprintln(os.Stderr, BuildSpikeHint(uidRanges, gidRanges))
	}
}

func RunCheckCommand(env *Env, args []string) error {
	fs := pflag.NewFlagSet("pinp-i pinp-check", pflag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	jsonOut := fs.Bool("json", false, "emit the report as JSON")
	noFail := fs.Bool("no-fail", false, "always exit 0")
	strict := fs.Bool("strict", false, "treat warnings as failures")
	only := fs.StringSlice("only", nil, "only keep checks whose name contains one of these substrings")
	if err := fs.Parse(args); err != nil {
		return err
	}

	report := RunChecks(env)
	if len(*only) > 0 {
		filtered := Report{}
		for _, check := range report.Checks {
			for _, needle := range *only {
				if strings.Contains(check.Name, needle) {
					filtered.add(check.Name, check.Status, check.Detail)
					break
				}
			}
		}
		report = filtered
	}

	if *jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(report); err != nil {
			return err
		}
	} else {
		for _, check := range report.Checks {
			fmt.Fprintf(os.Stdout, "%-5s %-28s %s\n", strings.ToUpper(string(check.Status)), check.Name, strings.ReplaceAll(check.Detail, "\n", " "))
		}
		fmt.Fprintf(os.Stdout, "\n%d pass, %d warn, %d fail\n", report.Summary.Pass, report.Summary.Warn, report.Summary.Fail)
	}
	if !*noFail && (report.Summary.Fail > 0 || (*strict && report.Summary.Warn > 0)) {
		return ErrCheckFailed
	}
	return nil
}
