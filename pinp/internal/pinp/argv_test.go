package pinp

import (
	"path/filepath"
	"reflect"
	"testing"
)

func TestSELinuxEnabled(t *testing.T) {
	dir := t.TempDir()
	missing := filepath.Join(dir, "missing")
	cases := []struct {
		name string
		env  Env
		want bool
	}{
		{"on", Env{SELinux: "on"}, true},
		{"off", Env{SELinux: "off", SELinuxFS: dir}, false},
		{"auto-present", Env{SELinux: "auto", SELinuxFS: dir}, true},
		{"auto-missing", Env{SELinux: "auto", SELinuxFS: missing}, false},
		{"default-auto-missing", Env{SELinuxFS: missing}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := SELinuxEnabled(&c.env); got != c.want {
				t.Fatalf("SELinuxEnabled = %v, want %v", got, c.want)
			}
		})
	}
}

func TestSecurityLabelConflict(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want bool
	}{
		{"none", []string{"run", "debian:latest"}, false},
		{"two-token-label", []string{"--security-opt", "label=disable", "debian:latest"}, true},
		{"equals-label", []string{"--security-opt=label=disable", "debian:latest"}, true},
		{"label-type", []string{"--security-opt", "label=type:container_t", "x"}, true},
		{"seccomp-two-token", []string{"--security-opt", "seccomp=/tmp/x", "x"}, false},
		{"seccomp-equals", []string{"--security-opt=seccomp=/tmp/x", "x"}, false},
		{"unrelated", []string{"--userns=host", "x"}, false},
		{"missing-value", []string{"--security-opt"}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := SecurityLabelConflict(c.args)
			if (got != "") != c.want {
				t.Fatalf("SecurityLabelConflict(%v) = %q, want conflict=%v", c.args, got, c.want)
			}
		})
	}
}

func TestOuterRunArgsLabelGating(t *testing.T) {
	dir := t.TempDir()
	base := Env{Store: "/store", StoreOpts: ":ro", SELinuxFS: dir}
	on := base
	on.SELinux = "on"
	off := base
	off.SELinux = "off"
	auto := base
	auto.SELinux = "auto"

	if got := OuterRunArgs(&on, nil, nil); !hasLabelDisable(got) {
		t.Fatalf("on: missing label=disable in %v", got)
	}
	if got := OuterRunArgs(&off, nil, nil); hasLabelDisable(got) {
		t.Fatalf("off: unexpected label=disable in %v", got)
	}
	if got := OuterRunArgs(&auto, nil, nil); !hasLabelDisable(got) {
		t.Fatalf("auto with present selinuxfs: missing label=disable in %v", got)
	}
}

func hasLabelDisable(args []string) bool {
	for i, a := range args {
		if a == "--security-opt" && i+1 < len(args) && args[i+1] == "label=disable" {
			return true
		}
	}
	return false
}

func TestConflicts(t *testing.T) {
	cases := []struct {
		args []string
		want int
	}{
		{[]string{"run", "img"}, 0},
		{[]string{"--userns=host", "img"}, 1},
		{[]string{"--uidmap", "0:0:1", "--gidmap=0:0:1", "img"}, 2},
		{[]string{"--device", "/dev/fuse", "img"}, 1},
		{[]string{"-v", "/x:/var/lib/shared", "img"}, 1},
		{[]string{"--security-opt", "label=disable", "img"}, 0},
	}
	for _, c := range cases {
		if got := len(Conflicts(c.args)); got != c.want {
			t.Errorf("len(Conflicts(%v)) = %d, want %d", c.args, got, c.want)
		}
	}
}

func TestFindUserns(t *testing.T) {
	cases := []struct {
		name      string
		args      []string
		value     string
		start     int
		end       int
		wantFound bool
	}{
		{"none", []string{"run", "img"}, "", 0, 0, false},
		{"equals", []string{"--userns=keep-id", "img"}, "keep-id", 0, 1, true},
		{"two-token", []string{"--userns", "host", "img"}, "host", 0, 2, true},
		{"empty", []string{"--userns"}, "", 0, 1, true},
		{"late", []string{"-it", "--userns=auto", "img"}, "auto", 1, 2, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			value, start, end, found := FindUserns(c.args)
			if found != c.wantFound || value != c.value || start != c.start || end != c.end {
				t.Fatalf("FindUserns(%v) = %q,%d,%d,%v; want %q,%d,%d,%v",
					c.args, value, start, end, found, c.value, c.start, c.end, c.wantFound)
			}
		})
	}
}

