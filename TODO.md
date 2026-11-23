# TODO

- [ ] Test `:z` / `:Z` volume relabeling and `--security-opt label=nested` on an SELinux-enforcing VM (Fedora/RHEL) as alternatives to `--security-opt label=disable` for the podman-in-podman dev container.
  - Add `:z` to the two host bind mounts (`$PWD/../..:/opt/workdir` and `$HOME/.local/share/containers/storage:/var/lib/shared`) and confirm L1 still sees host images with `R/O=true` (`podman image ls`).
  - Use `label=nested` (instead of `label=disable`) and confirm the inner podman can create its nested user namespaces, mount fuse-overlayfs, and run `echo "hello world"` + `podman build` without AVC denials (check `ausearch` / journal).
  - Decide whether `label=disable` can be qualified as SELinux-only in `INFO.md`.
  - Context: `label=disable` is a confirmed no-op on this non-SELinux host (see investigation, 2026-11-21); it only matters for bind mounts / nested operations on SELinux hosts.
  - Findings (2026-11-22, `tests/selinux/RESULTS.md`): on a Fedora 45 SELinux-enforcing VM, `:z` / `:Z` + `label=nested` are **not** substitutes for `label=disable`. Only `label=disable` lets the inner podman run `echo "hello world"` and `podman build`; every alternative fails at `crun: mount devpts: Permission denied`, with zero AVCs even after `semodule -DB`. `label=disable` is required on SELinux hosts and a no-op elsewhere, so the "SELinux-only" qualification is supported.
