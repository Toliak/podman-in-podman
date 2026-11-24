package pinp

import (
	"reflect"
	"testing"
)

const (
	capturedUIDMap = "0 1 1002\n1002 0 1\n65534 65534 2\n100000 1003 64531\n"
	capturedGIDMap = "0 1 1006\n1006 0 1\n65534 65534 2\n100000 1007 64527\n"
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
		t.Fatal("all-identity map must be refused")
	}
	if IsHostIdentityMap(mustParse(t, capturedUIDMap)) {
		t.Fatal("nested L1 map must not be refused")
	}
}

func TestDeriveSubIDRanges(t *testing.T) {
	uidRanges, _ := DeriveSubIDRanges(mustParse(t, capturedUIDMap), 1002, 0)
	wantUID := []Range{{Start: 1, Size: 1001}, {Start: 100000, Size: 64531}}
	if !reflect.DeepEqual(uidRanges, wantUID) {
		t.Fatalf("derived subuid = %v, want %v", uidRanges, wantUID)
	}
	gidRanges, _ := DeriveSubIDRanges(mustParse(t, capturedGIDMap), 1006, 0)
	wantGID := []Range{{Start: 1, Size: 1005}, {Start: 100000, Size: 64527}}
	if !reflect.DeepEqual(gidRanges, wantGID) {
		t.Fatalf("derived subgid = %v, want %v", gidRanges, wantGID)
	}
	if reflect.DeepEqual(uidRanges, gidRanges) {
		t.Fatal("uid and gid ranges must stay separate (host UID != GID)")
	}
}

func TestDeriveSubIDRangesMaxSize(t *testing.T) {
	ranges, _ := DeriveSubIDRanges(mustParse(t, capturedUIDMap), 1002, 32768)
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
	ranges, warnings := DeriveSubIDRanges(extents, 1002, 0)
	if len(ranges) != maxSubIDRanges {
		t.Fatalf("got %d ranges, want <= %d: %v", len(ranges), maxSubIDRanges, ranges)
	}
	if len(warnings) == 0 {
		t.Fatal("expected a warning when collapsing to 5 extents")
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
