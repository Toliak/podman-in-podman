package pinp

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseSubIDFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "subuid")
	content := "# comment\ndevuser:100000:32768\ndevuser:1:1001\nother:5:10\n\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	ranges, err := ParseSubIDFile(path, "devuser")
	if err != nil {
		t.Fatal(err)
	}
	if len(ranges) != 2 {
		t.Fatalf("got %d ranges, want 2: %v", len(ranges), ranges)
	}
	if ranges[0] != (Range{Start: 100000, Size: 32768}) || ranges[1] != (Range{Start: 1, Size: 1001}) {
		t.Fatalf("unexpected ranges: %v", ranges)
	}
	if total := TotalSize(ranges); total != 33769 {
		t.Fatalf("TotalSize = %d, want 33769", total)
	}
}

func TestParseSubIDFileBadRange(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "subuid")
	if err := os.WriteFile(path, []byte("devuser:notanint:10\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ParseSubIDFile(path, "devuser"); err == nil {
		t.Fatal("expected error for a malformed subordinate ID range")
	}
}

func TestOuterMaps(t *testing.T) {
	uidmaps, gidmaps := OuterMaps(1002, 1006, DefaultTotal)
	if len(uidmaps) != 4 || len(gidmaps) != 4 {
		t.Fatalf("expected 4 extents each, got uid=%v gid=%v", uidmaps, gidmaps)
	}
	if uidmaps[0] != "1002:0:1" || uidmaps[2] != "100000:1003:64531" {
		t.Fatalf("unexpected uidmaps: %v", uidmaps)
	}
	if gidmaps[0] != "1006:0:1" || gidmaps[2] != "100000:1007:64527" {
		t.Fatalf("unexpected gidmaps: %v", gidmaps)
	}
}