func TestStripUserns(t *testing.T) {
	cases := []struct {
		name       string
		args       []string
		start, end int
		want       []string
	}{
		{"equals", []string{"--userns=keep-id", "img"}, 0, 1, []string{"img"}},
		{"two-token", []string{"--userns", "keep-id", "img"}, 0, 2, []string{"img"}},
		{"middle", []string{"-it", "--userns=keep-id", "img", "bash"}, 1, 2, []string{"-it", "img", "bash"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := StripUserns(c.args, c.start, c.end)
			if len(got) != len(c.want) {
				t.Fatalf("StripUserns(%v) = %v, want %v", c.args, got, c.want)
			}
			for i := range got {
				if got[i] != c.want[i] {
					t.Fatalf("StripUserns(%v) = %v, want %v", c.args, got, c.want)
				}
			}
		})
	}
}

func TestParseKeepID(t *testing.T) {
	cases := []struct {
		name    string
		value   string
		want    KeepID
		wantErr bool
	}{
		{"empty", "", KeepID{}, true},
		{"bare keep-id", "keep-id", KeepID{UID: 1002, GID: 1006, Size: DefaultTotal}, false},
		{"uid+gid", "keep-id:uid=100,gid=100", KeepID{UID: 100, GID: 100, Size: DefaultTotal}, false},
		{"uid only", "keep-id:uid=42", KeepID{UID: 42, GID: 1006, Size: DefaultTotal}, false},
		{"gid only", "keep-id:gid=7", KeepID{UID: 1002, GID: 7, Size: DefaultTotal}, false},
		{"size", "keep-id:size=8192", KeepID{UID: 1002, GID: 1006, Size: 8192}, false},
		{"all", "keep-id:gid=9,size=4096,uid=8", KeepID{UID: 8, GID: 9, Size: 4096}, false},
		{"uid=0,gid=0", "keep-id:uid=0,gid=0", KeepID{UID: 0, GID: 0, Size: DefaultTotal}, false},
		{"host", "host", KeepID{}, true},
		{"auto", "auto", KeepID{}, true},
		{"nomap", "nomap", KeepID{}, true},
		{"ns", "ns:foo", KeepID{}, true},
		{"auto:size", "auto:size=8192", KeepID{}, true},
		{"unknown option", "keep-id:foo=1", KeepID{}, true},
		{"missing value", "keep-id:uid", KeepID{}, true},
		{"bad value", "keep-id:uid=x", KeepID{}, true},
		{"negative", "keep-id:uid=-1", KeepID{}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := ParseKeepID(c.value, 1002, 1006, DefaultTotal)
			if (err != nil) != c.wantErr {
				t.Fatalf("ParseKeepID(%q) = %v, %v; wantErr=%v", c.value, got, err, c.wantErr)
			}
			if !c.wantErr && !reflect.DeepEqual(got, c.want) {
				t.Fatalf("ParseKeepID(%q) = %v, want %v", c.value, got, c.want)
			}
		})
	}
}

func TestFindSubcommand(t *testing.T) {
	cases := []struct {
		args []string
		want int
	}{
		{[]string{"run", "img"}, 0},
		{[]string{"--log-level", "debug", "run", "img"}, 2},
		{[]string{"--log-level=debug", "run", "img"}, 1},
		{[]string{"--", "run"}, 1},
		{[]string{}, -1},
	}
	for _, c := range cases {
		got, err := FindSubcommand(c.args)
		if err != nil {
			t.Fatalf("FindSubcommand(%v): %v", c.args, err)
		}
		if got != c.want {
			t.Errorf("FindSubcommand(%v) = %d, want %d", c.args, got, c.want)
		}
	}
}
