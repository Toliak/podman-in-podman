# AGENTS.md

`pinp` — Go CLI wrappers for running rootless podman inside rootless podman.

## Layout

- `pinp/` — Go module (`module pinp`, imports unchanged): `go.mod`, `go.sum`, `cmd/pinp-e`, `cmd/pinp-i`, `internal/pinp`.
  - `cmd/pinp-e` — host wrapper: adds outer uidmap/gidmap, `/dev/fuse`, read-only shared image store and (on SELinux hosts) `--security-opt label=disable` to `run`/`create`.
  - `cmd/pinp-i` — L1 wrapper: adds inner `--uidmap`s to `run`/`create`, injects config-derived namespace/engine/storage flags on `run`/`create`/`build` when `containers.conf` keys are unset, runs preflight checks, exposes the pinp-only subcommands `pinp-check` and `pinp-prepare` (alias `gen-sub`).
  - `internal/pinp` — argv parsing/injection, id maps, `podman info`, effective config merge, wrapper TOML config, file caps, preflight report, `pinp-` subcommand dispatch (`commands.go`), and L1 subuid/subgid derivation (`prepare.go`).
- `tests/` — Python project (`pyproject.toml`, `uv.lock`, `conftest.py`, `helpers.py`, `test_*.py`): pytest/ruff black-box suite managed by `uv`.
- `tests/selinux/` — opt-in QEMU/SELinux experiment harness (`make selinux-test`); not part of `make all`.
- Root = `Makefile` + docs (`.gitignore`, `AGENTS.md`, `INFO.md`, `TODO.md`) + generated `bin/`, `tests/.venv/`, caches.
- `INFO.md` = background/requirements; `TODO.md` = tracked experiments (do not remove).

## Commands

- `make all` = build both binaries + `go -C pinp test ./...` + pytest + `ruff check` + `ruff format --check` + `go -C pinp vet ./...`.
- `make build | test | lint | vet | clean | sync | selinux-test`
- `make selinux-test` = boot a Fedora 45 QEMU VM (KVM or TCG) and run the SELinux `:z` / `label=nested` matrix via pytest (`tests/selinux/test_matrix.py`, marker `selinux`, gated by `PINP_SELINUX_TEST=1`). Downloads ~607 MB; artifacts in `.selinux-cache/` (override with `PINP_SELINUX_CACHE`).
- Opt-in e2e: `PINP_E2E=1 uv run --project tests pytest -q -c tests/pyproject.toml -m e2e tests/test_e2e.py`.
- Venv: `make sync` runs `uv sync --project tests --frozen`; `tests/uv.lock` is committed, the venv lives at `tests/.venv`, requires `uv` 0.13+.
- Go deps are only `pflag`, `toml`, `gocapability`. Do NOT import `github.com/containers/*` or podman packages. Modules are fetched at build time (no `vendor/`).
- Inspect without executing: `PINP_EXPLAIN=1 bin/pinp-e run debian:latest`.
- Silence the echoed command: `PINP_ECHO=0 bin/pinp-e run debian:latest`.

## Testing

- Unit tests may be Go (`*_test.go` in the Go module, `go -C pinp test ./...`); every other test — integration, e2e, SELinux — must be written in or executable via pytest.
- Checked 2026-11-24: Go unit tests cover shell quoting, wrapper config precedence, id maps (incl. target-aware `OuterMaps` + `ParseKeepID` + `ValidateKeepID`), argv helpers, config-arg injection, the `pinp-` dispatch table and L1 id-map derivation; 160 pytest tests collected (158 pass, 2 opt-in skips) — black-box CLI contract tests with a stub podman; no live-podman e2e by default.
- `make test` runs `go -C pinp test ./...` then pytest (both depend on `make sync`).
- `tests/selinux/test_matrix.py` is opt-in (`PINP_SELINUX_TEST=1`, marker `selinux`); the bash scripts remain the harness body. `tests/test_e2e.py` is opt-in (`PINP_E2E=1`, marker `e2e`).

## Config

- `pinp.toml` (TOML only, `BurntSushi/toml`): `PINP_CONFIG` selects the file; default `$XDG_CONFIG_HOME/pinp/config.toml`, else `$HOME/.config/pinp/config.toml`. An absent file is a no-op (`pinp-i` runs inside L1 where it may not exist). Malformed TOML aborts with a clear error.
- Precedence: compiled defaults < `pinp.toml` < `PINP_*` env (env always wins). Unset keys never clobber defaults.
- Sections: `[wrapper]` (`podman`, `echo`, `explain`, `passthrough`, `skip_checks`), `[host]` (uid/gid/user/home, subuid/subgid files, store, store_opts, fuse_device, shared_store, newuidmap/newgidmap, uid_map_file, gid_map_file, setcap, usr_share_containers, etc_containers, `selinux`, `selinuxfs`), `[l1]` (`inner_uidmaps`, `config_args`). Both wrappers read the same file.

