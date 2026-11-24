package pinp

import "testing"

func TestPinpCommand(t *testing.T) {
	cases := []struct {
		sub  string
		want string
	}{
		{"pinp-check", CommandCheck},
		{"pinp-prepare", CommandPrepare},
		{"pinp-gen-sub", CommandPrepare},
		{"prepare", CommandPrepare},
		{"gen-sub", CommandPrepare},
		{"check", CommandNone},
		{"run", CommandNone},
		{"create", CommandNone},
		{"build", CommandNone},
		{"", CommandNone},
	}
	for _, c := range cases {
		if got := PinpCommand(c.sub); got != c.want {
			t.Errorf("PinpCommand(%q) = %q, want %q", c.sub, got, c.want)
		}
	}
}

func TestIsReservedPinpName(t *testing.T) {
	for _, sub := range []string{"pinp-check", "pinp-prepare", "pinp-frobnicate", "pinp-"} {
		if !IsReservedPinpName(sub) {
			t.Errorf("IsReservedPinpName(%q) = false, want true", sub)
		}
	}
	for _, sub := range []string{"check", "prepare", "run", "", "podman-check"} {
		if IsReservedPinpName(sub) {
			t.Errorf("IsReservedPinpName(%q) = true, want false", sub)
		}
	}
}

func TestKnownPinpCommandsAreDispatched(t *testing.T) {
	for _, name := range KnownPinpCommands() {
		if !IsReservedPinpName(name) {
			t.Errorf("known command %q does not use the reserved prefix", name)
		}
		if PinpCommand(name) == CommandNone {
			t.Errorf("known command %q is not in the dispatch table", name)
		}
	}
}
