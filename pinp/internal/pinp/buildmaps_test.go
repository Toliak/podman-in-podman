package pinp

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestNeedsBuildMaps(t *testing.T) {
	cases := []struct {
		name string
		uid  []Range
		gid  []Range
		want bool
	}{
		{"baked sparse L1", []Range{{1, 1001}, {100000, 32768}}, []Range{{1, 1001}, {100000, 32768}}, true},
		{"wide contiguous pool", []Range{{231072, 65536}}, []Range{{231072, 65536}}, false},
		{"exactly nobody", []Range{{1, 65534}}, []Range{{1, 65534}}, false},
		{"one short of nobody", []Range{{1, 65533}}, []Range{{1, 65533}}, true},
		{"gid only sparse", []Range{{231072, 65536}}, []Range{{1, 1001}, {100000, 32768}}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := NeedsBuildMaps(c.uid, c.gid); got != c.want {
				t.Fatalf("NeedsBuildMaps = %v, want %v", got, c.want)
			}
		})
	}
}

func TestDeriveBuildMaps(t *testing.T) {
	cases := []struct {
		name             string
		uid, gid         []Range
		wantUID, wantGID []string
	}{
		{
			name:    "baked sparse L1 (file order is not sorted)",
			uid:     []Range{{100000, 32768}, {1, 1001}},
			gid:     []Range{{100000, 32768}, {1, 1001}},
			wantUID: []string{"0:1:1001", "1001:1002:32766", "65534:33768:2"},
			wantGID: []string{"0:1:1001", "1001:1002:32766", "65534:33768:2"},
		},
		{
			name:    "widened L1 derives uid and gid separately",
			uid:     []Range{{1, 1001}, {100000, 64531}, {65534, 2}},
			gid:     []Range{{1, 1005}, {100000, 64527}, {65534, 2}},
			wantUID: []string{"0:1:1001", "1001:1002:2", "1003:1004:64529", "65534:65533:2"},
			wantGID: []string{"0:1:1005", "1005:1006:2", "1007:1008:64525", "65534:65533:2"},
		},
		{
			name:    "single sparse range",
			uid:     []Range{{1, 32768}},
			gid:     []Range{{1, 32768}},
			wantUID: []string{"0:1:32766", "65534:32767:2"},
			wantGID: []string{"0:1:32766", "65534:32767:2"},
		},
		{
			name:    "wide pool still derives (BuildMaps gates it)",
			uid:     []Range{{231072, 65536}},
			gid:     []Range{{231072, 65536}},
			wantUID: []string{"0:1:65534", "65534:65535:2"},
			wantGID: []string{"0:1:65534", "65534:65535:2"},
		},
		{
			name:    "caps to five extents",
			uid:     []Range{{1, 100}, {1000, 100}, {2000, 100}, {3000, 100}, {4000, 100}, {5000, 100}},
			gid:     []Range{{1, 100}, {1000, 100}, {2000, 100}, {3000, 100}, {4000, 100}, {5000, 100}},
			wantUID: []string{"0:1:100", "100:101:100", "200:201:100", "300:301:98", "65534:399:2"},
			wantGID: []string{"0:1:100", "100:101:100", "200:201:100", "300:301:98", "65534:399:2"},
		},
		{
			name:    "drops a trailing one-ID range",
			uid:     []Range{{1, 1001}, {100000, 32768}, {200000, 1}},
			gid:     []Range{{1, 1001}, {100000, 32768}, {200000, 1}},
			wantUID: []string{"0:1:1001", "1001:1002:32766", "65534:33768:2"},
			wantGID: []string{"0:1:1001", "1001:1002:32766", "65534:33768:2"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			uid, gid, _ := DeriveBuildMaps(c.uid, c.gid)
			if !reflect.DeepEqual(uid, c.wantUID) {
				t.Fatalf("uid = %v, want %v", uid, c.wantUID)
			}
			if !reflect.DeepEqual(gid, c.wantGID) {
				t.Fatalf("gid = %v, want %v", gid, c.wantGID)
			}
			if len(uid) > maxBuildExtents || len(gid) > maxBuildExtents {
				t.Fatalf("more than %d extents: uid=%v gid=%v", maxBuildExtents, uid, gid)
			}
		})
	}
}

