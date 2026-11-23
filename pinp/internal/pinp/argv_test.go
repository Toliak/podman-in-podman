package pinp

import (
	"path/filepath"
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
