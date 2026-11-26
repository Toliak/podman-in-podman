from __future__ import annotations

import json
from pathlib import Path

import pytest

from helpers import base_env, config_env, explained_argv, run_wrapper

USERNS_MODES_REJECTED_BOTH = [
    "host",
    "auto",
    "nomap",
    "ns:foo",
    "auto:size=8192",
]

LEGACY_MAPS = [
    "1002:0:1",
    "0:1:1002",
    "100000:1003:64531",
    "65534:65534:2",
    "1006:0:1",
    "0:1:1006",
    "100000:1007:64527",
    "65534:65534:2",
]

DEFAULT_MAPS = [
    "0:0:1",
    "1:1:1002",
    "100000:1003:64531",
    "65534:65534:2",
    "0:0:1",
    "1:1:1006",
    "100000:1007:64527",
    "65534:65534:2",
]

U100_MAPS = [
    "100:0:1",
    "0:1:100",
    "101:101:902",
    "100000:1003:64531",
    "65534:65534:2",
    "100:0:1",
    "0:1:100",
    "101:101:906",
    "100000:1007:64527",
    "65534:65534:2",
]


@pytest.mark.parametrize("wrapper", ["pinp-e", "pinp-i"])
@pytest.mark.parametrize("mode", USERNS_MODES_REJECTED_BOTH)
def test_userns_modes_rejected_with_hint(
    binaries: Path, tmp_path: Path, wrapper: str, mode: str
) -> None:
    env = base_env(tmp_path)
    result = run_wrapper(binaries, wrapper, ["run", f"--userns={mode}", "debian:latest"], env)
    assert result.returncode != 0
    assert "conflicting arguments" in result.stderr
    assert "unsupported" in result.stderr
    assert "PINP_PASSTHROUGH=1" in result.stderr


# base_env uses tester uid=1002 gid=1006 with a 65536-ID subordinate pool.
L1_BARE_UID_MAPS = ["0:1:1002", "1002:0:1", "1003:1003:64530", "65534:65535:2"]
L1_BARE_GID_MAPS = ["0:1:1006", "1006:0:1", "1007:1007:64526", "65534:65535:2"]


@pytest.mark.parametrize("userns_args", [["--userns=keep-id"], ["--userns", "keep-id"]])
def test_keep_id_is_target_aware_on_l1(
    binaries: Path, tmp_path: Path, userns_args: list[str]
) -> None:
    env = base_env(tmp_path)
    result = run_wrapper(binaries, "pinp-i", ["run", *userns_args, "debian:latest"], env)
    argv = explained_argv(result)
    assert "--userns" not in argv
    assert "keep-id" not in argv
    assert argv[-1] == "debian:latest"
    assert argv.count("--uidmap") == 4
    assert argv.count("--gidmap") == 4
    for m in L1_BARE_UID_MAPS + L1_BARE_GID_MAPS:
        assert m in argv


def test_pinp_i_explicit_target(binaries: Path, tmp_path: Path) -> None:
    env = base_env(tmp_path)
    result = run_wrapper(
        binaries,
        "pinp-i",
        ["run", "--userns=keep-id:uid=100,gid=200", "debian:latest"],
        env,
    )
    argv = explained_argv(result)
    assert "--userns" not in argv
    assert "--uidmap" in argv and "--gidmap" in argv
    assert argv.count("--uidmap") == 4
    assert argv.count("--gidmap") == 4
    assert "100:0:1" in argv
    assert "200:0:1" in argv


def test_pinp_i_size_option(binaries: Path, tmp_path: Path) -> None:
    env = base_env(tmp_path)
    result = run_wrapper(
        binaries, "pinp-i", ["run", "--userns=keep-id:size=8192", "debian:latest"], env
    )
    argv = explained_argv(result)
    # bare keep-id target 1002, size 8192 -> bulk capped at 8191 (parents 0..8190)
    assert "1003:1003:7188" in argv
    assert "65534:65535:2" in argv


