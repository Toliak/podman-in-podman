# Non-SELinux nested-podman golden path

Opt-in end-to-end test of the shipped wrappers (`pinp-e` -> L1 -> `pinp-i` -> L2)
on a **non-SELinux** Fedora 45 guest. It reuses the `tests/selinux/` image build
(download, baked overlay, kernel/initrd, `vm.env`, SSH key) from the shared cache
and only differs in the kernel cmdline (`selinux=0`) and the package set.

This is **not** part of `make all`. It boots a QEMU VM and needs network egress
(the ~607 MB Fedora image is shared with `make selinux-test`, so it is downloaded
once).

## Relationship to the SELinux harness

| Shared with `tests/selinux/` | Kept per-mode |
|---|---|
| base qcow2, `baked.qcow2`, `kernel/`, `vm.env`, SSH key (`$CACHE_DIR`) | `prepared.qcow2`, pidfile, serial log, case overlays (`$RUN_CACHE`) |
| `lib.sh` (SSH/qemu helpers), `prepare-image.sh`, `boot.sh` | `prepare-guest.sh`, `run-matrix.sh`, `summarize.sh`, guest scripts |
| `guest/image/` L1 build context (Containerfile + confs) | the package set, the golden-path checks, the pytest entry point |

`lib.sh` exports `PINP_VM_*` overrides (default unchanged) so the SELinux harness
keeps its old filenames/SSH port and remains reproducible.

## Layout

| Path | Purpose |
|---|---|
| `lib.sh` | sources `../selinux/lib.sh`, sets `RUN_CACHE`, SSH port 2223, `selinux=0` |
| `boot.sh` | shim -> `../selinux/boot.sh` with the mode overrides |
| `up.sh` | prepare image + guest, boot a fresh COW overlay |
| `prepare-guest.sh` | one-time provisioning (`guest/setup.sh`), freeze `prepared.qcow2` |
| `collect-case.sh` | push `guest/run-case.sh`, run it, tar results back |
| `run-matrix.sh` | manual: prepare + boot + run the golden case + `RESULTS.md` |
| `summarize.sh` | render `RESULTS.md` |
| `vm-exec.sh` | run one argv in the guest over SSH (used by the pytest fixture) |
| `guest/setup.sh` | dnf (no SELinux packages), assert `getenforce=Disabled`, build `pinp-outer` |
| `guest/run-case.sh` | golden path: `pinp-e run` (bare `keep-id`) -> `pinp-i run`/`build`/`pinp-check` |
| `test_nonselinux.py` | opt-in pytest entry point (marker `nonselinux`, `PINP_NONSELINUX_TEST=1`) |
| `results/`, `RESULTS.md` | generated evidence (results gitignored) |

The shared **portable checks** live in `tests/helpers.py::assert_portable_checks`
and are also run on the host by `tests/test_e2e.py`, so the host and the VM assert
the same contract (no `label=disable`, `/dev/fuse` + read-only shared store + id
maps injected, inner uid maps injected, preflight emits a valid report).

## Run

```
make build
bash tests/nonselinux/up.sh            # boots the VM (throws away on stop)
bash tests/nonselinux/boot.sh stop
make nonselinux-test                   # prepare + boot + pytest + teardown
```

Direct:

```
PINP_NONSELINUX_TEST=1 uv run --project tests pytest -q -s \
  -c tests/pyproject.toml -m nonselinux tests/nonselinux/test_nonselinux.py
```

Without `PINP_NONSELINUX_TEST=1` the test skips, so the default suite is unchanged.

## Overrides

`PINP_NONSELINUX_CACHE` (default `<repo>/.nonselinux-cache`),
`PINP_NONSELINUX_SSH_PORT` (default `2223`). The base image cache is the usual
`PINP_SELINUX_CACHE` (default `<repo>/.selinux-cache`).
