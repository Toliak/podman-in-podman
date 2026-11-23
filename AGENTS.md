# AGENTS.md

`pinp` — Go CLI wrappers for running rootless podman inside rootless podman.

## Layout

- `pinp/` — Go module (`module pinp`, imports unchanged): `go.mod`, `go.sum`, `cmd/pinp-e`, `cmd/pinp-i`, `internal/pinp`.
  - `cmd/pinp-e` — host wrapper: adds outer uidmap/gidmap, `/dev/fuse`, read-only shared image store and (on SELinux hosts) `--security-opt label=disable` to `run`/`create`.
  - `cmd/pinp-i` — L1 wrapper: adds inner `--uidmap`s to `run`/`create`, runs preflight checks, exposes `check`.
  - `internal/pinp` — argv parsing/injection, id maps, `podman info`, effective config merge, wrapper TOML config, file caps, preflight report.
- `tests/` — Python project (`pyproject.toml`, `conftest.py`, `helpers.py`, `test_*.py`): pytest/ruff black-box suite.
- `tests/selinux/` — opt-in QEMU/SELinux experiment harness (`make selinux-test`); not part of `make all`.
- Root = `Makefile` + docs (`.gitignore`, `AGENTS.md`, `INFO.md`, `TODO.md`) + generated `bin/`, `.venv/`, caches.
- `INFO.md` = background/requirements; `TODO.md` = tracked experiments (do not remove).

## Commands

- `make all` = build both binaries + `go -C pinp test ./...` + pytest + `ruff check` + `ruff format --check` + `go -C pinp vet ./...`.
- `make build | test | lint | vet | clean`
- `make selinux-test` = boot a Fedora 45 QEMU VM (KVM or TCG) and run the SELinux `:z` / `label=nested` matrix via pytest (`tests/selinux/test_matrix.py`, marker `selinux`, gated by `PINP_SELINUX_TEST=1`). Downloads ~607 MB; artifacts in `.selinux-cache/` (override with `PINP_SELINUX_CACHE`).
- Opt-in e2e: `PINP_E2E=1 .venv/bin/python -m pytest -q -c tests/pyproject.toml -m e2e tests/test_e2e.py`.
- Venv: `uv venv .venv && uv pip install --python .venv/bin/python pytest ruff`.
- Go deps are only `pflag`, `toml`, `gocapability`. Do NOT import `github.com/containers/*` or podman packages. Modules are fetched at build time (no `vendor/`).
- Inspect without executing: `PINP_EXPLAIN=1 bin/pinp-e run debian:latest`.
- Silence the echoed command: `PINP_ECHO=0 bin/pinp-e run debian:latest`.

## Testing

- Unit tests may be Go (`*_test.go` in the Go module, `go -C pinp test ./...`); every other test — integration, e2e, SELinux — must be written in or executable via pytest.
- Checked 2026-11-23: Go unit tests cover shell quoting, wrapper config precedence, id maps and argv helpers; 102 pytest tests collected (100 pass, 2 opt-in skips) — black-box CLI contract tests with a stub podman; no live-podman e2e by default.
- `make test` runs `go -C pinp test ./...` then pytest.
- `tests/selinux/test_matrix.py` is opt-in (`PINP_SELINUX_TEST=1`, marker `selinux`); the bash scripts remain the harness body. `tests/test_e2e.py` is opt-in (`PINP_E2E=1`, marker `e2e`).

## Config

- `pinp.toml` (TOML only, `BurntSushi/toml`): `PINP_CONFIG` selects the file; default `$XDG_CONFIG_HOME/pinp/config.toml`, else `$HOME/.config/pinp/config.toml`. An absent file is a no-op (`pinp-i` runs inside L1 where it may not exist). Malformed TOML aborts with a clear error.
- Precedence: compiled defaults < `pinp.toml` < `PINP_*` env (env always wins). Unset keys never clobber defaults.
- Sections: `[wrapper]` (`podman`, `echo`, `explain`, `passthrough`, `skip_checks`), `[host]` (uid/gid/user/home, subuid/subgid files, store, store_opts, fuse_device, shared_store, newuidmap/newgidmap, usr_share_containers, etc_containers, `selinux`, `selinuxfs`), `[l1]` (`inner_uidmaps`). Both wrappers read the same file.

## Conventions

- Wrappers `syscall.Exec` the real podman; never capture or re-serialize its output. `PINP_ECHO=1` (default) prints one shell-quoted line of the resolved command to stderr immediately before exec; stdout stays pure podman output. No echo on `PINP_EXPLAIN` or `pinp-i check`.
- Inject only for `run`/`create`; everything else passes through unchanged. Insert flags immediately after the subcommand, never after the image name. Conflicting caller flags abort.
- `--userns=keep-id` is stripped and pinp's own maps are injected (keep-id-*compatible*, not native keep-id); every other `--userns` value is rejected with a `PINP_PASSTHROUGH=1` hint. See the spike findings in `TODO.md`.
- `--security-opt label=disable` is injected by `pinp-e` only when SELinux is enabled: `PINP_SELINUX=auto` (default) probes `/sys/fs/selinux`, `on`/`off` force it. A caller-supplied `--security-opt label=...` conflicts when injection is on.
- Control wrappers via `PINP_*` env vars or `pinp.toml` — no `--pinp-*` flags in podman's argv.
- `pinp-i build` injects nothing yet: sparse-subuid build behavior is an unresolved spike, so it warns and passes through.
- File-cap checks use `gocapability.NewFile` with a `getcap` fallback (v3 namespaced xattrs inside L1 return `EINVAL`).
- Effective config follows `containers-config(5)`: main files merged low→high, then sorted `.conf.d` drop-ins, then `CONTAINERS_CONF` / `CONTAINERS_CONF_OVERRIDE` / `CONTAINERS_STORAGE_CONF`.
- `pinp-i check --json --no-fail` is the deterministic preflight interface used by tests; it warns on an active `containers.conf userns=` and on `PODMAN_USERNS` (both silently overridden by injected maps).

## Gotchas

- Never unify uidmap and gidmap formulas — host UID != GID. Host needs >= 65536 subordinate IDs for each.
- The outer map is already 4 extents; `newuidmap` caps at 5.
- L1's `/etc/subuid` is intentionally sparse (`devuser:100000:32768` + `devuser:1:1001`).
- Test hooks: `PINP_UID/GID/USER`, `PINP_SUBUID_FILE`, `PINP_SUBGID_FILE`, `PINP_STORE`, `PINP_HOME`, `PINP_PODMAN`, `PINP_USR_SHARE_CONTAINERS`, `PINP_ETC_CONTAINERS`, `XDG_CONFIG_HOME`, `PINP_FUSE_DEVICE`, `PINP_SHARED_STORE`, `PINP_NEWUIDMAP`, `PINP_NEWGIDMAP`, `PINP_INNER_UIDMAPS`, `PINP_STORE_OPTS`, `PINP_PASSTHROUGH`, `PINP_SKIP_CHECKS`, `PINP_ECHO`, `PINP_SELINUX`, `PINP_SELINUXFS`, `PINP_CONFIG`, `PINP_E2E`, `PINP_SELINUX_TEST`.
