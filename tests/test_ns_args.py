from __future__ import annotations

import json
from pathlib import Path

import pytest

from helpers import base_env, explained_argv, run_wrapper, unset_config_env

CONFIG_FLAG_NAMES = [
    "--network",
    "--ipc",
    "--uts",
    "--cgroupns",
    "--pid",
    "--cgroups",
    "--log-driver",
    "--runtime",
    "--cgroup-manager",
    "--events-backend",
    "--storage-driver",
    "--storage-opt",
]


def _injected(argv: list[str], flag: str, value: str) -> bool:
    return any(argv[i] == flag and argv[i + 1] == value for i in range(len(argv) - 1))


def test_run_injects_all_config_args(binaries: Path, tmp_path: Path) -> None:
    env = unset_config_env(tmp_path)
    result = run_wrapper(binaries, "pinp-i", ["run", "debian:trixie"], env)
    assert explained_argv(result) == [
        "podman",
        "run",
        "--network",
        "host",
        "--ipc",
        "host",
        "--uts",
        "host",
        "--cgroupns",
        "host",
        "--pid",
        "host",
        "--cgroups",
        "enabled",
        "--log-driver",
        "k8s-file",
        "--runtime",
        "crun",
        "--cgroup-manager",
        "cgroupfs",
        "--events-backend",
        "file",
        "--storage-driver",
        "overlay",
        "--storage-opt",
        "overlay.mount_program=/usr/bin/fuse-overlayfs",
        "--uidmap",
        "0:0:1500",
        "--uidmap",
        "65534:1500:2",
        "debian:trixie",
    ]


def test_create_injects_all_config_args(binaries: Path, tmp_path: Path) -> None:
    env = unset_config_env(tmp_path)
    result = run_wrapper(binaries, "pinp-i", ["create", "debian:trixie"], env)
    argv = explained_argv(result)
    assert argv[1] == "create"
    for flag in CONFIG_FLAG_NAMES:
        assert flag in argv
    assert argv[-1] == "debian:trixie"


def test_build_injects_build_subset(binaries: Path, tmp_path: Path) -> None:
    env = unset_config_env(tmp_path)
    result = run_wrapper(binaries, "pinp-i", ["build", "-t", "x", "."], env)
    argv = explained_argv(result)
    assert argv == [
        "podman",
        "build",
        "--network",
        "host",
        "--ipc",
        "host",
        "--uts",
        "host",
        "--cgroupns",
        "host",
        "--pid",
        "host",
        "--runtime",
        "crun",
        "--cgroup-manager",
        "cgroupfs",
        "--events-backend",
        "file",
        "--storage-driver",
        "overlay",
        "--storage-opt",
        "overlay.mount_program=/usr/bin/fuse-overlayfs",
        "-t",
        "x",
        ".",
    ]
    assert "--cgroups" not in argv
    assert "--log-driver" not in argv
    assert "--uidmap" not in argv


def test_fully_configured_env_injects_nothing(binaries: Path, tmp_path: Path) -> None:
    env = base_env(tmp_path)
    result = run_wrapper(binaries, "pinp-i", ["run", "debian:trixie"], env)
    assert explained_argv(result) == [
        "podman",
        "run",
        "--uidmap",
        "0:0:1500",
        "--uidmap",
        "65534:1500:2",
        "debian:trixie",
    ]


def test_partial_config(binaries: Path, tmp_path: Path) -> None:
    env = unset_config_env(tmp_path)
    (Path(env["PINP_ETC_CONTAINERS"]) / "containers.conf").write_text(
        '[containers]\nnetns="host"\n'
    )
    result = run_wrapper(binaries, "pinp-i", ["run", "debian:latest"], env)
    argv = explained_argv(result)
    assert "--network" not in argv
    assert "--ipc" in argv
    assert "--cgroups" in argv


