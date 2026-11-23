package pinp

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const shellSafeChars = "@%_+=:,./-"

func shellSafe(r rune) bool {
	if r >= 'a' && r <= 'z' {
		return true
	}
	if r >= 'A' && r <= 'Z' {
		return true
	}
	if r >= '0' && r <= '9' {
		return true
	}
	return strings.ContainsRune(shellSafeChars, r)
}

func ShellQuote(s string) string {
	if s == "" {
		return "''"
	}
	safe := true
	for _, r := range s {
		if !shellSafe(r) {
			safe = false
			break
		}
	}
	if safe {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func EchoCommand(podman string, args []string) string {
	parts := make([]string, 0, len(args)+1)
	parts = append(parts, ShellQuote(filepath.Base(podman)))
	for _, a := range args {
		parts = append(parts, ShellQuote(a))
	}
	return strings.Join(parts, " ")
}

func Echo(podman string, args []string) {
	fmt.Fprintln(os.Stderr, EchoCommand(podman, args))
}
