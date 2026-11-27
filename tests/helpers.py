from __future__ import annotations

import json
import os
import subprocess
from collections.abc import Callable
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]

UID_MAP_L1 = "0 1 1002\n1002 0 1\n65534 65534 2\n100000 1003 64531\n"
GID_MAP_L1 = "0 1 1006\n1006 0 1\n65534 65534 2\n100000 1007 64527\n"
HOST_IDENTITY_MAP = "0 0 4294967295\n"

CANNED_INFO = {
    "host": {
        "arch": "amd64",
        "buildahVersion": "1.44.1",
        "cgroupManager": "cgroupfs",
        "cgroupVersion": "v2",
        "networkBackend": "netavark",
        "serviceIsRemote": False,
        "rootlessNetworkCmd": "pasta",
        "idMappings": {
            "uidmap": [{"container_id": 0, "host_id": 1002, "size": 1}],
            "gidmap": [{"container_id": 0, "host_id": 1006, "size": 1}],
        },
        "ociRuntime": {"name": "crun", "path": "/usr/bin/crun"},
        "security": {
            "apparmorEnabled": False,
            "capabilities": "CAP_CHOWN",
            "rootless": True,
            "seccompEnabled": True,
            "selinuxEnabled": False,
        },
        "pasta": {"executable": "/usr/bin/pasta"},
    },
    "store": {
        "graphDriverName": "overlay",
        "graphRoot": "/tmp/graph",
        "runRoot": "/tmp/run",
        "transientStore": False,
    },
}


def base_env(tmp_path: Path, *, sparse: bool = False) -> dict[str, str]:
    home = tmp_path / "home"
    home.mkdir(exist_ok=True)
    if sparse:
        content = "tester:231072:40000\ntester:300000:30000\n"
    else:
        content = "tester:231072:65536\n"
    (tmp_path / "subuid").write_text(content)
    (tmp_path / "subgid").write_text(content)
    usr = tmp_path / "usr"
    etc = tmp_path / "etc"
    xdg = tmp_path / "xdg"
    containers = xdg / "containers"
    for directory in (usr, etc, xdg, containers):
        directory.mkdir(parents=True, exist_ok=True)
    (etc / "containers.conf").write_text(
        "[containers]\n"
        'netns="host"\n'
        'ipcns="host"\n'
        'utsns="host"\n'
        'cgroupns="host"\n'
        'pidns="host"\n'
        'cgroups="enabled"\n'
        'log_driver="k8s-file"\n'
        "[engine]\n"
        'runtime="crun"\n'
        'cgroup_manager="cgroupfs"\n'
        'events_logger="file"\n'
    )
    (etc / "storage.conf").write_text(
        '[storage]\ndriver="overlay"\n'
        '[storage.options]\nadditionalimagestores=["/var/lib/shared"]\n'
        '[storage.options.overlay]\nmount_program="/usr/bin/fuse-overlayfs"\n'
    )
    fuse = tmp_path / "fuse-overlayfs"
    fuse.write_text("#!/bin/sh\nexit 0\n")
    fuse.chmod(0o755)
    return {
        "PINP_EXPLAIN": "1",
        "PINP_UID": "1002",
        "PINP_GID": "1006",
        "PINP_USER": "tester",
        "PINP_SUBUID_FILE": str(tmp_path / "subuid"),
        "PINP_SUBGID_FILE": str(tmp_path / "subgid"),
        "PINP_STORE": str(tmp_path / "store"),
        "PINP_HOME": str(home),
        "PINP_SELINUX": "auto",
        "PINP_SELINUXFS": str(tmp_path / "selinuxfs"),
        "PATH": os.environ.get("PATH", "/usr/bin:/bin"),
        "HOME": str(home),
        "PINP_USR_SHARE_CONTAINERS": str(usr),
        "PINP_ETC_CONTAINERS": str(etc),
        "PINP_FUSE_OVERLAYFS": str(fuse),
        "XDG_CONFIG_HOME": str(home / ".config"),
    }


def build_maps_env(tmp_path: Path) -> dict[str, str]:
    """A sparse L1 subordinate-ID pool too small to reach container 65534.

    Mirrors the baked /etc/sub{u,g}id in tests/selinux/guest/image: a low
    1001-ID range plus a 32768-ID anchor (33769 total). The default rootless
    build userns cannot map gid 65534, so pinp-i build must inject derived
    --userns-uid-map / --userns-gid-map extents.
    """
    env = base_env(tmp_path)
    content = "tester:100000:32768\ntester:1:1001\n"
    (tmp_path / "subuid").write_text(content)
    (tmp_path / "subgid").write_text(content)
    return env


def unset_config_env(tmp_path: Path) -> dict[str, str]:
    env = base_env(tmp_path)
    empty = tmp_path / "empty-config"
    for name in ("PINP_USR_SHARE_CONTAINERS", "PINP_ETC_CONTAINERS", "XDG_CONFIG_HOME"):
        d = empty / name.lower()
        d.mkdir(parents=True, exist_ok=True)
        env[name] = str(d)
    return env