@pytest.mark.parametrize(
    "args",
    [
        ["run", "--network", "bridge", "debian:latest"],
        ["run", "--network=bridge", "debian:latest"],
        ["run", "--net", "bridge", "debian:latest"],
    ],
)
def test_caller_flag_suppresses_matching_injection(
    binaries: Path, tmp_path: Path, args: list[str]
) -> None:
    env = unset_config_env(tmp_path)
    result = run_wrapper(binaries, "pinp-i", args, env)
    argv = explained_argv(result)
    assert not _injected(argv, "--network", "host")
    assert "--ipc" in argv


@pytest.mark.parametrize(
    "args",
    [
        ["--runtime", "runc", "run", "debian:latest"],
        ["--runtime=runc", "run", "debian:latest"],
    ],
)
def test_caller_runtime_flag_suppresses_matching_injection(
    binaries: Path, tmp_path: Path, args: list[str]
) -> None:
    env = unset_config_env(tmp_path)
    result = run_wrapper(binaries, "pinp-i", args, env)
    argv = explained_argv(result)
    assert not _injected(argv, "--runtime", "crun")
    assert "--network" in argv


def test_pinp_config_args_off(binaries: Path, tmp_path: Path) -> None:
    env = unset_config_env(tmp_path)
    env["PINP_CONFIG_ARGS"] = "0"
    result = run_wrapper(binaries, "pinp-i", ["run", "debian:latest"], env)
    argv = explained_argv(result)
    assert "--network" not in argv
    assert argv.count("--uidmap") == 2


def test_passthrough_and_other_subcommands(binaries: Path, tmp_path: Path) -> None:
    env = unset_config_env(tmp_path)
    env["PINP_PASSTHROUGH"] = "1"
    result = run_wrapper(binaries, "pinp-i", ["run", "debian:latest"], env)
    assert explained_argv(result) == ["podman", "run", "debian:latest"]
    plain = unset_config_env(tmp_path)
    for args in (["ps"], ["images"], ["pull", "debian"], ["version"], ["info"], []):
        result = run_wrapper(binaries, "pinp-i", args, plain)
        assert explained_argv(result) == ["podman", *args]


def test_check_unset_config_warns(binaries: Path, stub_podman: Path, tmp_path: Path) -> None:
    env = unset_config_env(tmp_path)
    env["PINP_EXPLAIN"] = "0"
    env["PINP_PODMAN"] = str(stub_podman)
    result = run_wrapper(
        binaries, "pinp-i", ["pinp-check", "--json", "--no-fail", "--only", "config netns"], env
    )
    assert result.returncode == 0, result.stderr
    data = json.loads(result.stdout)
    assert len(data["checks"]) == 1
    check = data["checks"][0]
    assert check["name"] == "config netns"
    assert check["status"] == "warn"
    assert "not set" in check["detail"]
    assert "injects" in check["detail"]


def test_check_additionalimagestores_warns(
    binaries: Path, stub_podman: Path, tmp_path: Path
) -> None:
    env = unset_config_env(tmp_path)
    env["PINP_EXPLAIN"] = "0"
    env["PINP_PODMAN"] = str(stub_podman)
    result = run_wrapper(
        binaries,
        "pinp-i",
        ["pinp-check", "--json", "--no-fail", "--only", "config additionalimagestores"],
        env,
    )
    assert result.returncode == 0, result.stderr
    data = json.loads(result.stdout)
    assert len(data["checks"]) == 1
    check = data["checks"][0]
    assert check["status"] == "warn"
    assert "no podman CLI equivalent" in check["detail"]


def test_check_fully_configured_passes(binaries: Path, stub_podman: Path, tmp_path: Path) -> None:
    env = base_env(tmp_path)
    env["PINP_EXPLAIN"] = "0"
    env["PINP_PODMAN"] = str(stub_podman)
    result = run_wrapper(
        binaries, "pinp-i", ["pinp-check", "--json", "--no-fail", "--only", "config netns"], env
    )
    assert result.returncode == 0, result.stderr
    data = json.loads(result.stdout)
    assert len(data["checks"]) == 1
    assert data["checks"][0]["status"] == "pass"
