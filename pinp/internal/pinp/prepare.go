package pinp

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
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

// mountInfoPath returns the file IsMountPoint parses. PINP_MOUNTINFO is a test
// hook (like PINP_UID_MAP_FILE) so the shared-store gate can be exercised
// without a real mount.
func mountInfoPath() string {
	if v := strings.TrimSpace(os.Getenv("PINP_MOUNTINFO")); v != "" {
		return v
	}
	return "/proc/self/mountinfo"
}

// unescapeMountField decodes the octal escapes used in /proc/self/mountinfo
// (space, tab, newline and backslash).
func unescapeMountField(s string) string {
	return strings.NewReplacer(
		`\040`, " ",
		`\011`, "\t",
		`\012`, "\n",
		`\134`, `\`,
	).Replace(s)
}

// IsMountPoint reports whether path is a mount point, by scanning the fifth
// field of /proc/self/mountinfo. This detects pinp-e's read-only bind mount of
// the host store at /var/lib/shared inside L1 (existence alone is not enough,
// because the L1 image bakes the directory).
func IsMountPoint(path string) (bool, error) {
	data, err := os.ReadFile(mountInfoPath())
	if err != nil {
		return false, err
	}
	want := filepath.Clean(path)
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 5 {
			continue
		}
		if filepath.Clean(unescapeMountField(fields[4])) == want {
			return true, nil
		}
	}
	return false, nil
}

// DeriveAdditionalStores merges the effective additionalimagestores with the
// shared store, preserving order and dropping duplicates. The shared store is
// appended only when it is not already present.
func DeriveAdditionalStores(existing []string, shared string) []string {
	out := make([]string, 0, len(existing)+1)
	seen := map[string]bool{}
	for _, store := range existing {
		if store == "" || seen[store] {
			continue
		}
		seen[store] = true
		out = append(out, store)
	}
	if shared != "" && !seen[shared] {
		out = append(out, shared)
	}
	return out
}

// StorageConfContent renders an additionalimagestores drop-in.
func StorageConfContent(stores []string) string {
	var b strings.Builder
	b.WriteString("[storage.options]\nadditionalimagestores = [\n")
	for _, store := range stores {
		fmt.Fprintf(&b, "    %q,\n", store)
	}
	b.WriteString("]\n")
	return b.String()
}

// storageConfPath resolves the storage.conf that the L1 rootless engine reads.
// An explicit override wins; otherwise it is the mapped user's
// ~/.config/containers/storage.conf (not root's, even when --write runs as root).
func storageConfPath(env *Env, target PrepareTarget) string {
	if env.StorageConf != "" {
		return env.StorageConf
	}
	home := target.Home
	if home == "" {
		home = env.Home
	}
	return filepath.Join(home, ".config", "containers", "storage.conf")
}

// storageDropInPath is the non-destructive drop-in carrying additionalimagestores
// next to the resolved storage.conf.
func storageDropInPath(storageConf string) string {
	return filepath.Join(filepath.Dir(storageConf), "storage.conf.d", "pinp-shared-store.conf")
}

// effectiveStores reads the additionalimagestores podman would use for the
// target user. It points ConfigHome at the target's containers dir so a
// root-running --write does not read root's config.
func effectiveStores(env *Env, storageConf string) []string {
	probe := *env
	probe.ConfigHome = filepath.Dir(storageConf)
	cfg, err := LoadEffectiveConfig(&probe)
	if err != nil {
		return nil
	}
	return cfg.Storage.Storage.Options.AdditionalImageStores
}

// writeStorageDropIn writes the drop-in idempotently and, when running as root
// at L1 runtime, hands the file and any newly created parent dirs to the mapped
// user. Non-root callers (e.g. tests) skip the chown silently. A failure is
// returned as a warning string, never a hard error.
func writeStorageDropIn(path, content string, target PrepareTarget) string {
	if existing, err := os.ReadFile(path); err == nil && string(existing) == content {
		return ""
	}
	dir := filepath.Dir(path)
	var created []string
	for p := dir; ; p = filepath.Dir(p) {
		if _, err := os.Stat(p); err == nil {
			break
		}
		created = append(created, p)
		if filepath.Dir(p) == p {
			break
		}
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Sprintf("create %s: %v", dir, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return fmt.Sprintf("write %s: %v", path, err)
	}
	if os.Geteuid() == 0 {
		for _, p := range created {
			if err := os.Chown(p, target.UID, target.GID); err != nil {
				return fmt.Sprintf("chown %s: %v", p, err)
			}
		}
		if err := os.Chown(path, target.UID, target.GID); err != nil {
			return fmt.Sprintf("chown %s: %v", path, err)
		}
	}
	return ""
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
	Home string
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

// lookupUserHome resolves a username to its home directory. It is a variable so
// unit tests can stub it; a miss falls back to env.Home.
var lookupUserHome = func(name string) (string, bool) {
	u, err := user.Lookup(name)
	if err != nil || u.HomeDir == "" {
		return "", false
	}
	return u.HomeDir, true
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
	User        string         `json:"user"`
	SubUID      prepareFileOut `json:"subuid"`
	SubGID      prepareFileOut `json:"subgid"`
	StorageConf prepareFileOut `json:"storage_conf"`
	Caps        []string       `json:"caps"`
	Info        []string       `json:"info"`
	Warnings    []string       `json:"warnings"`
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
	if home, ok := lookupUserHome(target.User); ok {
		target.Home = home
	} else {
		target.Home = env.Home
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
	genStorage := fs.Bool("gen-storage-conf", false, "generate the additionalimagestores storage.conf drop-in")
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

	genChanged := fs.Changed("gen-subuid") || fs.Changed("gen-subgid") || fs.Changed("gen-subxid") || fs.Changed("gen-storage-conf")
	var selUID, selGID, selStorage bool
	var err error
	if genChanged {
		selUID, selGID, err = selectPair(true, *genUID, *genGID, *genXID)
		if err != nil {
			return err
		}
		selStorage = *genStorage
	} else {
		selUID, selGID, selStorage = true, true, true
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

	storageConf := storageConfPath(env, target)
	storagePath := storageDropInPath(storageConf)
	var storageContent string
	storageConfigured := false
	info := []string{}
	if selStorage {
		mounted, mountErr := IsMountPoint(env.SharedStore)
		switch {
		case mountErr != nil:
			info = append(info, fmt.Sprintf("cannot read mount info: %v; skipping additionalimagestores", mountErr))
		case !mounted:
			info = append(info, fmt.Sprintf("%s is not a mount point; skipping additionalimagestores", env.SharedStore))
		default:
			stores := DeriveAdditionalStores(effectiveStores(env, storageConf), env.SharedStore)
			storageContent = StorageConfContent(stores)
			storageConfigured = true
		}
	}

	if *write && selStorage && storageConfigured {
		if subidOwnerIsRoot(target) {
			warnings = append(warnings, fmt.Sprintf("--write: storage.conf owner resolves to root; a rootless inner podman may be unable to use %s (set --user / PINP_USER; writing anyway)", storagePath))
		}
		if w := writeStorageDropIn(storagePath, storageContent, target); w != "" {
			warnings = append(warnings, w)
		}
	}

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
		User:        target.User,
		SubUID:      prepareFileOut{File: env.SubUIDFile, Lines: subuidLines},
		SubGID:      prepareFileOut{File: env.SubGIDFile, Lines: subgidLines},
		StorageConf: prepareFileOut{File: storagePath, Lines: []string{}},
		Caps:        caps,
		Info:        info,
		Warnings:    warnings,
	}
	if storageConfigured {
		out.StorageConf.Lines = strings.Split(strings.TrimSuffix(storageContent, "\n"), "\n")
	}
	if !selUID {
		out.SubUID.Lines = []string{}
	}
	if !selGID {
		out.SubGID.Lines = []string{}
	}
	if !selStorage {
		out.StorageConf.File = ""
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
		if selStorage && storageConfigured {
			fmt.Fprintf(os.Stdout, "# %s\n%s", storagePath, storageContent)
		}
		if len(caps) > 0 {
			fmt.Fprintln(os.Stdout, "# file capabilities")
			for _, c := range caps {
				fmt.Fprintln(os.Stdout, c)
			}
		}
	}
	for _, msg := range info {
		fmt.Fprintln(os.Stderr, "pinp-i: info:", msg)
	}
	for _, w := range warnings {
		fmt.Fprintln(os.Stderr, "pinp-i: warning:", w)
	}
	return nil
}
