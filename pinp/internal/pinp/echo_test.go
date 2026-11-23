package pinp

import "testing"

func TestShellQuote(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"", "''"},
		{"podman", "podman"},
		{"a-b_c.d/e:f,g+h=i@j%k", "a-b_c.d/e:f,g+h=i@j%k"},
		{"1234567890", "1234567890"},
		{"hello world", "'hello world'"},
		{"it's fine", `'it'\''s fine'`},
		{"$HOME", "'$HOME'"},
		{"a;b", "'a;b'"},
	}
	for _, c := range cases {
		if got := ShellQuote(c.in); got != c.want {
			t.Errorf("ShellQuote(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestEchoCommand(t *testing.T) {
	got := EchoCommand("/usr/bin/podman", []string{"run", "hello world", "debian:latest"})
	want := "podman run 'hello world' debian:latest"
	if got != want {
		t.Fatalf("EchoCommand = %q, want %q", got, want)
	}
}

func TestEchoCommandEscapesSingleQuote(t *testing.T) {
	got := EchoCommand("podman", []string{"ps", "it's"})
	want := `podman ps 'it'\''s'`
	if got != want {
		t.Fatalf("EchoCommand = %q, want %q", got, want)
	}
}
