package pinp

import (
	"reflect"
	"testing"
)

func TestDeriveInnerRunMapsSparse(t *testing.T) {
	sparse := []Range{{Start: 1, Size: 1001}, {Start: 100000, Size: 32768}}
	cases := []struct {
		name             string
		keep             KeepID
		wantUID, wantGID []string
	}{
		{
			name:    "default 0/0",
			keep:    KeepID{UID: 0, GID: 0},
			wantUID: []string{"0:0:1", "1:1:33767", "65534:33768:2"},
			wantGID: []string{"0:0:1", "1:1:33767", "65534:33768:2"},
		},
		{
			name:    "target 100/100",
			keep:    KeepID{UID: 100, GID: 100},
			wantUID: []string{"0:1:100", "100:0:1", "101:101:33667", "65534:33768:2"},
			wantGID: []string{"0:1:100", "100:0:1", "101:101:33667", "65534:33768:2"},
		},
		{
			name:    "bare 1002/1006",
			keep:    KeepID{UID: 1002, GID: 1006},
			wantUID: []string{"0:1:1002", "1002:0:1", "1003:1003:32765", "65534:33768:2"},
			wantGID: []string{"0:1:1006", "1006:0:1", "1007:1007:32761", "65534:33768:2"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			uid, gid, warnings, err := DeriveInnerRunMaps(sparse, sparse, tc.keep)
			if err != nil {
				t.Fatalf("DeriveInnerRunMaps: %v", err)
			}
			if len(warnings) != 0 {
				t.Fatalf("unexpected warnings: %v", warnings)
			}
			if !reflect.DeepEqual(uid, tc.wantUID) {
				t.Fatalf("uid maps = %v, want %v", uid, tc.wantUID)
			}
			if !reflect.DeepEqual(gid, tc.wantGID) {
				t.Fatalf("gid maps = %v, want %v", gid, tc.wantGID)
			}
		})
	}
}

func TestDeriveInnerRunMapsWidePool(t *testing.T) {
	wide := []Range{{Start: 231072, Size: 65536}}
	uid, gid, _, err := DeriveInnerRunMaps(wide, wide, KeepID{UID: 1002, GID: 1006})
	if err != nil {
		t.Fatalf("DeriveInnerRunMaps: %v", err)
	}
	wantUID := []string{"0:1:1002", "1002:0:1", "1003:1003:64530", "65534:65535:2"}
	wantGID := []string{"0:1:1006", "1006:0:1", "1007:1007:64526", "65534:65535:2"}
	if !reflect.DeepEqual(uid, wantUID) {
		t.Fatalf("uid maps = %v, want %v", uid, wantUID)
	}
	if !reflect.DeepEqual(gid, wantGID) {
		t.Fatalf("gid maps = %v, want %v", gid, wantGID)
	}
}

func TestDeriveInnerRunMapsSizeCap(t *testing.T) {
	sparse := []Range{{Start: 1, Size: 1001}, {Start: 100000, Size: 32768}}
	uid, _, _, err := DeriveInnerRunMaps(sparse, sparse, KeepID{UID: 0, GID: 0, Size: 8192})
	if err != nil {
		t.Fatalf("DeriveInnerRunMaps: %v", err)
	}
	want := []string{"0:0:1", "1:1:8190", "65534:33768:2"}
	if !reflect.DeepEqual(uid, want) {
		t.Fatalf("uid maps = %v, want %v", uid, want)
	}
}

func TestDeriveInnerRunMapsErrors(t *testing.T) {
	sparse := []Range{{Start: 1, Size: 1001}, {Start: 100000, Size: 32768}}
	if _, _, _, err := DeriveInnerRunMaps(sparse, sparse, KeepID{UID: 90000}); err == nil {
		t.Fatal("expected error for out-of-range target")
	}
	if _, _, _, err := DeriveInnerRunMaps(sparse, sparse, KeepID{Size: 70000}); err == nil {
		t.Fatal("expected error for size above the pool")
	}
	if _, _, warnings, err := DeriveInnerRunMaps(nil, nil, KeepID{}); err != nil || len(warnings) == 0 {
		t.Fatalf("expected no-error warning fallback for empty pool, got warnings=%v err=%v", warnings, err)
	}
}
