package pinp

import (
	"os"
	"path/filepath"
	"reflect"
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
	cases := []struct {
		name             string
		uid, gid         int
		u, g             int
		total            int
		wantUID, wantGID []string
	}{
		{
			name:    "default U=0,G=0",
			uid:     1002,
			gid:     1006,
			u:       0,
			g:       0,
			total:   DefaultTotal,
			wantUID: []string{"0:0:1", "1:1:1002", "100000:1003:64531", "65534:65534:2"},
			wantGID: []string{"0:0:1", "1:1:1006", "100000:1007:64527", "65534:65534:2"},
		},
		{
			name:    "bare keep-id U=$UID,G=$GID (legacy)",
			uid:     1002,
			gid:     1006,
			u:       1002,
			g:       1006,
			total:   DefaultTotal,
			wantUID: []string{"1002:0:1", "0:1:1002", "100000:1003:64531", "65534:65534:2"},
			wantGID: []string{"1006:0:1", "0:1:1006", "100000:1007:64527", "65534:65534:2"},
		},
		{
			name:    "keep-id:uid=100,gid=100",
			uid:     1002,
			gid:     1006,
			u:       100,
			g:       100,
			total:   DefaultTotal,
			wantUID: []string{"100:0:1", "0:1:100", "101:101:902", "100000:1003:64531", "65534:65534:2"},
			wantGID: []string{"100:0:1", "0:1:100", "101:101:906", "100000:1007:64527", "65534:65534:2"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			uidmaps, gidmaps := OuterMaps(c.uid, c.gid, c.u, c.g, c.total)
			if !reflect.DeepEqual(uidmaps, c.wantUID) {
				t.Fatalf("uidmaps = %v, want %v", uidmaps, c.wantUID)
			}
			if !reflect.DeepEqual(gidmaps, c.wantGID) {
				t.Fatalf("gidmaps = %v, want %v", gidmaps, c.wantGID)
			}
			if len(uidmaps) > 5 || len(gidmaps) > 5 {
				t.Fatalf("more than 5 extents: uid=%v gid=%v", uidmaps, gidmaps)
			}
		})
	}
}

func TestValidateKeepID(t *testing.T) {
	cases := []struct {
		name    string
		k       KeepID
		wantErr bool
	}{
		{"default", DefaultKeepID(), false},
		{"explicit target", KeepID{UID: 100, GID: 100, Size: DefaultTotal}, false},
		{"size equal to pool", KeepID{UID: 0, GID: 0, Size: 65536}, false},
		{"size above pool", KeepID{UID: 0, GID: 0, Size: 70000}, true},
		{"size too small", KeepID{UID: 0, GID: 0, Size: 1000}, true},
		{"uid out of range", KeepID{UID: 90000, GID: 0, Size: DefaultTotal}, true},
		{"gid out of range", KeepID{UID: 0, GID: 90000, Size: DefaultTotal}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := ValidateKeepID(c.k, 1002, 1006, 65536)
			if (err != nil) != c.wantErr {
				t.Fatalf("ValidateKeepID(%v) = %v, wantErr=%v", c.k, err, c.wantErr)
			}
		})
	}
}