func TestBuildMapArgs(t *testing.T) {
	got := BuildMapArgs([]string{"0:1:1001", "65534:1002:2"}, []string{"0:1:1005", "65534:1006:2"})
	want := []string{
		"--userns-uid-map", "0:1:1001",
		"--userns-uid-map", "65534:1002:2",
		"--userns-gid-map", "0:1:1005",
		"--userns-gid-map", "65534:1006:2",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("BuildMapArgs = %v, want %v", got, want)
	}
	if len(BuildMapArgs(nil, nil)) != 0 {
		t.Fatal("nil maps must render no flags")
	}
}

func TestBuildConflicts(t *testing.T) {
	cases := []struct {
		args []string
		want []string
	}{
		{[]string{"-t", "x", "."}, nil},
		{[]string{"--userns=host", "-t", "x", "."}, []string{"--userns"}},
		{[]string{"--userns", "keep-id", "-t", "x", "."}, []string{"--userns"}},
		{[]string{"--userns-uid-map", "0:1:1001", "-t", "x", "."}, []string{"--userns-uid-map"}},
		{[]string{"--userns-gid-map-group=devuser", "--isolation=chroot"}, []string{"--userns-gid-map-group", "--isolation"}},
	}
	for _, c := range cases {
		got := BuildConflicts(c.args)
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("BuildConflicts(%v) = %v, want %v", c.args, got, c.want)
		}
	}
}

func TestBuildMaps(t *testing.T) {
	dir := t.TempDir()
	env := &Env{User: "tester"}
	env.SubUIDFile = filepath.Join(dir, "subuid")
	env.SubGIDFile = filepath.Join(dir, "subgid")
	content := "tester:100000:32768\ntester:1:1001\n"
	if err := os.WriteFile(env.SubUIDFile, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(env.SubGIDFile, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	uid, gid, warnings := BuildMaps(env)
	if len(warnings) > 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	want := []string{"0:1:1001", "1001:1002:32766", "65534:33768:2"}
	if !reflect.DeepEqual(uid, want) || !reflect.DeepEqual(gid, want) {
		t.Fatalf("BuildMaps = %v / %v, want %v", uid, gid, want)
	}
	os.Remove(env.SubGIDFile)
	if _, _, w := BuildMaps(env); len(w) == 0 || !strings.Contains(w[0], "leaving podman build's default userns") {
		t.Fatalf("missing warning on unreadable subgid: %v", w)
	}
	wide := "tester:231072:65536\n"
	if err := os.WriteFile(env.SubUIDFile, []byte(wide), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(env.SubGIDFile, []byte(wide), 0o644); err != nil {
		t.Fatal(err)
	}
	if uid, gid, w := BuildMaps(env); len(uid) != 0 || len(gid) != 0 || len(w) != 0 {
		t.Fatalf("wide pool = %v / %v / %v, want no injection", uid, gid, w)
	}
}

func TestBuildReadiness(t *testing.T) {
	env := &Env{InjectBuildMaps: true}
	status, detail := BuildReadiness(env, []Range{{1, 1001}, {100000, 32768}}, []Range{{1, 1001}, {100000, 32768}})
	if status != StatusPass || !strings.Contains(detail, "--userns-uid-map 0:1:1001") {
		t.Fatalf("sparse readiness = %s %q", status, detail)
	}
	status, detail = BuildReadiness(env, []Range{{231072, 65536}}, []Range{{231072, 65536}})
	if status != StatusPass || !strings.Contains(detail, "no build map injection needed") {
		t.Fatalf("wide readiness = %s %q", status, detail)
	}
	env.InjectBuildMaps = false
	status, _ = BuildReadiness(env, []Range{{1, 1001}}, []Range{{1, 1001}})
	if status != StatusWarn {
		t.Fatalf("disabled injection readiness = %s, want warn", status)
	}
}
