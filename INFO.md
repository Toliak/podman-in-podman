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
- Usable `newuidmap` / `newgidmap` (setuid-root, or `cap_setuid`/`cap_setgid`
  file capabilities) to set up the outer user namespace.
- Outer container run flags:
  - `--device /dev/fuse`
  - `--security-opt label=disable` — only on SELinux-enforcing hosts; a no-op
    elsewhere. `pinp-e` injects it conditionally (`PINP_SELINUX=auto` probes
    `/sys/fs/selinux`, `on`/`off` force it). See `TODO.md` findings.
  - custom `--uidmap` / `--gidmap`: maps the container's user
    back to the host user, container root to a subuid, and exposes a wide range
    (`100000:...`) of intermediate IDs — do **not** unify uidmap and gidmap when
    UID != GID.
  - `-v $HOME/.local/share/containers/storage:/var/lib/shared` (optional): bind
    the host image store so the inner podman sees host images read-only.
- Base image present locally (e.g. `debian:trixie`) — no pull needed inside.

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

Config files (see `tests/selinux/guest/image/`):
- `/etc/containers/containers.conf` — `netns=host`, `ipcns=host`,
  `utsns=host`, `cgroupns=host`, `cgroups=disabled`, `log_driver=k8s-file`,
  `cgroup_manager=cgroupfs`, `runtime=crun`, `events_logger=file`.
- `~/.config/containers/containers.conf` — `pidns=host`, `cgroups=enabled`,
  `default_sysctls=[]`, `volumes=[]`.
- `~/.config/containers/storage.conf` — `driver=overlay`,
  `additionalimagestores=["/var/lib/shared"]`,
  `mount_program=/usr/bin/fuse-overlayfs`, `mountopt="nodev,fsync=0"`.

`/var/lib/shared` mount point must exist and be owned by the user.

## Running the inner container (L2)

Run as the non-root user, with a `--uidmap` that fits the container's subuid
ranges:
```
podman run --rm --uidmap 0:0:1500 --uidmap 65534:1500:2 debian:trixie echo "hello world"
```

## Quick sanity checks

- In L1: `podman image ls` → host images listed with `RO=true` (shared store).
- In L1: `podman info` → `rootless=true`, `driver=overlay`, `runtime=crun`.
- In L2: `/proc/self/uid_map` shows a nested mapping (not a single identity map).
