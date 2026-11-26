package pinp

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"strconv"
	"strings"

	"github.com/spf13/pflag"
)

const (
	// nobodyID is the identity-mapped overflow extent produced by OuterMaps
	// ("65534:65534:2"). It is never a subordinate range for the L1 user.
	nobodyID = 65534
	// maxSubIDRanges mirrors the newuidmap 5-extent ceiling (AGENTS.md).
	maxSubIDRanges = 5
)

// IDMapExtent is one "inside parent length" line of /proc/self/{uid,gid}_map.
// Inside is the ID as seen inside the L1 user namespace, i.e. an L1 UID/GID.
type IDMapExtent struct {
	Inside int `json:"inside"`
	Parent int `json:"parent"`
	Length int `json:"length"`
}

// ParseIDMap parses the three-column proc id-map format.
func ParseIDMap(data []byte) ([]IDMapExtent, error) {
	var out []IDMapExtent
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) != 3 {
			return nil, fmt.Errorf("bad id map line %q", line)
		}
		inside, err := strconv.Atoi(fields[0])
		if err != nil {
			return nil, fmt.Errorf("bad inside ID in %q: %w", line, err)
		}
		parent, err := strconv.Atoi(fields[1])
		if err != nil {
			return nil, fmt.Errorf("bad parent ID in %q: %w", line, err)
		}
		length, err := strconv.Atoi(fields[2])
		if err != nil {
			return nil, fmt.Errorf("bad length in %q: %w", line, err)
		}
		out = append(out, IDMapExtent{Inside: inside, Parent: parent, Length: length})
	}
	return out, nil
}

// ParseIDMapFile reads and parses an id-map file.
func ParseIDMapFile(path string) ([]IDMapExtent, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return ParseIDMap(data)
}

// IsHostIdentityMap reports whether extents describe a host (non-nested)
// identity map, where no L1 subordinate ranges can be derived. A single extent
// or a wide extent with inside==parent==0 ("0 0 4294967295") is such a map.
// A one-ID "0 0 1" extent is NOT a host identity map: with keep-id:uid=0 the
// host user legitimately lands at container UID 0 alongside other extents.
func IsHostIdentityMap(extents []IDMapExtent) bool {
	if len(extents) < 2 {
		return true
	}
	for _, e := range extents {
		if e.Inside == 0 && e.Parent == 0 && e.Length > 1 {
			return true
		}
	}
	return false
}

// PrepareTarget is the L1 user that owns the generated /etc/sub{u,g}id lines.
// It is deliberately decoupled from the user running --write: at L1 runtime
// --write runs as root while the files must belong to the non-root L1 podman
// user that the host user is mapped to.
type PrepareTarget struct {
	User string
	UID  int
	GID  int
}

// lookupUserByID resolves an L1 uid to a username via /etc/passwd. It is a
// variable so unit tests can stub it without depending on the host's passwd.
var lookupUserByID = func(uid int) (string, bool) {
	u, err := user.LookupId(strconv.Itoa(uid))
	if err != nil {
		return "", false
	}
	return u.Username, true
}

// hostUserInside returns the inside id of the <id>:0:1 extent -- the L1 user
// the host user (parent 0) is mapped to, wherever the keep-id target put it.
// A wide 0 0 <large> host identity extent is not a host-user extent.
func hostUserInside(extents []IDMapExtent) (int, bool) {
	for _, e := range extents {
		if e.Parent == 0 && e.Length == 1 {
			return e.Inside, true
		}
	}
	return 0, false
}

