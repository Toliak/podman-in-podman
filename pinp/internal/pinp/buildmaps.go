package pinp

import (
	"fmt"
	"sort"
	"strings"
)

const (
	// nobodyExtentSize is the size of the overflow extent (65534..65535) that
	// apt's sandbox (setgroups/setegid to 65534) needs inside a build userns.
	nobodyExtentSize = 2
	// maxBuildExtents mirrors the kernel's 5-extent limit for uid_map/gid_map.
	maxBuildExtents = 5
)

var buildConflictFlags = map[string]bool{
	"--userns":               true,
	"--userns-uid-map":       true,
	"--userns-gid-map":       true,
	"--userns-uid-map-user":  true,
	"--userns-gid-map-group": true,
	"--isolation":            true,
	"--subuidname":           true,
	"--subgidname":           true,
	"--uidmap":               true,
	"--gidmap":               true,
}

// BuildConflicts reports caller-supplied build flags that would collide with
// the derived --userns-*-map extents. Both --flag=value and --flag VALUE forms
// are detected by name only.
func BuildConflicts(args []string) []string {
	var found []string
	for _, token := range args {
		name := token
		if idx := strings.IndexByte(token, '='); idx >= 0 {
			name = token[:idx]
		}
		if buildConflictFlags[name] {
			found = append(found, name)
		}
	}
	return found
}

// NeedsBuildMaps reports whether a build userns needs injected extents. A
// rootless buildah maps container 0 to the L1 user's own id and container
// 1..T to the T subordinate IDs, so container 65534 (nobody) is reachable
// only when T >= 65534. A sparse L1 pool (e.g. 1001 + 32768) cannot map it
// and apt's sandbox then fails with "setgroups 65534 failed".
func NeedsBuildMaps(uidRanges, gidRanges []Range) bool {
	return TotalSize(uidRanges) < nobodyID || TotalSize(gidRanges) < nobodyID
}

func sortedBuildRanges(ranges []Range) []Range {
	out := append([]Range(nil), ranges...)
	sort.Slice(out, func(i, j int) bool {
		if out[i].Start != out[j].Start {
			return out[i].Start < out[j].Start
		}
		return out[i].Size < out[j].Size
	})
	return out
}

// buildMapExtents renders one --userns-*-map triple per subordinate range.
//
// podman build --userns-*-map hostIDs are interpreted in buildah's rootless
// build userns (U_build), NOT as the absolute /etc/sub{u,g}id start values.
// U_build numbers parent IDs sequentially from 1, one extent per range sorted
// by start, so an absolute map anchored at 100000 is out of range and crun's
// direct write of gid_map fails EPERM. crun also rejects an extent whose
// parent range straddles two U_build extents, so each triple stays inside one.
// The last two parent IDs are reserved for container 65534..65535.
func buildMapExtents(ranges []Range) ([]string, []string) {
	var warnings []string
	sorted := sortedBuildRanges(ranges)
	if len(sorted) > maxBuildExtents-1 {
		sorted = sorted[:maxBuildExtents-1]
		warnings = append(warnings, fmt.Sprintf("more than %d subordinate ranges; kept the first %d", maxBuildExtents-1, maxBuildExtents-1))
	}
	for len(sorted) > 0 && sorted[len(sorted)-1].Size < nobodyExtentSize {
		sorted = sorted[:len(sorted)-1]
		warnings = append(warnings, "dropped trailing range too small to host the nobody extent")
	}
	if len(sorted) == 0 {
		return nil, append(warnings, "no subordinate ID range large enough to host the nobody extent")
	}
	total := TotalSize(sorted)
	var out []string
	parent := 1
	for i, r := range sorted {
		size := r.Size
		if i == len(sorted)-1 {
			size -= nobodyExtentSize
		}
		if size > 0 {
			out = append(out, fmt.Sprintf("%d:%d:%d", parent-1, parent, size))
		}
		parent += r.Size
	}
	out = append(out, fmt.Sprintf("%d:%d:%d", nobodyID, total-1, nobodyExtentSize))
	return out, warnings
}

// DeriveBuildMaps derives the uid and gid extents completely separately (host
// UID != GID). It is a pure derivation; BuildMaps gates it on NeedsBuildMaps.
func DeriveBuildMaps(uidRanges, gidRanges []Range) ([]string, []string, []string) {
	uid, uidWarnings := buildMapExtents(uidRanges)
	gid, gidWarnings := buildMapExtents(gidRanges)
	if len(uid) == 0 || len(gid) == 0 {
		return nil, nil, append(uidWarnings, gidWarnings...)
	}
	return uid, gid, append(uidWarnings, gidWarnings...)
}

// BuildMapArgs renders the derived extents as repeated --userns-*-map flags.
func BuildMapArgs(uid, gid []string) []string {
	out := make([]string, 0, 2*(len(uid)+len(gid)))
	for _, m := range uid {
		out = append(out, "--userns-uid-map", m)
	}
	for _, m := range gid {
		out = append(out, "--userns-gid-map", m)
	}
	return out
}

// BuildMaps reads L1's /etc/sub{u,g}id for the current user and derives the
// build extents. Read errors are returned as warnings with no injection: the
// file is optional inside L1 and podman's default userns still works.
func BuildMaps(env *Env) ([]string, []string, []string) {
	uidRanges, err := ParseSubIDFile(env.SubUIDFile, env.User)
	if err != nil {
		return nil, nil, []string{fmt.Sprintf("read %s for build maps: %v; leaving podman build's default userns", env.SubUIDFile, err)}
	}
	gidRanges, err := ParseSubIDFile(env.SubGIDFile, env.User)
	if err != nil {
		return nil, nil, []string{fmt.Sprintf("read %s for build maps: %v; leaving podman build's default userns", env.SubGIDFile, err)}
	}
	if len(uidRanges) == 0 || len(gidRanges) == 0 {
		return nil, nil, []string{fmt.Sprintf("no subordinate ID ranges for %s; leaving podman build's default userns", env.User)}
	}
	if !NeedsBuildMaps(uidRanges, gidRanges) {
		return nil, nil, nil
	}
	return DeriveBuildMaps(uidRanges, gidRanges)
}

// BuildReadiness reports the preflight detail for the build userns.
func BuildReadiness(env *Env, uidRanges, gidRanges []Range) (Status, string) {
	if len(uidRanges) == 0 || len(gidRanges) == 0 {
		return StatusWarn, "subordinate ID ranges unavailable"
	}
	if !NeedsBuildMaps(uidRanges, gidRanges) {
		return StatusPass, fmt.Sprintf("pool covers container %d; no build map injection needed", nobodyID)
	}
	if !env.InjectBuildMaps {
		return StatusWarn, fmt.Sprintf("sparse pool; container %d is unmapped and build map injection is disabled", nobodyID)
	}
	uid, gid, warnings := DeriveBuildMaps(uidRanges, gidRanges)
	if len(uid) == 0 || len(gid) == 0 {
		return StatusWarn, strings.Join(warnings, "; ")
	}
	detail := fmt.Sprintf("sparse pool; pinp-i build injects %s", strings.Join(BuildMapArgs(uid, gid), " "))
	if len(warnings) > 0 {
		detail += " (" + strings.Join(warnings, "; ") + ")"
	}
	return StatusPass, detail
}
