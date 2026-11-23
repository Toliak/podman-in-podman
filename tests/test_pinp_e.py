from __future__ import annotations

from pathlib import Path

import pytest

from helpers import base_env, explained_argv, run_wrapper

EXPECTED_MAPS = [
    "--uidmap",
    "1002:0:1",
    "--uidmap",
    "0:1:1002",
    "--uidmap",
    "100000:1003:64531",
    "--uidmap",
    "65534:65534:2",
    "--gidmap",
    "1006:0:1",
    "--gidmap",
    "0:1:1006",
    "--gidmap",
    "100000:1007:64527",
    "--gidmap",
    "65534:65534:2",
]


def test_run_injection(binaries: Path, tmp_path: Path) -> None:
    env = base_env(tmp_path)
    result = run_wrapper(binaries, "pinp-e", ["run", "debian:latest"], env)
    assert explained_argv(result) == [
        "podman",
        "run",
        "--security-opt",
        "label=disable",
        "--device",
        "/dev/fuse",
        "-v",
        f"{tmp_path / 'store'}:/var/lib/shared:ro",
        *EXPECTED_MAPS,
        "debian:latest",
    ]


def test_create_injection(binaries: Path, tmp_path: Path) -> None:
    env = base_env(tmp_path)
    result = run_wrapper(binaries, "pinp-e", ["create", "--name", "x", "debian:latest"], env)
    argv = explained_argv(result)
    assert argv[1] == "create"
    assert argv[argv.index("--name") + 1] == "x"
    assert argv[-1] == "debian:latest"
    assert "--uidmap" in argv


@pytest.mark.parametrize(
    "args",
    [
        ["build", "-t", "x", "."],
        ["ps", "-a"],
        ["images"],
        ["pull", "debian:trixie"],
        ["rm", "foo"],
        ["exec", "-it", "foo", "bash"],
        ["info"],
        ["version"],
        [],
    ],
)
def test_passthrough_subcommands(binaries: Path, tmp_path: Path, args: list[str]) -> None:
    env = base_env(tmp_path)
    result = run_wrapper(binaries, "pinp-e", args, env)
    assert explained_argv(result) == ["podman", *args]


@pytest.mark.parametrize(
    "prefix",
    [
        ["--log-level", "debug"],
        ["--log-level=debug"],
        ["--root", "/x", "--runroot", "/y"],
        ["-c", "remote"],
        ["--remote"],
        ["--storage-driver", "overlay", "--storage-opt", "overlay.mountopt=nodev"],
    ],
)
def test_global_flags_preserved(binaries: Path, tmp_path: Path, prefix: list[str]) -> None:
    env = base_env(tmp_path)
    args = [*prefix, "run", "debian:latest"]
    result = run_wrapper(binaries, "pinp-e", args, env)
    argv = explained_argv(result)
    assert argv[1 : 1 + len(prefix)] == prefix
    assert argv.index("--uidmap") > argv.index("run")
    assert argv[-1] == "debian:latest"


def test_injection_before_image_name(binaries: Path, tmp_path: Path) -> None:
    env = base_env(tmp_path)
    result = run_wrapper(binaries, "pinp-e", ["run", "-it", "debian:latest", "bash"], env)
    argv = explained_argv(result)
    assert argv.index("--uidmap") < argv.index("debian:latest")
    assert argv[-2:] == ["debian:latest", "bash"]


@pytest.mark.parametrize(
    "extra",
    [
        ["--uidmap", "0:0:1"],
        ["--uidmap=0:0:1"],
        ["--gidmap", "0:0:1"],
        ["--userns=host"],
        ["-v", "/x:/var/lib/shared"],
        ["--volume=/x:/var/lib/shared"],
        ["--device", "/dev/fuse"],
    ],
)
def test_conflicts_rejected(binaries: Path, tmp_path: Path, extra: list[str]) -> None:
    env = base_env(tmp_path)
    result = run_wrapper(binaries, "pinp-e", ["run", *extra, "debian:latest"], env)
    assert result.returncode != 0
    assert "conflicting arguments" in result.stderr


def test_passthrough_env(binaries: Path, tmp_path: Path) -> None:
    env = base_env(tmp_path)
    env["PINP_PASSTHROUGH"] = "1"
    result = run_wrapper(binaries, "pinp-e", ["run", "debian:latest"], env)
    assert explained_argv(result) == ["podman", "run", "debian:latest"]


def test_store_opts_override(binaries: Path, tmp_path: Path) -> None:
    env = base_env(tmp_path)
    env["PINP_STORE_OPTS"] = ":z"
    result = run_wrapper(binaries, "pinp-e", ["run", "debian:latest"], env)
    assert f"{tmp_path / 'store'}:/var/lib/shared:z" in explained_argv(result)


def test_store_opts_can_be_disabled(binaries: Path, tmp_path: Path) -> None:
    env = base_env(tmp_path)
    env["PINP_STORE_OPTS"] = ""
    result = run_wrapper(binaries, "pinp-e", ["run", "debian:latest"], env)
    assert f"{tmp_path / 'store'}:/var/lib/shared" in explained_argv(result)


def test_sparse_host_ranges_warn(binaries: Path, tmp_path: Path) -> None:
    env = base_env(tmp_path, sparse=True)
    result = run_wrapper(binaries, "pinp-e", ["run", "debian:latest"], env)
    assert result.returncode == 0
    assert "sparse" in result.stderr


def test_insufficient_host_ranges_fail(binaries: Path, tmp_path: Path) -> None:
    env = base_env(tmp_path)
    (tmp_path / "subuid").write_text("tester:231072:100\n")
    (tmp_path / "subgid").write_text("tester:231072:100\n")
    result = run_wrapper(binaries, "pinp-e", ["run", "debian:latest"], env)
    assert result.returncode != 0
    assert "too small" in result.stderr