// DeriveSubIDRanges turns the "ID-inside-ns" column of an L1 id map into the
// subordinate-ID ranges for that user's /etc/sub{u,g}id. UID and GID are
// derived completely separately (host UID != GID). It drops the extent whose
// parent is 0 (the host user's own id, wherever the keep-id target placed it),
// container id 0 (L1 root), and identity-mapped extents (including the nobody
// overflow), then caps each range at maxSize (0 = unlimited) and keeps at most
// maxSubIDRanges extents.
func DeriveSubIDRanges(extents []IDMapExtent, maxSize int) ([]Range, []string) {
	var warnings []string
	var ranges []Range
	for _, e := range extents {
		if e.Parent == 0 || e.Inside == e.Parent || e.Inside == nobodyID {
			continue
		}
		start, size := e.Inside, e.Length
		if start == 0 {
			start, size = 1, size-1
		}
		if size <= 0 {
			continue
		}
		if maxSize > 0 && size > maxSize {
			size = maxSize
		}
		ranges = append(ranges, Range{Start: start, Size: size})
	}
	if len(ranges) > maxSubIDRanges {
		ranges = ranges[:maxSubIDRanges]
		warnings = append(warnings, fmt.Sprintf("more than %d extents; kept the first %d", maxSubIDRanges, maxSubIDRanges))
	}
	return ranges, warnings
}

// SubIDFileContent renders the derived ranges as /etc/sub{u,g}id lines.
func SubIDFileContent(user string, ranges []Range) string {
	var b strings.Builder
	for _, r := range ranges {
		fmt.Fprintf(&b, "%s:%d:%d\n", user, r.Start, r.Size)
	}
	return b.String()
}

// CapCommands returns the shell commands that (re)apply file capabilities to
// the id-map helpers, mirroring tests/selinux/guest/image/Containerfile.
func CapCommands(env *Env, uid, gid bool) []string {
	var cmds []string
	var paths []string
	if uid {
		paths = append(paths, env.NewUIDMap)
	}
	if gid {
		paths = append(paths, env.NewGIDMap)
	}
	if len(paths) > 0 {
		cmds = append(cmds, "chmod -s "+strings.Join(paths, " "))
	}
	if uid {
		cmds = append(cmds, "setcap cap_setuid+ep "+env.NewUIDMap)
	}
	if gid {
		cmds = append(cmds, "setcap cap_setgid+ep "+env.NewGIDMap)
	}
	return cmds
}

type prepareFileOut struct {
	File  string   `json:"file"`
	Lines []string `json:"lines"`
}

type prepareOut struct {
	User     string         `json:"user"`
	SubUID   prepareFileOut `json:"subuid"`
	SubGID   prepareFileOut `json:"subgid"`
	Caps     []string       `json:"caps"`
	Warnings []string       `json:"warnings"`
}

func selectPair(changed bool, uid, gid bool, x string) (bool, bool, error) {
	if !changed {
		return true, true, nil
	}
	switch x {
	case "":
		return uid, gid, nil
	case "u", "uid":
		return true, false, nil
	case "g", "gid":
		return false, true, nil
	case "ug", "both", "all":
		return true, true, nil
	default:
		return false, false, fmt.Errorf("invalid selector %q (want u, g, or ug)", x)
	}
}

func linesFor(user string, ranges []Range) []string {
	content := SubIDFileContent(user, ranges)
	if content == "" {
		return []string{}
	}
	return strings.Split(strings.TrimSuffix(content, "\n"), "\n")
}

// resolveTarget derives the owner of the L1 /etc/sub{u,g}id lines. By default
// it is the L1 user the host user is mapped to (the parent-0, length-1 extent),
// resolved to a name via /etc/passwd; --user / PINP_USER override it. It is
// intentionally independent of the user executing --write (root at L1 runtime).
func resolveTarget(env *Env, uidExtents, gidExtents []IDMapExtent, overrideUser string) (PrepareTarget, []string) {
	var warnings []string
	target := PrepareTarget{User: env.User, UID: env.UID, GID: env.GID}
	if id, ok := hostUserInside(uidExtents); ok {
		target.UID = id
	}
	if id, ok := hostUserInside(gidExtents); ok {
		target.GID = id
	}
	switch {
	case overrideUser != "":
		target.User = overrideUser
	case env.UserSet:
		target.User = env.User
	default:
		if name, ok := lookupUserByID(target.UID); ok {
			target.User = name
		} else {
			warnings = append(warnings, fmt.Sprintf("no passwd entry for uid %d; using numeric uid as the /etc/subuid owner", target.UID))
			target.User = strconv.Itoa(target.UID)
		}
	}
	return target, warnings
}

