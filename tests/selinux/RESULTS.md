# SELinux nested-podman experiment

Guest: `Fedora-Cloud-Base-Generic-45-1.4.x86_64.qcow2`, SELinux enforcing, direct kernel boot.
Topology: VM (SELinux host) -> outer `podman run` (L1, `pinp-outer`) -> inner podman (L2).

## Case matrix

| Case | label | suffix | outer rc | images rc | hello rc | build rc | AVC denials |
|---|---|---|---|---|---|---|---|
| A | disable | none | 0 | 0 | 0 | 0 | 0 |
| B | none | none | 0 | 0 | 126 | 125 | 0 |
| C | nested | none | 0 | 0 | 126 | 125 | 0 |
| D | nested | z | 0 | 0 | 126 | 1 | 0 |
| E | nested | Z | 0 | 0 | 126 | 1 | 0 |
| F | none | z | 0 | 0 | 126 | 1 | 0 |
| G | disable | z | 0 | 0 | 0 | 0 | 0 |
| H | disable | Z | 0 | 0 | 0 | 0 | 0 |

## Artifacts

Per-case evidence is in `tests/selinux/results/<case>/`: `run.stderr.txt`, `images.txt`, `inner-info.txt`, `hello.txt`, `build.txt`, `check.json`, `outer-labels.txt`, `labels.before.txt`, `labels.after.txt`, `avc.txt`, `audit2allow.txt`, `kernel.log.txt`, `sestatus.txt`.

## Findings (reference run: Fedora 45, podman 6.1.3, container-selinux 2.251.0)

- Case A (`label=disable`): L1 sees host images with `R/O=true`; inner `echo "hello world"` and inner `podman build` both succeed; no AVCs.
- Cases B-F (no label option / `label=nested`, with or without `:z`/`:Z`): the outer container still starts and host images are still visible `R/O=true`, but inner `podman run` and `podman build` fail with `crun: mount devpts to dev/pts: Permission denied`.
- `:z`/`:Z` do relabel the bind sources (`system_u:object_r:container_file_t`, see `labels.after.txt`) but do not rescue nested containers.
- Zero AVC/USER_AVC events in every case, even with `semodule -DB` (dontaudit disabled). The devpts failure is therefore not an SELinux policy denial; it correlates with L1 being confined (`container_t`) versus unconfined (`label=disable` yields an empty `ProcessLabel`, see `outer-labels.txt`). Likely crun applying a `context=` devpts mount option inside a nested userns.
- `label=nested` produced the same `container_t:s0:cX,cY` ProcessLabel as the default, so it did not relax confinement on this guest.
- Cases G/H: `label=disable` plus `:z`/`:Z` still works, so relabeling alone is harmless to the current PoC.

## TODO.md decision

`:z`/`:Z` + `label=nested` are **not** drop-in replacements for `--security-opt label=disable` on this guest. `label=disable` is required on SELinux-enforcing hosts and was already confirmed a no-op on the non-SELinux Arch host (`pinp-nolabel`), so the "SELinux-only" qualification is supported. Recommendation: keep `label=disable` in `pinp-e`; keep the TODO item open only if a different SELinux-preserving mechanism (e.g. a custom policy permitting nested `devpts` mounts) is worth investigating.

This harness records findings only; it does not edit `INFO.md` or `pinp-e`.
