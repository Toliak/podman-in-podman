# SELinux nested-podman harness

Opt-in black-box experiment for `TODO.md`: does `:z`/`:Z` + `--security-opt label=nested` replace `--security-opt label=disable` on an SELinux-enforcing host?

This is **not** part of `make all`. It downloads ~607 MB, boots a QEMU VM, and needs network egress.

## Why a VM

SELinux is a kernel-wide LSM and is not namespaced. The dev host's LSM list is `capability,landlock,lockdown,yama,bpf` (SELinux compiled but not enabled), so no container on it can enforce a policy. The only isolated way is a separate kernel.

## No ISO, no installation

The guest is `Fedora-Cloud-Base-Generic-45-1.4.x86_64.qcow2` — a **pre-installed disk image**, not an installer ISO. It already has a bootable kernel, a loaded SELinux policy, and labeled files. `prepare-image.sh` verifies its published SHA256, creates a COW overlay, injects an SSH key with `virt-customize` (no seed ISO), disables cloud-init, and extracts `vmlinuz` + `initramfs` for QEMU direct kernel boot. `boot.sh` passes `selinux=1 enforcing=1` on the kernel cmdline and asserts `getenforce == Enforcing`.

KVM is used when `/dev/kvm` is writable, otherwise TCG (functionally identical, 10-30x slower). No nested virtualization is needed inside the guest.

## What is copied from the host

Only small files: `bin/pinp-e`, `bin/pinp-i`, the scripts, and `guest/image/` (~4 KB: Containerfile + 3 confs). `pinp-outer` is built **inside the VM** with `podman build`, and `debian:trixie` is pulled there over QEMU user-net NAT. No `podman save` of host images; `quay.io/podman/stable` is unrelated (`Containerfile` is `FROM debian:trixie`).

## Layout

| Path | Purpose |
|---|---|
| `lib.sh` | shared vars, SSH helpers, KVM/TCG detection, VM lifecycle |
| `prepare-image.sh` | download + checksum + baked overlay + kernel/initrd extraction |
| `prepare-guest.sh` | boot once, run `guest/setup.sh`, freeze as `prepared.qcow2` |
| `boot.sh` | `start [overlay.qcow2] \| stop \| status` |
| `run-matrix.sh` | eight cases on fresh COW overlays, collect artifacts |
| `summarize.sh` | render `RESULTS.md` from collected artifacts |
| `guest/setup.sh` | dnf packages, `devuser` + subuids, build/pull images, SELinux assertions |
| `guest/run-case.sh` | one outer `podman run` + inner checks |
| `guest/collect-avc.sh` | `ausearch`, `journalctl -k`, `audit2allow` |
| `guest/image/` | `pinp-outer` build context |
| `results/` | generated per-case evidence (not committed) |
| `test_matrix.py` | opt-in pytest entry point (marker `selinux`, gated by `PINP_SELINUX_TEST=1`); runs `run-matrix.sh` via a subprocess |

Artifacts live in `${PINP_SELINUX_CACHE:-<repo>/.selinux-cache}` — project-local, on-disk (not a tmpfs), and `PINP_SELINUX_CACHE` can override it. `/tmp` is deliberately avoided because it is a RAM-backed tmpfs on this host.

## Prerequisites

`qemu-system-x86_64`, `qemu-img`, `libguestfs` (`virt-customize`, `guestfish`), `openssh`, `curl`, `python3`, `sha256sum`. All present on the dev host. Optional: `shellcheck`.

## Run

```
make build
bash tests/selinux/prepare-image.sh
bash tests/selinux/prepare-guest.sh
bash tests/selinux/boot.sh start
bash tests/selinux/boot.sh stop
make selinux-test
```

`make selinux-test` now delegates to pytest (it builds the binaries, then runs `test_matrix.py` under the `selinux` marker with `PINP_SELINUX_TEST=1`), which runs the whole matrix via `run-matrix.sh` and then `summarize.sh`. You can also invoke the opt-in test directly:

```
PINP_SELINUX_TEST=1 uv run --project tests pytest -q -s -c tests/pyproject.toml -m selinux tests/selinux/test_matrix.py
```

Without `PINP_SELINUX_TEST=1` the test skips, so the default suite stays fast and green. The harness is now pytest-executable per the testing policy in `AGENTS.md`, while the bash scripts remain the harness body.

Remove `${CACHE_DIR}/prepared.qcow2` to force a fresh guest setup; remove `${CACHE_DIR}/baked.qcow2` to force re-baking.

## Case matrix

| Case | `--security-opt` | bind suffix | Purpose |
|---|---|---|---|
| A | `label=disable` | none | control (today's PoC) |
| B | none | none | baseline: proves harness detects AVCs |
| C | `label=nested` | none | isolate nested transition |
| D | `label=nested` | `:z` | main candidate |
| E | `label=nested` | `:Z` | private-label comparison |
| F | none | `:z` | isolate relabel-only |
| G | `label=disable` | `:z` | control with relabel |
| H | `label=disable` | `:Z` | control with private label |

Per case: `ls -Z` before/after the two bind sources, `podman image ls` `R/O=true` in L1, inner `echo "hello world"`, inner `podman build`, inner `pinp-i check --json --no-fail`, graph driver/runtime, and AVC reports.

## Critical gotchas

- `:z`/`:Z` write `security.selinux` xattrs on the **mount source**. A 9p/virtiofs share from this non-SELinux host cannot carry them, so all bind sources (workdir and shared store) must be inside the guest's native ext4/xfs. Never bind-mount from the host into the VM.
- `:z` mutates labels of the VM's own image store; `:Z` assigns a private MCS that can break sharing. One fresh COW overlay per case avoids cross-contamination.
- `--device /dev/fuse` into a confined container may be denied independently of `label=nested`; `run-case.sh` records that separately.
- `pinp-outer` is Debian-based (no SELinux awareness). Labels are applied by the VM kernel to L1; `label=nested` is about the L1 process label permitting nested containers.
- Sparse-`/etc/subuid` `podman build` failures are an unrelated unresolved spike — report them separately from AVCs.

## Result

`make selinux-test` regenerates `RESULTS.md` from `tests/selinux/results/`. On the reference run (Fedora 45, podman 6.1.3, container-selinux 2.251.0), only `label=disable` permits the inner podman to run containers and builds; `:z`/`:Z` + `label=nested` are not substitutes, and no AVCs are emitted even with `semodule -DB`. See `RESULTS.md` and the findings note in `TODO.md`.

Per the experiment's scope, `INFO.md` and `pinp-e` are **not** edited by this harness.
