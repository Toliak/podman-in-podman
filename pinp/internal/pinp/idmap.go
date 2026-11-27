package pinp

import (
	"bufio"
	"fmt"
	"os"
	"os/user"
	"strconv"
	"strings"
)

type Range struct {
	Start int
	Size  int
}

func ParseSubIDFile(path, userName string) ([]Range, error) {
	// Match the numeric uid too, mirroring containers/storage idtools. This lets
	// a numeric line written before useradd (or a numeric owner) validate.
	uidStr := ""
	if u, err := user.Lookup(userName); err == nil {
		uidStr = u.Uid
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()
	var ranges []Range
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.Split(line, ":")
		if len(parts) != 3 {
			continue
		}
		name := strings.TrimSpace(parts[0])
		if name != userName && (uidStr == "" || name != uidStr) {
			continue
		}
		start, err := strconv.Atoi(strings.TrimSpace(parts[1]))
		if err != nil {
			return nil, fmt.Errorf("bad subordinate ID start in %s: %w", path, err)
		}
		size, err := strconv.Atoi(strings.TrimSpace(parts[2]))
		if err != nil {
			return nil, fmt.Errorf("bad subordinate ID size in %s: %w", path, err)
		}
		ranges = append(ranges, Range{Start: start, Size: size})
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return ranges, nil
}

func TotalSize(ranges []Range) int {
	total := 0
	for _, r := range ranges {
		total += r.Size
	}
	return total
}

// OuterMaps builds the outer --uidmap/--gidmap extents. uid/gid are the host
// user's real IDs (they size the low range and the anchor); u/g are the target
// container IDs where the host user lands (DefaultKeepID gives 0/0). The low
// range always covers container IDs 0..max(id,u) so the L1 image's USER (id)
// stays mapped, and the 100000 anchor is preserved. At most 5 extents, which
// is the newuidmap ceiling.
func OuterMaps(uid, gid, u, g, total int) ([]string, []string) {
	build := func(id, target int) []string {
		low := id
		if target > low {
			low = target
		}
		out := []string{fmt.Sprintf("%d:0:1", target)}
		if target > 0 {
			out = append(out, fmt.Sprintf("0:1:%d", target))
		}
		if low > target {
			out = append(out, fmt.Sprintf("%d:%d:%d", target+1, target+1, low-target))
		}
		out = append(out,
			fmt.Sprintf("%d:%d:%d", ContainerRangeStart, low+1, total-low-3),
			fmt.Sprintf("%d:%d:2", total-2, total-2),
		)
		return out
	}
	return build(uid, u), build(gid, g)
}

// ValidateKeepID bounds a resolved keep-id target against the host's
// subordinate-ID pool and the extent arithmetic in OuterMaps.
func ValidateKeepID(k KeepID, uid, gid, pool int) error {
	low := uid
	if gid > low {
		low = gid
	}
	if k.UID > low {
		low = k.UID
	}
	if k.GID > low {
		low = k.GID
	}
	if k.UID < 0 || k.GID < 0 || k.UID >= k.Size-2 || k.GID >= k.Size-2 {
		return fmt.Errorf("keep-id uid=%d gid=%d out of range for size=%d", k.UID, k.GID, k.Size)
	}
	if k.Size > pool {
		return fmt.Errorf("keep-id size=%d exceeds the host subordinate ID pool (%d)", k.Size, pool)
	}
	if k.Size <= low+3 {
		return fmt.Errorf("keep-id size=%d is too small (need > %d)", k.Size, low+3)
	}
	return nil
}

func ValidateHostRanges(uidRanges, gidRanges []Range) error {
	uidTotal, gidTotal := TotalSize(uidRanges), TotalSize(gidRanges)
	if uidTotal < DefaultTotal || gidTotal < DefaultTotal {
		return fmt.Errorf("host subordinate ID ranges too small: uid=%d gid=%d (need >= %d each)", uidTotal, gidTotal, DefaultTotal)
	}
	if len(uidRanges) > 1 || len(gidRanges) > 1 {
		fmt.Fprintln(os.Stderr, "pinp-e: warning: sparse host /etc/subuid or /etc/subgid ranges; the outer map may cross parent userns extent boundaries")
	}
	return nil
}