// ResolvePrepareTarget reads the id-map files and resolves the target. Used by
// the L1 preflight consistency check so it compares against the same owner as
// pinp-prepare --write.
func ResolvePrepareTarget(env *Env) (PrepareTarget, []string) {
	uidExtents, _ := ParseIDMapFile(env.UIDMapFile)
	gidExtents, _ := ParseIDMapFile(env.GIDMapFile)
	return resolveTarget(env, uidExtents, gidExtents, "")
}

// subidOwnerIsRoot reports whether the resolved owner would produce root-owned
// subordinate-ID lines, which a rootless inner podman cannot use.
func subidOwnerIsRoot(t PrepareTarget) bool {
	return t.User == "root" || t.User == "0"
}

// applyFileCaps clears setuid AND setgid on the helper and runs setcap. Any
// failure (EPERM/EINVAL inside a userns, missing binary, ...) is returned as a
// warning string; it never hard-fails prepare (caps are best applied at image
// build time).
func applyFileCaps(env *Env, path, cap string) string {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Sprintf("%s: %v", path, err)
	}
	if err := os.Chmod(path, info.Mode()&^(os.ModeSetuid|os.ModeSetgid)); err != nil {
		return fmt.Sprintf("chmod -s %s: %v", path, err)
	}
	cmd := exec.Command(env.Setcap, cap, path)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Sprintf("setcap %s %s: %v (%s)", cap, path, err, strings.TrimSpace(string(out)))
	}
	return ""
}

