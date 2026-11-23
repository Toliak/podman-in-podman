# AGENTS.md

`pinp` — Go CLI wrappers for running rootless podman inside rootless podman.

## Layout

- `cmd/pinp-e` — host wrapper: adds outer uidmap/gidmap, `/dev/fuse`, read-only shared image store and `--security-opt label=disable` to `run`/`create`.
- `cmd/pinp-i` — L1 wrapper: adds inner `--uidmap`s to `run`/`create`, runs preflight checks, exposes `check`.
- `internal/pinp` — argv parsing/injection, id maps, `podman info`, effective config merge, file caps, preflight report.
- `tests/` + `pyproject.toml` — pytest/ruff black-box suite. `Makefile` ties it together.
- `tests/selinux/` — opt-in QEMU/SELinux experiment harness (`make selinux-test`); not part of `make all`.
- `INFO.md` = background/requirements; `TODO.md` = tracked experiments (do not remove).

## Commands

- `make all` = build both binaries + pytest + `ruff check` + `ruff format --check` + `go vet ./...`.
- `make build | test | lint | vet | clean`
- `make selinux-test` = boot a Fedora 45 QEMU VM (KVM or TCG) and run the SELinux `:z` / `label=nested` matrix. Downloads ~607 MB; artifacts in `.selinux-cache/` (override with `PINP_SELINUX_CACHE`).
- Venv: `uv venv .venv && uv pip install --python .venv/bin/python pytest ruff`.
- Go deps are only `pflag`, `toml`, `gocapability`. Do NOT import `github.com/containers/*` or podman packages. Modules are fetched at build time (no `vendor/`).
- Inspect without executing: `PINP_EXPLAIN=1 bin/pinp-e run debian:latest`.

## Conventions

- Wrappers `syscall.Exec` the real podman; never capture or re-serialize its output.
- Inject only for `run`/`create`; everything else passes through unchanged. Insert flags immediately after the subcommand, never after the image name. Conflicting caller flags abort.
- Control wrappers via `PINP_*` env vars only — no `--pinp-*` flags in podman's argv.
- `label=disable` is always injected by `pinp-e` (PoC decision; SELinux alternatives live in `TODO.md`).
- `pinp-i build` injects nothing yet: sparse-subuid build behavior is an unresolved spike, so it warns and passes through.
- File-cap checks use `gocapability.NewFile` with a `getcap` fallback (v3 namespaced xattrs inside L1 return `EINVAL`).
- Effective config follows `containers-config(5)`: main files merged low→high, then sorted `.conf.d` drop-ins, then `CONTAINERS_CONF` / `CONTAINERS_CONF_OVERRIDE` / `CONTAINERS_STORAGE_CONF`.
- `pinp-i check --json --no-fail` is the deterministic preflight interface used by tests.

## Gotchas

- Never unify uidmap and gidmap formulas — host UID != GID. Host needs >= 65536 subordinate IDs for each.
- The outer map is already 4 extents; `newuidmap` caps at 5.
- L1's `/etc/subuid` is intentionally sparse (`devuser:100000:32768` + `devuser:1:1001`).
- Test hooks: `PINP_UID/GID/USER`, `PINP_SUBUID_FILE`, `PINP_SUBGID_FILE`, `PINP_STORE`, `PINP_HOME`, `PINP_PODMAN`, `PINP_USR_SHARE_CONTAINERS`, `PINP_ETC_CONTAINERS`, `XDG_CONFIG_HOME`, `PINP_FUSE_DEVICE`, `PINP_SHARED_STORE`, `PINP_NEWUIDMAP`, `PINP_NEWGIDMAP`, `PINP_INNER_UIDMAPS`, `PINP_STORE_OPTS`, `PINP_PASSTHROUGH`, `PINP_SKIP_CHECKS`.
