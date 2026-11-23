package pinp

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
)

type Range struct {
	Start int
	Size  int
}

func ParseSubIDFile(path, userName string) ([]Range, error) {
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
		if len(parts) != 3 || strings.TrimSpace(parts[0]) != userName {
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

func OuterMaps(uid, gid, total int) ([]string, []string) {
	build := func(id int) []string {
		return []string{
			fmt.Sprintf("%d:0:1", id),
			fmt.Sprintf("0:1:%d", id),
			fmt.Sprintf("%d:%d:%d", ContainerRangeStart, id+1, total-id-3),
			fmt.Sprintf("%d:%d:2", total-2, total-2),
		}
	}
	return build(uid), build(gid)
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
