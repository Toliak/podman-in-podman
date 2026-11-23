from __future__ import annotations

import json
from pathlib import Path

import pytest

from helpers import base_env, config_env, explained_argv, run_wrapper

USERNS_MODES = [
    "host",
    "auto",
    "nomap",
    "ns:foo",
    "keep-id:uid=1000",
    "auto:size=8192",
]


@pytest.mark.parametrize("wrapper", ["pinp-e", "pinp-i"])
@pytest.mark.parametrize("mode", USERNS_MODES)
def test_userns_modes_rejected_with_hint(
    binaries: Path, tmp_path: Path, wrapper: str, mode: str
) -> None:
    env = base_env(tmp_path)
    result = run_wrapper(binaries, wrapper, ["run", f"--userns={mode}", "debian:latest"], env)
    assert result.returncode != 0
    assert "conflicting arguments" in result.stderr
    assert "unsupported" in result.stderr
    assert "PINP_PASSTHROUGH=1" in result.stderr


@pytest.mark.parametrize("wrapper", ["pinp-e", "pinp-i"])
@pytest.mark.parametrize("userns_args", [["--userns=keep-id"], ["--userns", "keep-id"]])
def test_keep_id_is_stripped_and_maps_injected(
    binaries: Path, tmp_path: Path, wrapper: str, userns_args: list[str]
) -> None:
    env = base_env(tmp_path)
    result = run_wrapper(binaries, wrapper, ["run", *userns_args, "debian:latest"], env)
    argv = explained_argv(result)
    assert "--userns" not in argv
    assert "keep-id" not in argv
    assert argv[-1] == "debian:latest"
    if wrapper == "pinp-e":
        assert argv.count("--uidmap") == 4
        assert argv.count("--gidmap") == 4
    else:
        assert argv.count("--uidmap") == 2


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


def test_config_userns_preflight_warns(binaries: Path, stub_podman: Path, tmp_path: Path) -> None:
    env = config_env(tmp_path, stub_podman)
    override = tmp_path / "userns.conf"
    override.write_text('[containers]\nuserns="keep-id"\n')
    env["CONTAINERS_CONF_OVERRIDE"] = str(override)
    result = run_wrapper(
        binaries, "pinp-i", ["check", "--json", "--no-fail", "--only", "config userns"], env
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
        binaries, "pinp-i", ["check", "--json", "--no-fail", "--only", "env PODMAN_USERNS"], env
    )
    assert result.returncode == 0, result.stderr
    data = json.loads(result.stdout)
    assert len(data["checks"]) == 1
    check = data["checks"][0]
    assert check["name"] == "env PODMAN_USERNS"
    assert check["status"] in ("warn", "fail")
    assert "--uidmap" in check["detail"]
