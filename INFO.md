# Running podman in podman (rootless in rootless)

What is needed on the **host** and inside the **outer container** to run a nested
rootless podman (tested: Debian in Debian, `podman ... echo "hello world"`).
Reference configs live in `tests/selinux/guest/image/`.

## The core idea

A rootless inner podman creates *another* user namespace. For that to work, the
outer container's user namespace must contain every UID the inner podman wants
to map. The default rootless mapping only exposes the current user, so the outer
container must be started with a **custom `--uidmap`/`--gidmap`** that hands it a
large contiguous range of subordinate IDs. Everything else exists to make that
nested namespace usable.

## On the host

- Rootless podman + OCI runtime (`crun`), cgroup v2.
- `/dev/fuse` available.
- `/etc/subuid` / `/etc/subgid`: the user owns **at least ~64k** subordinate IDs
  (e.g. `agentic:231072:65536`).
- Usable `newuidmap` / `newgidmap` (setuid-root **or** `cap_setuid`/`cap_setgid`
  file capabilities) to set up the outer user namespace. This is the **host** rule;
  inside L1 the helpers must instead be non-setuid with file caps (see below).
- Outer container run flags:
  - `--device /dev/fuse`
  - `--security-opt label=disable` — only on SELinux-enforcing hosts; a no-op
    elsewhere. `pinp-e` injects it conditionally (`PINP_SELINUX=auto` probes
    `/sys/fs/selinux`, `on`/`off` force it). See `TODO.md` findings.
  - custom `--uidmap` / `--gidmap`: a **target-aware** map that puts the host
    user at a chosen container UID/GID, keeps the `100000:...` anchor of
    intermediate IDs, and never exceeds the 5-extent `newuidmap` ceiling — do
    **not** unify uidmap and gidmap when UID != GID. With no `--userns` flag the
    host user becomes container root (`uid=0,gid=0`); see the section below.
  - `-v $HOME/.local/share/containers/storage:/var/lib/shared` (optional): bind
    the host image store so the inner podman sees host images read-only.
- Base image present locally (e.g. `debian:trixie`) — no pull needed inside.

### Host `--userns` policy (`pinp-e`)

`--userns` and `--uidmap`/`--gidmap` are mutually exclusive, so `pinp-e` strips
`--userns` and injects its own maps:

| `pinp-e` argv | host user lands at | notes |
|---|---|---|
| no `--userns` | container UID 0 / GID 0 | **default**; overrides podman's `--userns=host`, host user becomes container root |
| `--userns=keep-id` | UID `$UID` / GID `$GID` | today's legacy 4-extent map |
| `--userns=keep-id:uid=U,gid=G` | UID `U` / GID `G` | target-aware map (5 extents for `U != 0, U != $UID`) |
| `--userns=keep-id:uid=U` | UID `U` / GID `$GID` | missing component defaults to the host ID |
| `--userns=keep-id:gid=G` | UID `$UID` / GID `G` | |
| `--userns=keep-id:size=S` | `$UID` / `$GID`, total `S` | `S` must not exceed the host subuid pool |

`--userns=` (empty) is an alias for `--userns=host` and is rejected; `host`,
`auto`, `nomap`, `ns:`, `container:` and `auto:size=` are rejected with a
`PINP_PASSTHROUGH=1` hint. Both `=` and two-token forms are accepted.

**Shared-store caveat (spike-3)**: when the target is root (`U==0` or `G==0`),
the bind-mounted host store is owned by container UID 0 inside L1, so a rootless
inner engine cannot open `/var/lib/shared/overlay-images/images.lock`. `pinp-e`
emits a stderr warning but keeps the store enabled (it never disables it). Bare
`--userns=keep-id` (`U=$UID,G=$GID`) keeps the store usable; an explicit
`keep-id:uid=U,gid=G` target additionally requires the L1 engine to run **as
that UID `U`** (spike 3: the store is then owned by `U`). Root-in-L1 (rootful
engine) is not a workaround — it fails at `crun: mount 'proc' to 'proc':
Operation not permitted`. See `/tmp/opencode/pinp/userns-spike-3.md` and
`TODO.md` for the (now implemented) L1 inner policy.

## In the outer container (L1 image)

Packages:
- `podman`, `fuse-overlayfs`, `uidmap` (`newuidmap`/`newgidmap`), `crun`,
  `libcap2-bin` (for `setcap`).

A **non-root user** (e.g. `devuser`, UID/GID matching the host user) that podman
runs as (`USER devuser`). Its config dir must be owned by that user.

`/etc/subuid` and `/etc/subgid` for that user, with ranges that fit inside the
outer container's mapped IDs:
```
devuser:100000:32768
devuser:1:<USER_UID - 1>
```

`newuidmap` / `newgidmap` must run **as the user**, not as root, so they look up
that user's subordinate ranges:
```
chmod -s /usr/bin/newuidmap /usr/bin/newgidmap
setcap cap_setuid+ep /usr/bin/newuidmap
setcap cap_setgid+ep /usr/bin/newgidmap
```