def test_pinp_i_size_above_pool_rejected(binaries: Path, tmp_path: Path) -> None:
    env = base_env(tmp_path)
    result = run_wrapper(
        binaries, "pinp-i", ["run", "--userns=keep-id:size=70000", "debian:latest"], env
    )
    assert result.returncode != 0
    assert "exceeds the L1 subordinate ID pool" in result.stderr


def test_pinp_i_custom_inner_maps_override(binaries: Path, tmp_path: Path) -> None:
    env = base_env(tmp_path)
    env["PINP_INNER_UIDMAPS"] = "0:0:100 65534:100:2"
    result = run_wrapper(
        binaries, "pinp-i", ["run", "--userns=keep-id:uid=100,gid=100", "debian:latest"], env
    )
    argv = explained_argv(result)
    assert "0:0:100" in argv and "65534:100:2" in argv
    assert "--gidmap" not in argv


@pytest.mark.parametrize("userns_args", [["--userns=keep-id"], ["--userns", "keep-id"]])
def test_bare_keep_id_uses_legacy_map_on_host(
    binaries: Path, tmp_path: Path, userns_args: list[str]
) -> None:
    env = base_env(tmp_path)
    result = run_wrapper(binaries, "pinp-e", ["run", *userns_args, "debian:latest"], env)
    argv = explained_argv(result)
    assert "--userns" not in argv
    assert "keep-id" not in argv
    assert argv[-1] == "debian:latest"
    for m in LEGACY_MAPS:
        assert m in argv
    assert argv.count("--uidmap") == 4
    assert argv.count("--gidmap") == 4


def test_default_is_root_at_zero(binaries: Path, tmp_path: Path) -> None:
    env = base_env(tmp_path)
    result = run_wrapper(binaries, "pinp-e", ["run", "debian:latest"], env)
    argv = explained_argv(result)
    for m in DEFAULT_MAPS:
        assert m in argv


@pytest.mark.parametrize(
    "userns_args",
    [
        ["--userns=keep-id:uid=0,gid=0"],
        ["--userns", "keep-id:uid=0,gid=0"],
    ],
)
def test_explicit_uid0_matches_default(
    binaries: Path, tmp_path: Path, userns_args: list[str]
) -> None:
    env = base_env(tmp_path)
    result = run_wrapper(binaries, "pinp-e", ["run", *userns_args, "debian:latest"], env)
    argv = explained_argv(result)
    assert "--userns" not in argv
    for m in DEFAULT_MAPS:
        assert m in argv


def test_explicit_keep_id_target(binaries: Path, tmp_path: Path) -> None:
    env = base_env(tmp_path)
    result = run_wrapper(
        binaries, "pinp-e", ["run", "--userns=keep-id:uid=100,gid=100", "debian:latest"], env
    )
    argv = explained_argv(result)
    assert "--userns" not in argv
    for m in U100_MAPS:
        assert m in argv
    assert argv.count("--uidmap") == 5
    assert argv.count("--gidmap") == 5


def test_uid_only_and_gid_only(binaries: Path, tmp_path: Path) -> None:
    env = base_env(tmp_path)
    uid_only = explained_argv(
        run_wrapper(binaries, "pinp-e", ["run", "--userns=keep-id:uid=100", "debian:latest"], env)
    )
    assert "100:0:1" in uid_only
    assert "1006:0:1" in uid_only
    gid_only = explained_argv(
        run_wrapper(binaries, "pinp-e", ["run", "--userns=keep-id:gid=100", "debian:latest"], env)
    )
    assert "1002:0:1" in gid_only
    assert "100:0:1" in gid_only


def test_size_option(binaries: Path, tmp_path: Path) -> None:
    env = base_env(tmp_path)
    result = run_wrapper(
        binaries, "pinp-e", ["run", "--userns=keep-id:size=8192", "debian:latest"], env
    )
    argv = explained_argv(result)
    assert "100000:1003:7187" in argv
    assert "100000:1007:7183" in argv
    assert "8190:8190:2" in argv