def prepare_env(
    tmp_path: Path,
    *,
    uid_map: str | None = None,
    gid_map: str | None = None,
    with_identity: bool = True,
    shared_mounted: bool = True,
) -> dict[str, str]:
    env = base_env(tmp_path)
    if not with_identity:
        for key in ("PINP_UID", "PINP_GID", "PINP_USER"):
            env.pop(key, None)
    (tmp_path / "uid_map").write_text(uid_map if uid_map is not None else UID_MAP_L1)
    (tmp_path / "gid_map").write_text(gid_map if gid_map is not None else GID_MAP_L1)
    # gen-subuid / gen-subgid are left absent on purpose: --write creates them
    # (never real /etc); default mode must leave them untouched.
    for name in ("newuidmap", "newgidmap"):
        (tmp_path / name).write_text("#!/bin/sh\nexit 0\n")
        (tmp_path / name).chmod(0o755)
    setcap_log = tmp_path / "setcap.log"
    (tmp_path / "setcap").write_text(f'#!/bin/sh\nprintf "%s\\n" "$*" >> "{setcap_log}"\nexit 0\n')
    (tmp_path / "setcap").chmod(0o755)
    marker = tmp_path / "podman-called"
    (tmp_path / "podman-stub").write_text(f'#!/bin/sh\n: > "{marker}"\nexit 0\n')
    (tmp_path / "podman-stub").chmod(0o755)
    mountinfo = tmp_path / "mountinfo"
    shared_line = "36 25 0:32 / /var/lib/shared rw,relatime - ext4 /dev/sda1 rw\n"
    root_line = "36 25 0:32 / / rw,relatime - ext4 /dev/sda1 rw\n"
    mountinfo.write_text(shared_line if shared_mounted else root_line)
    env.update(
        {
            "PINP_UID_MAP_FILE": str(tmp_path / "uid_map"),
            "PINP_GID_MAP_FILE": str(tmp_path / "gid_map"),
            "PINP_SUBUID_FILE": str(tmp_path / "gen-subuid"),
            "PINP_SUBGID_FILE": str(tmp_path / "gen-subgid"),
            "PINP_STORAGE_CONF": str(tmp_path / "home" / ".config" / "containers" / "storage.conf"),
            "PINP_MOUNTINFO": str(mountinfo),
            "PINP_NEWUIDMAP": str(tmp_path / "newuidmap"),
            "PINP_NEWGIDMAP": str(tmp_path / "newgidmap"),
            "PINP_SETCAP": str(tmp_path / "setcap"),
            "PINP_PODMAN": str(tmp_path / "podman-stub"),
        }
    )
    return env


def run_wrapper(
    binaries: Path, wrapper: str, args: list[str], env: dict[str, str]
) -> subprocess.CompletedProcess[str]:
    return subprocess.run(
        [str(binaries / wrapper), *args],
        cwd=ROOT,
        env=env,
        capture_output=True,
        text=True,
        check=False,
    )


def explained_argv(result: subprocess.CompletedProcess[str]) -> list[str]:
    assert result.returncode == 0, result.stderr
    return json.loads(result.stdout)


def assert_portable_checks(
    run: Callable[[list[str], dict[str, str]], subprocess.CompletedProcess[str]],
    env: dict[str, str],
    pinp_e: str,
    pinp_i: str,
) -> None:
    """Checks that hold wherever SELinux is absent (dev host or non-SELinux guest).

    Shared by the host e2e test and the VM test so the same portable contract is
    exercised in both places. `run` executes one argv with `env` and returns a
    CompletedProcess (locally on the host, over SSH in the VM).
    """
    # pinp-e: no label=disable when SELinux is off, but /dev/fuse, the read-only
    # shared store and pinp's own id maps are always injected.
    result = run([pinp_e, "run", "debian:trixie", "true"], env)
    assert result.returncode == 0, result.stderr
    argv = json.loads(result.stdout)
    assert "label=disable" not in " ".join(argv), argv
    assert "--device" in argv and "/dev/fuse" in argv, argv
    assert any(arg.endswith(":/var/lib/shared:ro") for arg in argv), argv
    assert "--uidmap" in argv and "--gidmap" in argv, argv

    # pinp-i run/create: the nested inner uid maps are injected.
    result = run([pinp_i, "run", "debian:trixie", "true"], env)
    assert result.returncode == 0, result.stderr
    argv = json.loads(result.stdout)
    assert "0:0:1500" in argv and "65534:1500:2" in argv, argv

    # pinp-i pinp-check: the preflight runs end-to-end and emits a consistent
    # report. (The setuid/file-cap result is environment-specific — it is a hard
    # FAIL on a host and a PASS inside L1 — so only internal consistency is
    # asserted here; the golden path asserts zero failures inside L1.)
    result = run([pinp_i, "pinp-check", "--json", "--no-fail"], env)
    assert result.returncode == 0, result.stderr
    report = json.loads(result.stdout)
    assert report["checks"], result.stdout
    assert report["summary"]["fail"] == sum(
        1 for check in report["checks"] if check["status"] == "fail"
    ), result.stdout


def config_env(tmp_path: Path, stub_podman: Path) -> dict[str, str]:
    usr = tmp_path / "usr"
    etc = tmp_path / "etc"
    xdg = tmp_path / "xdg"
    containers = xdg / "containers"
    for directory in (usr, etc, xdg, containers):
        directory.mkdir(parents=True, exist_ok=True)
    (etc / "containers.conf").write_text(
        '[containers]\nnetns="host"\ncgroups="disabled"\n[engine]\nruntime="crun"\n'
    )
    (containers / "containers.conf").write_text('[containers]\nipcns="host"\ncgroups="enabled"\n')
    (tmp_path / "fuse").write_text("")
    (tmp_path / "shared").mkdir(exist_ok=True)
    env = base_env(tmp_path)
    env.update(
        {
            "PINP_EXPLAIN": "0",
            "PINP_PODMAN": str(stub_podman),
            "PINP_USR_SHARE_CONTAINERS": str(usr),
            "PINP_ETC_CONTAINERS": str(etc),
            "XDG_CONFIG_HOME": str(xdg),
            "PINP_FUSE_DEVICE": str(tmp_path / "fuse"),
            "PINP_SHARED_STORE": str(tmp_path / "shared"),
        }
    )
    return env
