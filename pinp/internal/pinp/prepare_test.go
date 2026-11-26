package pinp

import (
	"os"
	"reflect"
	"testing"
)

const (
	capturedUIDMap = "0 1 1002\n1002 0 1\n65534 65534 2\n100000 1003 64531\n"
	capturedGIDMap = "0 1 1006\n1006 0 1\n65534 65534 2\n100000 1007 64527\n"
	u0UIDMap       = "0 0 1\n1 1 1002\n65534 65534 2\n100000 1003 64531\n"
	u0GIDMap       = "0 0 1\n1 1 1006\n65534 65534 2\n100000 1007 64527\n"
	u100UIDMap     = "0 1 100\n100 0 1\n101 101 902\n65534 65534 2\n100000 1003 64531\n"
	u100GIDMap     = "0 1 100\n100 0 1\n101 101 906\n65534 65534 2\n100000 1007 64527\n"
	hostIdentity   = "0 0 4294967295\n"
)

func TestParseIDMap(t *testing.T) {
	got, err := ParseIDMap([]byte(capturedUIDMap))
	if err != nil {
		t.Fatal(err)
	}
	want := []IDMapExtent{
		{Inside: 0, Parent: 1, Length: 1002},
		{Inside: 1002, Parent: 0, Length: 1},
		{Inside: 65534, Parent: 65534, Length: 2},
		{Inside: 100000, Parent: 1003, Length: 64531},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ParseIDMap = %v, want %v", got, want)
	}
	if bad, err := ParseIDMap([]byte("0 1\n")); err == nil {
		t.Fatalf("expected an error for a short line, got %v", bad)
	}
	if bad, err := ParseIDMap([]byte("x 1 2\n")); err == nil {
		t.Fatalf("expected an error for a non-numeric line, got %v", bad)
	}
}

func TestIsHostIdentityMap(t *testing.T) {
	if !IsHostIdentityMap(mustParse(t, hostIdentity)) {
		t.Fatal("single host identity extent must be refused")
	}
	if !IsHostIdentityMap(mustParse(t, "0 0 5\n1 1 5\n")) {
		t.Fatal("wide 0 0 identity extent must be refused")
	}
	if IsHostIdentityMap(mustParse(t, capturedUIDMap)) {
		t.Fatal("nested L1 map must not be refused")
	}
	if IsHostIdentityMap(mustParse(t, u0UIDMap)) {
		t.Fatal("keep-id:uid=0 own-id extent (0 0 1) plus other extents must not be refused")
	}
}

func TestDeriveSubIDRanges(t *testing.T) {
	cases := []struct {
		name             string
		uidMap, gidMap   string
		wantUID, wantGID []Range
	}{
		{
			name:    "legacy keep-id",
			uidMap:  capturedUIDMap,
			gidMap:  capturedGIDMap,
			wantUID: []Range{{Start: 1, Size: 1001}, {Start: 100000, Size: 64531}},
			wantGID: []Range{{Start: 1, Size: 1005}, {Start: 100000, Size: 64527}},
		},
		{
			name:    "default U=0",
			uidMap:  u0UIDMap,
			gidMap:  u0GIDMap,
			wantUID: []Range{{Start: 100000, Size: 64531}},
			wantGID: []Range{{Start: 100000, Size: 64527}},
		},
		{
			name:    "keep-id:uid=100,gid=100",
			uidMap:  u100UIDMap,
			gidMap:  u100GIDMap,
			wantUID: []Range{{Start: 1, Size: 99}, {Start: 100000, Size: 64531}},
			wantGID: []Range{{Start: 1, Size: 99}, {Start: 100000, Size: 64527}},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			uidRanges, _ := DeriveSubIDRanges(mustParse(t, c.uidMap), 0)
			if !reflect.DeepEqual(uidRanges, c.wantUID) {
				t.Fatalf("derived subuid = %v, want %v", uidRanges, c.wantUID)
			}
			gidRanges, _ := DeriveSubIDRanges(mustParse(t, c.gidMap), 0)
			if !reflect.DeepEqual(gidRanges, c.wantGID) {
				t.Fatalf("derived subgid = %v, want %v", gidRanges, c.wantGID)
			}
			if reflect.DeepEqual(uidRanges, gidRanges) {
				t.Fatal("uid and gid ranges must stay separate (host UID != GID)")
			}
		})
	}
}

func TestDeriveSubIDRangesMaxSize(t *testing.T) {
	ranges, _ := DeriveSubIDRanges(mustParse(t, capturedUIDMap), 32768)
	want := []Range{{Start: 1, Size: 1001}, {Start: 100000, Size: 32768}}
	if !reflect.DeepEqual(ranges, want) {
		t.Fatalf("--max-size 32768 = %v, want %v", ranges, want)
	}
}