### Generating the files with `pinp-i pinp-prepare`

Rather than hand-baking the sparse lines above, `pinp-i pinp-prepare` (alias
`gen-sub`) derives `/etc/subuid` + `/etc/subgid` from the `ID-inside-ns` column
of `/proc/self/uid_map` / `/proc/self/gid_map`. It drops the extent whose
**parent is 0** (the host user's own id, wherever the keep-id target put it),
container id 0, and identity extents (including the nobody extent), keeping
<= 5 extents; uid and gid are derived **separately** (host UID != GID). For the
legacy L1 map this yields `devuser:1:1001` + `devuser:100000:64531` (uid) and
`devuser:1:1005` + `devuser:100000:64527` (gid); for the `uid=0,gid=0` default
it yields just `devuser:100000:64531` / `devuser:100000:64527`, and for
`uid=100,gid=100` it adds a `1:99` low range. A one-ID `0 0 1` own-id extent is
not a host identity map, so a target-aware L1 map is accepted. By default it
only prints the two files and the cap commands (zero side effects); `--write`
writes them, clears setuid **and setgid** on the helpers, and runs `setcap`
(`PINP_SETCAP`). `--max-size 32768` reproduces the hand-baked
`...:100000:32768` line. It refuses a single-extent host identity map
("not inside a nested userns").

The generated lines must be owned by the **L1 user the host user is mapped to**
— the inside id of the `parent==0,length==1` extent — not by the user running
`--write`. At L1 runtime `--write` runs as root (`podman exec -u root`), but the
files belong to that mapped non-root user (e.g. `devuser`), which is who runs
rootless podman and calls `newuidmap`. `pinp-prepare` derives that user and
resolves its name via `/etc/passwd`; override it with `--user NAME`,
`PINP_USER`, or `[host] user`. When no override is set and the mapped id has no
passwd entry, a numeric uid is used with a WARN. `--write` WARNs (but still
writes) when the owner would resolve to root, since a rootless inner engine
cannot use root-owned subordinate ranges. `pinp-i pinp-check`'s `subuid`/
`subgid consistency` checks use the same resolved owner.

**`pinp-prepare` must run at L1 runtime** (entrypoint or `podman exec -u root`),
not image build: `/proc/self/uid_map` only shows the nested map once `pinp-e`
has created L1. The SELinux L1 image keeps the hand-baked lines as a fallback
(they also fit the `uid=0,gid=0` default when the shared store is not mounted);
wiring the harness to `pinp-prepare --write` is deferred. Injection into the
default `InnerRunArgs` stays static; an explicit keep-id `--userns` derives
target-aware maps (see the L2 section below).

`pinp-i pinp-check` enforces exactly this (L1-only): it FAILs when either helper
is setuid-root or lacks `cap_setuid=ep` / `cap_setgid=ep` file caps. On the host
either setuid-root or file caps work, so `pinp-e` does not apply this rule.

Config files (see `tests/selinux/guest/image/`):
- `/etc/containers/containers.conf` — `netns=host`, `ipcns=host`,
  `utsns=host`, `cgroupns=host`, `cgroups=disabled`, `log_driver=k8s-file`,
  `cgroup_manager=cgroupfs`, `runtime=crun`, `events_logger=file`.
- `~/.config/containers/containers.conf` — `pidns=host`, `cgroups=enabled`,
  `default_sysctls=[]`, `volumes=[]`.
- `~/.config/containers/storage.conf` — `driver=overlay`,
  `additionalimagestores=["/var/lib/shared"]`,
  `mount_program=/usr/bin/fuse-overlayfs`, `mountopt="nodev,fsync=0"`.

These keys are optional now: `pinp-i` injects the equivalent podman flags on
`run`/`create`/`build` whenever the merged effective config leaves a key unset
(`--network host`, `--ipc host`, `--uts host`, `--cgroupns host`, `--pid host`,
`--cgroups enabled`, `--log-driver k8s-file`, `--runtime crun`,
`--cgroup-manager cgroupfs`, `--events-backend file`, `--storage-driver overlay`,
`--storage-opt overlay.mount_program=...`). `podman build` gets the same list minus
`--cgroups` / `--log-driver`. The full mapping lives in
`pinp/internal/pinp/config.go`; `PINP_CONFIG_ARGS=0` disables it. Unset keys are
`WARN` in `pinp-i pinp-check`, not `FAIL`.

`additionalimagestores` has no CLI equivalent — `podman --imagestore` redirects
*newly pulled* image writes, it does not add a read-only store — so it still needs
`storage.conf` and the `/var/lib/shared` mount.

`pinp-i pinp-prepare` can generate that config instead of baking it. When
`/var/lib/shared` is a mount point (checked via `/proc/self/mountinfo`), the
default `pinp-prepare` output and `--write` include an additive drop-in at
`<target home>/.config/containers/storage.conf.d/pinp-shared-store.conf`
containing the **union** of the effective `additionalimagestores` and the shared
store. It writes to the mapped L1 user's home (not root's, not `/etc`), because
rootless podman reads `$XDG_CONFIG_HOME/containers/storage.conf.d`; `--write`
creates it `0644` and chowns it to that user when running as root. When the store
is unmounted the generation is skipped with an INFO. `PINP_STORAGE_CONF` /
`[host] storage_conf` override the main `storage.conf` path (the drop-in dir is
derived from its parent), and `pinp-i pinp-check` reports a `storage.conf` line.

