from __future__ import annotations

import json
import os
import subprocess
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]

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
    }


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