def test_size_above_pool_rejected(binaries: Path, tmp_path: Path) -> None:
    env = base_env(tmp_path)
    result = run_wrapper(
        binaries, "pinp-e", ["run", "--userns=keep-id:size=70000", "debian:latest"], env
    )
    assert result.returncode != 0
    assert "exceeds the host subordinate ID pool" in result.stderr


@pytest.mark.parametrize("args", [["run", "--userns=", "debian:latest"], ["run", "--userns"]])
def test_empty_userns_rejected(binaries: Path, tmp_path: Path, args: list[str]) -> None:
    env = base_env(tmp_path)
    result = run_wrapper(binaries, "pinp-e", args, env)
    assert result.returncode != 0
    assert "conflicting arguments" in result.stderr
    assert "alias for --userns=host" in result.stderr
    assert "PINP_PASSTHROUGH=1" in result.stderr


@pytest.mark.parametrize(
    "mode",
    ["keep-id:foo=1", "keep-id:uid", "keep-id:uid=x", "keep-id:uid=-1"],
)
def test_invalid_keep_id_options_rejected(binaries: Path, tmp_path: Path, mode: str) -> None:
    env = base_env(tmp_path)
    result = run_wrapper(binaries, "pinp-e", ["run", f"--userns={mode}", "debian:latest"], env)
    assert result.returncode != 0
    assert "invalid --userns" in result.stderr


def test_keep_id_preserves_other_flags(binaries: Path, tmp_path: Path) -> None:
    env = base_env(tmp_path)
    result = run_wrapper(
        binaries,
        "pinp-e",
        ["run", "-it", "--userns=keep-id", "--name", "x", "debian:latest", "bash"],
        env,
    )
    argv = explained_argv(result)
    assert argv[-2:] == ["debian:latest", "bash"]
    assert argv[argv.index("--name") + 1] == "x"
    assert "--userns" not in argv


def test_passthrough_forwards_userns_verbatim(binaries: Path, tmp_path: Path) -> None:
    env = base_env(tmp_path)
    env["PINP_PASSTHROUGH"] = "1"
    result = run_wrapper(
        binaries, "pinp-e", ["run", "--userns=keep-id:uid=100,gid=100", "debian:latest"], env
    )
    argv = explained_argv(result)
    assert "--userns=keep-id:uid=100,gid=100" in argv


def test_config_userns_preflight_warns(binaries: Path, stub_podman: Path, tmp_path: Path) -> None:
    env = config_env(tmp_path, stub_podman)
    override = tmp_path / "userns.conf"
    override.write_text('[containers]\nuserns="keep-id"\n')
    env["CONTAINERS_CONF_OVERRIDE"] = str(override)
    result = run_wrapper(
        binaries, "pinp-i", ["pinp-check", "--json", "--no-fail", "--only", "config userns"], env
    )
    assert result.returncode == 0, result.stderr
    data = json.loads(result.stdout)
    assert len(data["checks"]) == 1
    check = data["checks"][0]
    assert check["name"] == "config userns"
    assert check["status"] in ("warn", "fail")
    assert "--uidmap" in check["detail"]


def test_env_podman_userns_preflight_warns(
    binaries: Path, stub_podman: Path, tmp_path: Path
) -> None:
    env = config_env(tmp_path, stub_podman)
    env["PODMAN_USERNS"] = "keep-id"
    result = run_wrapper(
        binaries,
        "pinp-i",
        ["pinp-check", "--json", "--no-fail", "--only", "env PODMAN_USERNS"],
        env,
    )
    assert result.returncode == 0, result.stderr
    data = json.loads(result.stdout)
    assert len(data["checks"]) == 1
    check = data["checks"][0]
    assert check["name"] == "env PODMAN_USERNS"
    assert check["status"] in ("warn", "fail")
    assert "--uidmap" in check["detail"]