## Conventions

- Wrappers `syscall.Exec` the real podman; never capture or re-serialize its output. `PINP_ECHO=1` (default) prints one shell-quoted line of the resolved command to stderr immediately before exec; stdout stays pure podman output. No echo on `PINP_EXPLAIN` or on the non-exec `pinp-i pinp-check` / `pinp-i pinp-prepare` commands.
- pinp-only subcommands are reserved under the `pinp-` prefix so they can never collide with a real podman subcommand. `pinp-i` keeps a single dispatch table (`commands.go`): `pinp-check` (preflight) and `pinp-prepare` (aliases `pinp-gen-sub`, `prepare`, `gen-sub`). Bare `check` is broken (forwarded verbatim to podman); any unknown `pinp-*` token is rejected with a clear error rather than forwarded. These commands are dispatched before the `PINP_PASSTHROUGH` short-circuit and never inject argv, `Exec`, or echo.
- `pinp-i pinp-prepare` derives L1's `/etc/subuid` + `/etc/subgid` from the first (`ID-inside-ns`) column of `/proc/self/uid_map` / `/proc/self/gid_map` (`PINP_UID_MAP_FILE` / `PINP_GID_MAP_FILE`). It drops the extent whose **parent is 0** (the host user's own id, wherever the keep-id target placed it), container id 0, identity extents (`inside==parent`, incl. the `65534:65534:2` nobody extent), caps to <= 5 extents (`--max-size` caps each range), and derives uid and gid **completely separately** (host UID != GID). A one-ID `0 0 1` own-id extent is NOT a host identity map — `IsHostIdentityMap` only refuses a single extent or a wide `0 0 <large>` extent. Default = print both files + both cap commands to stdout with zero side effects; `--write` writes `PINP_SUBUID_FILE` / `PINP_SUBGID_FILE`, clears setuid **and setgid** on the helpers, and runs `setcap` via `PINP_SETCAP` (WARN, not FAIL, on `EPERM`/`EINVAL`). Flags: `--gen-subuid`, `--gen-subgid`, `--gen-subxid=u|g|ug`, `--cap-newuidmap`, `--cap-newgidmap`, `--cap-newxidmap`, `--write`, `--max-size`, `--json`, `--user`. It must run at **L1 runtime** (entrypoint or `podman exec -u root`), not image build — `/proc/self/uid_map` only shows the 4-extent map once `pinp-e` has created L1. Injection into `InnerRunArgs` stays static; the SELinux L1 image keeps its hand-baked sparse lines as fallback (they also fit the `uid=0` default map).
- `pinp-prepare` line owner: the generated `/etc/subuid` + `/etc/subgid` lines belong to the **L1 user the host user is mapped to** — the inside id of the `parent==0,length==1` extent resolved to a name via `/etc/passwd` — NOT the user running `--write`. At L1 runtime `--write` runs as root while the files must be owned by that (non-root) mapped user. `--user NAME` / `PINP_USER` / `[host] user` override the derived owner (an explicit value is tracked as `Env.UserSet`); when no override is given and the mapped id has no passwd entry, a numeric uid is used with a WARN. `--write` WARNs (but still writes) when the owner resolves to root. `pinp-check`'s `subuid`/`subgid consistency` checks and the build-spike hint parse `/etc/sub{u,g}id` with the same resolved owner (`ResolvePrepareTarget`).
- Inject for `run`/`create`/`build`; everything else passes through unchanged. Insert flags immediately after the subcommand, never after the image name. Conflicting `--uidmap`/`--gidmap`/`--userns`/`--device`/`--volume` caller flags abort; a caller-supplied config flag suppresses that one injection instead of aborting.
- **Host `pinp-e` userns policy**: `--userns` is stripped before injecting pinp's own `--uidmap`/`--gidmap` (the two are mutually exclusive). `ParseKeepID` (`argv.go`) resolves `keep-id` → `uid=$UID,gid=$GID` (the legacy 4-extent map), `keep-id:uid=U,gid=G[,size=S]` → a **target-aware** `OuterMaps` where the host user lands at container UID `U`/GID `G`, missing `uid=`/`gid=` default to the host IDs, and `size=S` scales the bulk/nobody extents (rejected above the host subuid pool). **No `--userns` flag = `keep-id:uid=0,gid=0`** (the host user becomes container root), overriding podman's `--userns=host` default while still creating the userns + `100000` anchor. Empty `--userns=` is an alias for `host` and is rejected; `host`/`auto`/`nomap`/`ns:`/`container:`/`auto:size=` are rejected with a `PINP_PASSTHROUGH=1` hint. `pinp-e` emits a stderr **warning** (never disables the store) when the target is root (`U==0` or `G==0`) and the shared image store is enabled: the host store is then root-owned inside L1 and unusable by a rootless inner engine (see `TODO.md` spike 3). An explicit `U!=0` target needs the L1 engine to run as `U` for the store to be usable; bare `keep-id` keeps the store working. `pinp-i` (L1) keeps its **literal `keep-id` only** split policy and is out of scope. See the spike findings in `TODO.md`.
- `--security-opt label=disable` is injected by `pinp-e` only when SELinux is enabled: `PINP_SELINUX=auto` (default) probes `/sys/fs/selinux`, `on`/`off` force it. A caller-supplied `--security-opt label=...` conflicts when injection is on.
- Control wrappers via `PINP_*` env vars or `pinp.toml` — no `--pinp-*` flags in podman's argv.
- `pinp-i` injects 12 namespace/engine/storage flags when the merged `containers.conf`/`storage.conf` key is unset (`ConfigSettings` in `pinp/internal/pinp/config.go`); `PINP_CONFIG_ARGS=0` / `[l1] config_args` disables it. `podman build` gets the 10-flag subset (no `--cgroups` / `--log-driver`). Unset keys are `WARN` in preflight, not `FAIL`. `additionalimagestores` has no CLI equivalent and is never injected.
- `pinp-i build` injects config-derived flags (not `--uidmap`s) and still warns on sparse subordinate IDs; sparse-subuid build behavior is an unresolved spike.
- File-cap checks use `gocapability.NewFile` with a `getcap` fallback (v3 namespaced xattrs inside L1 return `EINVAL`). `pinp-i` preflight FAILs when `newuidmap`/`newgidmap` are setuid-root or lack `cap_setuid=ep` / `cap_setgid=ep` file caps; this "no suid" rule is L1-only — on the host, setuid-root or file caps both work.
- Effective config follows `containers-config(5)`: main files merged low→high, then sorted `.conf.d` drop-ins, then `CONTAINERS_CONF` / `CONTAINERS_CONF_OVERRIDE` / `CONTAINERS_STORAGE_CONF`.
- `pinp-i pinp-check --json --no-fail` is the deterministic preflight interface used by tests; unset config keys are `WARN` (pinp-i injects the flags), and it warns on an active `containers.conf userns=` and on `PODMAN_USERNS` (both silently overridden by injected maps). It also reports `uid_map`/`gid_map` and a derived `subuid`/`subgid consistency` warning (refresh with `pinp-prepare`).

