package pinp

import (
	"fmt"
	"os/exec"
)

// ResolveFuseOverlayFS resolves the fuse-overlayfs binary. An explicit override
// (PINP_FUSE_OVERLAYFS / [host] fuse_overlayfs) wins, then the effective
// storage.conf mount_program, then PATH via exec.LookPath. It deliberately does
// not hardcode /usr/bin so images that install fuse-overlayfs under
// /usr/local/bin keep working.
func ResolveFuseOverlayFS(env *Env, cfg *EffectiveConfig) (string, error) {
	if env != nil && env.FuseOverlayFS != "" {
		return env.FuseOverlayFS, nil
	}
	if cfg != nil {
		if mp := cfg.Storage.Storage.Options.Overlay.MountProgram; mp != "" {
			return mp, nil
		}
	}
	path, err := exec.LookPath("fuse-overlayfs")
	if err != nil {
		return "", fmt.Errorf("fuse-overlayfs not found on PATH; install it or set PINP_FUSE_OVERLAYFS: %w", err)
	}
	return path, nil
}
