package pinp

import "fmt"

// innerBulkCap is the highest container ID the contiguous bulk may own before
// the explicit container 65534..65535 (nobody) extent takes over.
const innerBulkCap = 65533

// InnerPoolSize returns T for a set of subordinate ranges: podman's rootless
// user namespace maps parent id 0 to the L1 user's own id and parent ids
// 1..T to the subordinate IDs (ranges sorted by start). Explicit inner
// --uidmap/--gidmap parent fields are offsets in that namespace, not absolute
// /etc/subuid start values.
func InnerPoolSize(ranges []Range) int {
	return TotalSize(ranges)
}

// innerExtents derives one dimension's inner map extents. pool is T (see
// InnerPoolSize); target is the container id the L1 user's own id lands on;
// size is the requested container namespace size (0 = default). The bulk maps
// containers 0..bulk-1 with the own id rotated to target, and the last two
// parent ids (pool-1, pool) always map container 65534..65535 so nobody works.
func innerExtents(pool, target, size int) ([]string, error) {
	if pool < 3 {
		return nil, fmt.Errorf("L1 subordinate ID pool too small (%d)", pool)
	}
	bulk := pool - 1
	if bulk > innerBulkCap {
		bulk = innerBulkCap
	}
	if size > 0 && size-1 < bulk {
		bulk = size - 1
	}
	if target < 0 || target >= bulk {
		return nil, fmt.Errorf("keep-id target %d is out of range for a pool of %d subordinate IDs", target, pool)
	}
	out := make([]string, 0, 4)
	if target > 0 {
		out = append(out, fmt.Sprintf("0:1:%d", target))
	}
	out = append(out, fmt.Sprintf("%d:0:1", target))
	if bulk-1 > target {
		out = append(out, fmt.Sprintf("%d:%d:%d", target+1, target+1, bulk-1-target))
	}
	out = append(out, fmt.Sprintf("%d:%d:2", nobodyID, pool-1))
	return out, nil
}

// DeriveInnerRunMaps derives the separate inner --uidmap and --gidmap extents
// for L2 from the L1 /etc/sub{u,g}id ranges and the resolved keep-id target.
// UID and GID are derived completely separately (host UID != GID). keep.Size,
// when <= 0, defaults to 1 + min(uidPool, gidPool).
func DeriveInnerRunMaps(uidRanges, gidRanges []Range, keep KeepID) ([]string, []string, []string, error) {
	uidPool, gidPool := InnerPoolSize(uidRanges), InnerPoolSize(gidRanges)
	if uidPool == 0 || gidPool == 0 {
		return nil, nil, []string{"no L1 subordinate ID ranges; leaving the static inner uid maps"}, nil
	}
	defaultSize := 1 + min(uidPool, gidPool)
	size := keep.Size
	if size <= 0 {
		size = defaultSize
	} else if size > defaultSize {
		return nil, nil, nil, fmt.Errorf("keep-id size=%d exceeds the L1 subordinate ID pool (uid=%d gid=%d)", size, uidPool, gidPool)
	}
	uidMaps, err := innerExtents(uidPool, keep.UID, size)
	if err != nil {
		return nil, nil, nil, err
	}
	gidMaps, err := innerExtents(gidPool, keep.GID, size)
	if err != nil {
		return nil, nil, nil, err
	}
	return uidMaps, gidMaps, nil, nil
}