## Gotchas

- Never unify uidmap and gidmap formulas — host UID != GID. Host needs >= 65536 subordinate IDs for each.
- The outer map is 4 extents by default (`uid=0,gid=0`) or 5 for a non-zero, non-`$UID` target; `newuidmap` caps at 5.
- L1's `/etc/subuid` is intentionally sparse (`devuser:100000:32768` + `devuser:1:1001`); the baked lines also work with the `uid=0,gid=0` default map when the shared store is not mounted.
- `podman --imagestore` redirects *newly pulled* image writes (`man podman`); it is not `additionalimagestores`, so that key still needs `storage.conf` plus the `/var/lib/shared` mount.
- Test hooks: `PINP_UID/GID/USER`, `PINP_SUBUID_FILE`, `PINP_SUBGID_FILE`, `PINP_STORE`, `PINP_HOME`, `PINP_PODMAN`, `PINP_USR_SHARE_CONTAINERS`, `PINP_ETC_CONTAINERS`, `XDG_CONFIG_HOME`, `PINP_FUSE_DEVICE`, `PINP_SHARED_STORE`, `PINP_NEWUIDMAP`, `PINP_NEWGIDMAP`, `PINP_UID_MAP_FILE`, `PINP_GID_MAP_FILE`, `PINP_SETCAP`, `PINP_INNER_UIDMAPS`, `PINP_STORE_OPTS`, `PINP_PASSTHROUGH`, `PINP_SKIP_CHECKS`, `PINP_ECHO`, `PINP_SELINUX`, `PINP_SELINUXFS`, `PINP_CONFIG`, `PINP_CONFIG_ARGS`, `PINP_E2E`, `PINP_SELINUX_TEST`.
