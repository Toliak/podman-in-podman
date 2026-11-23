package pinp

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
)

func ResolvePodman(name string) (string, error) {
	path, err := exec.LookPath(name)
	if err != nil {
		return "", fmt.Errorf("resolve podman %q: %w", name, err)
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	if self, err := os.Executable(); err == nil {
		if selfAbs, err := filepath.Abs(self); err == nil && selfAbs == abs {
			return "", fmt.Errorf("PINP_PODMAN resolves to the wrapper itself: %s", abs)
		}
	}
	return abs, nil
}

func Explain(podman string, args []string) error {
	full := append([]string{filepath.Base(podman)}, args...)
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(full)
}

func Exec(podman string, args []string, echo bool) error {
	path, err := ResolvePodman(podman)
	if err != nil {
		return err
	}
	if echo {
		Echo(podman, args)
	}
	full := append([]string{filepath.Base(path)}, args...)
	return syscall.Exec(path, full, os.Environ())
}