func TestDeriveSubIDRangesCapsToFive(t *testing.T) {
	var extents []IDMapExtent
	for i := 1; i <= 8; i++ {
		extents = append(extents, IDMapExtent{Inside: i * 10, Parent: i*10 + 1, Length: 5})
	}
	ranges, warnings := DeriveSubIDRanges(extents, 0)
	if len(ranges) != maxSubIDRanges {
		t.Fatalf("got %d ranges, want <= %d: %v", len(ranges), maxSubIDRanges, ranges)
	}
	if len(warnings) == 0 {
		t.Fatal("expected a warning when collapsing to 5 extents")
	}
}

func TestHostUserInside(t *testing.T) {
	if id, ok := hostUserInside(mustParse(t, capturedUIDMap)); !ok || id != 1002 {
		t.Fatalf("uid host-user inside = %d,%v, want 1002,true", id, ok)
	}
	if id, ok := hostUserInside(mustParse(t, capturedGIDMap)); !ok || id != 1006 {
		t.Fatalf("gid host-user inside = %d,%v, want 1006,true", id, ok)
	}
	if id, ok := hostUserInside(mustParse(t, u0UIDMap)); !ok || id != 0 {
		t.Fatalf("uid=0 host-user inside = %d,%v, want 0,true", id, ok)
	}
	if _, ok := hostUserInside(mustParse(t, hostIdentity)); ok {
		t.Fatal("wide 0 0 identity extent must not be a host-user extent")
	}
	if _, ok := hostUserInside(nil); ok {
		t.Fatal("empty map must not yield a host-user extent")
	}
}

func TestResolveTarget(t *testing.T) {
	orig := lookupUserByID
	defer func() { lookupUserByID = orig }()
	lookupUserByID = func(uid int) (string, bool) {
		if uid == 1002 {
			return "devuser", true
		}
		return "", false
	}
	uidExtents := mustParse(t, capturedUIDMap)
	gidExtents := mustParse(t, capturedGIDMap)

	derived := &Env{User: "root", UID: 0, GID: 0}
	target, warnings := resolveTarget(derived, uidExtents, gidExtents, "")
	if target.User != "devuser" || target.UID != 1002 || target.GID != 1006 {
		t.Fatalf("derived target = %+v, want {devuser 1002 1006}", target)
	}
	if len(warnings) != 0 {
		t.Fatalf("derived target warnings = %v, want none", warnings)
	}

	if target, _ := resolveTarget(derived, uidExtents, gidExtents, "override"); target.User != "override" {
		t.Fatalf("--user override = %q, want override", target.User)
	}
	explicit := &Env{User: "tester", UID: 1002, GID: 1006, UserSet: true}
	if target, _ := resolveTarget(explicit, uidExtents, gidExtents, ""); target.User != "tester" {
		t.Fatalf("PINP_USER override = %q, want tester", target.User)
	}

	lookupUserByID = func(int) (string, bool) { return "", false }
	target, warnings = resolveTarget(derived, uidExtents, gidExtents, "")
	if target.User != "1002" {
		t.Fatalf("numeric fallback owner = %q, want 1002", target.User)
	}
	if len(warnings) == 0 {
		t.Fatal("expected a warning for the numeric fallback")
	}
}

func TestSubIDOwnerIsRoot(t *testing.T) {
	cases := []struct {
		user string
		want bool
	}{
		{"root", true},
		{"0", true},
		{"devuser", false},
		{"1002", false},
	}
	for _, c := range cases {
		if got := subidOwnerIsRoot(PrepareTarget{User: c.user}); got != c.want {
			t.Errorf("subidOwnerIsRoot(%q) = %v, want %v", c.user, got, c.want)
		}
	}
}

func TestSubIDFileContent(t *testing.T) {
	got := SubIDFileContent("devuser", []Range{{Start: 1, Size: 1001}, {Start: 100000, Size: 64531}})
	want := "devuser:1:1001\ndevuser:100000:64531\n"
	if got != want {
		t.Fatalf("SubIDFileContent = %q, want %q", got, want)
	}
}

func TestCapCommands(t *testing.T) {
	env := &Env{NewUIDMap: "/usr/bin/newuidmap", NewGIDMap: "/usr/bin/newgidmap", Setcap: "setcap"}
	got := CapCommands(env, true, true)
	want := []string{
		"chmod -s /usr/bin/newuidmap /usr/bin/newgidmap",
		"setcap cap_setuid+ep /usr/bin/newuidmap",
		"setcap cap_setgid+ep /usr/bin/newgidmap",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("CapCommands = %v, want %v", got, want)
	}
	if got := CapCommands(env, true, false); len(got) != 2 || got[1] != "setcap cap_setuid+ep /usr/bin/newuidmap" {
		t.Fatalf("uid-only caps = %v", got)
	}
}