### Sparse L1 build userns maps (`pinp-i build`)

A rootless `buildah` maps container 0 to the L1 user's own id and container
`1..T` to the `T` subordinate IDs from `/etc/sub{u,g}id`. apt's `http` method
drops to `_apt` with `setgroups(65534)`, so a build fails with
`E: setgroups 65534 failed` whenever `T < 65534`. With the baked sparse L1 pool
(`1:1001` + `100000:32768`, `T=33769`) container gid 65534 is unmapped even
though L1's own `/proc/self/gid_map` does map it.

`pinp-i build` injects derived `--userns-uid-map` / `--userns-gid-map` extents in
that case. The `hostID` in those triples is **not** the absolute `/etc/subuid`
start value: it is an ID in buildah's own rootless userns (`U_build`), which
numbers parent IDs sequentially from 1, one extent per range sorted by start.
crun writes the RUN-step maps directly (no `newuidmap`) and the kernel rejects an
extent whose parent range is out of `U_build`'s range or straddles two of its
extents, so maps anchored at `100000` (absolute) or at `0` with a large size
(offset) both fail `write file /proc/<pid>/gid_map: Operation not permitted`.

For the baked pool `pinp-i` injects:
```
--userns-uid-map 0:1:1001 --userns-uid-map 1001:1002:32766 --userns-uid-map 65534:33768:2
--userns-gid-map 0:1:1001 --userns-gid-map 1001:1002:32766 --userns-gid-map 65534:33768:2
```
uid and gid are derived separately (host UID != GID), one triple per range, the
last two parent IDs reserved for the `65534:...:2` nobody extent, capped at the
kernel's 5-extent limit. No injection when the pool is wide (`T >= 65534`): the
default map already covers container 65534. `PINP_BUILD_MAPS=0` / `[l1]
build_maps` disables it; caller-supplied `--userns*` / `--isolation` /
`--uidmap` / `--gidmap` build flags abort with a passthrough hint.

`tests/e2e-build/Containerfile` is the proof: a realistic apt + `useradd` +
clang build with **no** `APT::Sandbox::User` workaround, run through
`make e2e-build` inside a host-nested L1.

`/var/lib/shared` mount point must exist and be owned by the user. With the
`uid=0,gid=0` default the host store is root-owned inside L1 instead — see the
shared-store caveat above.

## Running the inner container (L2)

Run as the non-root user. With **no `--userns` flag**, `pinp-i` injects the
static maps below (semantically `uid=0,gid=0`):
```
podman run --rm --uidmap 0:0:1500 --uidmap 65534:1500:2 debian:trixie echo "hello world"
```

With an explicit `--userns=keep-id[:uid=U,gid=G][,size=S]`, `pinp-i` strips the
flag and derives **separate** `--uidmap` + `--gidmap` extents from L1's
`/etc/sub{u,g}id` so the L1 user's own id lands at container `U`/`G`. The parent
field of an explicit inner `--uidmap` is an **offset into podman's rootless
userns** (`0` = the L1 user's own id, `1..T` = the subordinate IDs sorted by
start), not an absolute `/etc/subuid` value; the last two parents (`T-1`,`T`)
always map container `65534..65535`. On the baked sparse pool (`T=33769`) the
derived maps are, for example:
```
# no target rotation (uid=0,gid=0)
--uidmap 0:0:1 --uidmap 1:1:33767 --uidmap 65534:33768:2
# bare --userns=keep-id: own id at 1002 (uid) / 1006 (gid)
--uidmap 0:1:1002 --uidmap 1002:0:1 --uidmap 1003:1003:32765 --uidmap 65534:33768:2
--gidmap 0:1:1006 --gidmap 1006:0:1 --gidmap 1007:1007:32761 --gidmap 65534:33768:2
```
`size=S` caps the bulk (`S <= 1 + pool`); targets must stay `< bulk`. An
explicit `PINP_INNER_UIDMAPS` / `[l1] inner_uidmaps` wins verbatim (uidmap-only,
target-blind); an unreadable pool falls back to the static map with a WARN.
`podman build` is unaffected — it uses the `buildmaps.go` `--userns-*-map`
derivation and still rejects `--userns`.

## Quick sanity checks

- In L1: `podman image ls` → host images listed with `RO=true` (shared store).
- In L1: `podman info` → `rootless=true`, `driver=overlay`, `runtime=crun`.
- In L2: `/proc/self/uid_map` shows a nested mapping (not a single identity map).
