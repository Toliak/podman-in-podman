package pinp

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveFuseOverlayFS(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "fuse-overlayfs")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)

	got, err := ResolveFuseOverlayFS(&Env{}, nil)
	if err != nil || got != bin {
		t.Fatalf("PATH resolution = %q, %v; want %q", got, err, bin)
	}

	got, err = ResolveFuseOverlayFS(&Env{FuseOverlayFS: "/custom/fuse-overlayfs"}, nil)
	if err != nil || got != "/custom/fuse-overlayfs" {
		t.Fatalf("explicit resolution = %q, %v", got, err)
	}

	cfg := &EffectiveConfig{}
	cfg.Storage.Storage.Options.Overlay.MountProgram = "/local/fuse-overlayfs"
	got, err = ResolveFuseOverlayFS(&Env{}, cfg)
	if err != nil || got != "/local/fuse-overlayfs" {
		t.Fatalf("config resolution = %q, %v", got, err)
	}

	t.Setenv("PATH", t.TempDir())
	if _, err := ResolveFuseOverlayFS(&Env{}, nil); err == nil {
		t.Fatal("expected an error when fuse-overlayfs is not on PATH")
	}
}