// RunPrepareCommand implements `pinp-i pinp-prepare` (aliases prepare/gen-sub).
// By default it prints the generated /etc/subuid + /etc/subgid content and the
// file-cap commands to stdout with zero side effects; --write materializes the
// files and applies the caps. It never injects argv, execs podman, or echoes.
func RunPrepareCommand(env *Env, args []string) error {
	fs := pflag.NewFlagSet("pinp-i pinp-prepare", pflag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	genUID := fs.Bool("gen-subuid", false, "generate /etc/subuid")
	genGID := fs.Bool("gen-subgid", false, "generate /etc/subgid")
	genXID := fs.String("gen-subxid", "", "select files: u|g|ug")
	capUID := fs.Bool("cap-newuidmap", false, "print/apply the newuidmap file cap")
	capGID := fs.Bool("cap-newgidmap", false, "print/apply the newgidmap file cap")
	capXID := fs.String("cap-newxidmap", "", "select caps: u|g|ug")
	write := fs.Bool("write", false, "write the files and apply file capabilities")
	maxSize := fs.Int("max-size", 0, "cap each derived range at this size (0 = unlimited)")
	jsonOut := fs.Bool("json", false, "emit the result as JSON")
	userFlag := fs.String("user", "", "user that owns the generated /etc/subuid and /etc/subgid lines (default: derived from the id map)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		return fmt.Errorf("pinp-prepare takes no positional arguments: %v", fs.Args())
	}

	genChanged := fs.Changed("gen-subuid") || fs.Changed("gen-subgid") || fs.Changed("gen-subxid")
	selUID, selGID, err := selectPair(genChanged, *genUID, *genGID, *genXID)
	if err != nil {
		return err
	}
	capChanged := fs.Changed("cap-newuidmap") || fs.Changed("cap-newgidmap") || fs.Changed("cap-newxidmap")
	selCapUID, selCapGID, err := selectPair(capChanged, *capUID, *capGID, *capXID)
	if err != nil {
		return err
	}

	warnings := []string{}
	refused := false
	var uidRanges, gidRanges []Range

	uidExtents, uidErr := ParseIDMapFile(env.UIDMapFile)
	if uidErr != nil {
		refused = true
		warnings = append(warnings, fmt.Sprintf("read %s: %v", env.UIDMapFile, uidErr))
	} else if IsHostIdentityMap(uidExtents) {
		refused = true
		warnings = append(warnings, "not inside a nested userns (uid_map is a host identity map); refusing to derive subordinate IDs")
	} else {
		r, w := DeriveSubIDRanges(uidExtents, *maxSize)
		uidRanges = r
		warnings = append(warnings, w...)
	}

	gidExtents, gidErr := ParseIDMapFile(env.GIDMapFile)
	if gidErr != nil {
		refused = true
		warnings = append(warnings, fmt.Sprintf("read %s: %v", env.GIDMapFile, gidErr))
	} else if IsHostIdentityMap(gidExtents) {
		refused = true
		warnings = append(warnings, "not inside a nested userns (gid_map is a host identity map); refusing to derive subordinate IDs")
	} else {
		r, w := DeriveSubIDRanges(gidExtents, *maxSize)
		gidRanges = r
		warnings = append(warnings, w...)
	}

	target, targetWarnings := resolveTarget(env, uidExtents, gidExtents, *userFlag)
	warnings = append(warnings, targetWarnings...)

	subuidLines := linesFor(target.User, uidRanges)
	subgidLines := linesFor(target.User, gidRanges)
	caps := CapCommands(env, selCapUID, selCapGID)

	if *write && !refused {
		if subidOwnerIsRoot(target) {
			warnings = append(warnings, "--write: /etc/subuid owner resolves to root; a rootless inner podman cannot use root-owned subordinate ranges (set --user / PINP_USER; writing anyway)")
		}
		if selUID {
			if err := os.WriteFile(env.SubUIDFile, []byte(SubIDFileContent(target.User, uidRanges)), 0o644); err != nil {
				return fmt.Errorf("write %s: %w", env.SubUIDFile, err)
			}
		}
		if selGID {
			if err := os.WriteFile(env.SubGIDFile, []byte(SubIDFileContent(target.User, gidRanges)), 0o644); err != nil {
				return fmt.Errorf("write %s: %w", env.SubGIDFile, err)
			}
		}
		if selCapUID {
			if w := applyFileCaps(env, env.NewUIDMap, "cap_setuid+ep"); w != "" {
				warnings = append(warnings, w)
			}
		}
		if selCapGID {
			if w := applyFileCaps(env, env.NewGIDMap, "cap_setgid+ep"); w != "" {
				warnings = append(warnings, w)
			}
		}
	} else if *write && refused {
		warnings = append(warnings, "--write skipped: not inside a nested userns")
	}

	out := prepareOut{
		User:     target.User,
		SubUID:   prepareFileOut{File: env.SubUIDFile, Lines: subuidLines},
		SubGID:   prepareFileOut{File: env.SubGIDFile, Lines: subgidLines},
		Caps:     caps,
		Warnings: warnings,
	}
	if !selUID {
		out.SubUID.Lines = []string{}
	}
	if !selGID {
		out.SubGID.Lines = []string{}
	}

	if *jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(out); err != nil {
			return err
		}
	} else {
		if selUID {
			fmt.Fprintf(os.Stdout, "# %s\n%s", env.SubUIDFile, SubIDFileContent(target.User, uidRanges))
		}
		if selGID {
			fmt.Fprintf(os.Stdout, "# %s\n%s", env.SubGIDFile, SubIDFileContent(target.User, gidRanges))
		}
		if len(caps) > 0 {
			fmt.Fprintln(os.Stdout, "# file capabilities")
			for _, c := range caps {
				fmt.Fprintln(os.Stdout, c)
			}
		}
	}
	for _, w := range warnings {
		fmt.Fprintln(os.Stderr, "pinp-i: warning:", w)
	}
	return nil
}