func TestSelectPair(t *testing.T) {
	cases := []struct {
		changed  bool
		uid, gid bool
		x        string
		wantU    bool
		wantG    bool
		wantErr  bool
	}{
		{false, false, false, "", true, true, false},
		{true, true, false, "", true, false, false},
		{true, false, false, "u", true, false, false},
		{true, false, false, "g", false, true, false},
		{true, false, false, "ug", true, true, false},
		{true, false, false, "bogus", false, false, true},
	}
	for _, c := range cases {
		u, g, err := selectPair(c.changed, c.uid, c.gid, c.x)
		if (err != nil) != c.wantErr || u != c.wantU || g != c.wantG {
			t.Errorf("selectPair(%v,%v,%v,%q) = %v,%v,%v", c.changed, c.uid, c.gid, c.x, u, g, err)
		}
	}
}

func mustParse(t *testing.T, s string) []IDMapExtent {
	t.Helper()
	extents, err := ParseIDMap([]byte(s))
	if err != nil {
		t.Fatal(err)
	}
	return extents
}

func TestIsMountPoint(t *testing.T) {
	dir := t.TempDir()
	mountinfo := dir + "/mountinfo"
	content := "36 25 0:32 / /var/lib/shared rw,relatime - ext4 /dev/sda1 rw\n" +
		"37 25 0:33 / /tmp/with\\040space rw - ext4 /dev/sda2 rw\n"
	if err := os.WriteFile(mountinfo, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PINP_MOUNTINFO", mountinfo)
	for path, want := range map[string]bool{
		"/var/lib/shared":     true,
		"/var/lib/shared/":    true,
		"/tmp/with space":     true,
		"/var/lib/not-shared": false,
	} {
		got, err := IsMountPoint(path)
		if err != nil {
			t.Fatalf("IsMountPoint(%q): %v", path, err)
		}
		if got != want {
			t.Errorf("IsMountPoint(%q) = %v, want %v", path, got, want)
		}
	}
}

func TestIsMountPointMissingFile(t *testing.T) {
	t.Setenv("PINP_MOUNTINFO", "/nonexistent/mountinfo")
	if _, err := IsMountPoint("/var/lib/shared"); err == nil {
		t.Fatal("expected error for missing mountinfo")
	}
}

func TestDeriveAdditionalStores(t *testing.T) {
	cases := []struct {
		existing []string
		shared   string
		want     []string
	}{
		{nil, "/var/lib/shared", []string{"/var/lib/shared"}},
		{[]string{"/var/lib/shared"}, "/var/lib/shared", []string{"/var/lib/shared"}},
		{[]string{"/other", "/var/lib/shared"}, "/var/lib/shared", []string{"/other", "/var/lib/shared"}},
		{[]string{"/other", "/other", ""}, "/var/lib/shared", []string{"/other", "/var/lib/shared"}},
	}
	for _, c := range cases {
		if got := DeriveAdditionalStores(c.existing, c.shared); !reflect.DeepEqual(got, c.want) {
			t.Errorf("DeriveAdditionalStores(%v, %q) = %v, want %v", c.existing, c.shared, got, c.want)
		}
	}
}

func TestStorageConfContent(t *testing.T) {
	got := StorageConfContent([]string{"/other", "/var/lib/shared"})
	want := "[storage.options]\nadditionalimagestores = [\n    \"/other\",\n    \"/var/lib/shared\",\n]\n"
	if got != want {
		t.Fatalf("StorageConfContent = %q, want %q", got, want)
	}
}

func TestStoragePaths(t *testing.T) {
	env := &Env{Home: "/home/fallback"}
	target := PrepareTarget{Home: "/home/devuser"}
	conf := storageConfPath(env, target)
	if conf != "/home/devuser/.config/containers/storage.conf" {
		t.Fatalf("storageConfPath = %q", conf)
	}
	if drop := storageDropInPath(conf); drop != "/home/devuser/.config/containers/storage.conf.d/pinp-shared-store.conf" {
		t.Fatalf("storageDropInPath = %q", drop)
	}
	env.StorageConf = "/custom/storage.conf"
	if got := storageConfPath(env, target); got != "/custom/storage.conf" {
		t.Fatalf("override storageConfPath = %q", got)
	}
	target.Home = ""
	if got := storageConfPath(&Env{Home: "/home/fallback"}, target); got != "/home/fallback/.config/containers/storage.conf" {
		t.Fatalf("fallback storageConfPath = %q", got)
	}
}

func TestWriteStorageDropInIdempotent(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/storage.conf.d/pinp-shared-store.conf"
	content := StorageConfContent([]string{"/var/lib/shared"})
	target := PrepareTarget{UID: os.Getuid(), GID: os.Getgid()}
	if w := writeStorageDropIn(path, content, target); w != "" {
		t.Fatalf("writeStorageDropIn warn = %q", w)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != content {
		t.Fatalf("content = %q, want %q", got, content)
	}
	if w := writeStorageDropIn(path, content, target); w != "" {
		t.Fatalf("idempotent write warn = %q", w)
	}
}
