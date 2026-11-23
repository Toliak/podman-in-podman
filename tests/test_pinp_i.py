from __future__ import annotations

import json
from pathlib import Path

import pytest

from helpers import base_env, config_env, explained_argv, run_wrapper


def test_run_injection(binaries: Path, tmp_path: Path) -> None:
    env = base_env(tmp_path)
    result = run_wrapper(binaries, "pinp-i", ["run", "debian:trixie", "echo", "hi"], env)
    assert explained_argv(result) == [
        "podman",
        "run",
        "--uidmap",
        "0:0:1500",
        "--uidmap",
        "65534:1500:2",
        "debian:trixie",
        "echo",
        "hi",
    ]


def test_create_injection(binaries: Path, tmp_path: Path) -> None:
    env = base_env(tmp_path)
    result = run_wrapper(binaries, "pinp-i", ["create", "debian:trixie"], env)
    argv = explained_argv(result)
    assert argv[1] == "create"
    assert argv.count("--uidmap") == 2
    assert argv[-1] == "debian:trixie"


def test_custom_inner_maps(binaries: Path, tmp_path: Path) -> None:
    env = base_env(tmp_path)
    env["PINP_INNER_UIDMAPS"] = "0:0:100 65534:100:2"
    result = run_wrapper(binaries, "pinp-i", ["run", "debian:trixie"], env)
    assert explained_argv(result) == [
        "podman",
        "run",
        "--uidmap",
        "0:0:100",
        "--uidmap",
        "65534:100:2",
        "debian:trixie",
    ]


def test_build_passthrough_with_spike_hint(binaries: Path, tmp_path: Path) -> None:
    env = base_env(tmp_path, sparse=True)
    result = run_wrapper(binaries, "pinp-i", ["build", "-t", "x", "."], env)
    argv = explained_argv(result)
    assert argv == ["podman", "build", "-t", "x", "."]
    assert "--uidmap" not in argv
    assert "spike" in result.stderr.lower()
    assert "userns-uid-map" in result.stderr


def test_build_without_sparse_ranges_is_quiet(binaries: Path, tmp_path: Path) -> None:
    env = base_env(tmp_path)
    result = run_wrapper(binaries, "pinp-i", ["build", "-t", "x", "."], env)
    assert result.returncode == 0
    assert "spike" not in result.stderr.lower()


@pytest.mark.parametrize(
    "args",
    [["ps"], ["images"], ["pull", "debian"], ["version"], ["info"], []],
)
def test_passthrough_subcommands(binaries: Path, tmp_path: Path, args: list[str]) -> None:
    env = base_env(tmp_path)
    result = run_wrapper(binaries, "pinp-i", args, env)
    assert explained_argv(result) == ["podman", *args]


def test_skip_checks_execs_podman(binaries: Path, stub_podman: Path, tmp_path: Path) -> None:
    env = base_env(tmp_path)
    env["PINP_EXPLAIN"] = "0"
    env["PINP_SKIP_CHECKS"] = "1"
    env["PINP_PODMAN"] = str(stub_podman)
    result = run_wrapper(binaries, "pinp-i", ["run", "debian:trixie"], env)
    assert result.returncode == 0, result.stderr


def test_check_json_structure(binaries: Path, stub_podman: Path, tmp_path: Path) -> None:
    env = base_env(tmp_path)
    env["PINP_EXPLAIN"] = "0"
    env["PINP_PODMAN"] = str(stub_podman)
    result = run_wrapper(binaries, "pinp-i", ["check", "--json", "--no-fail"], env)
    assert result.returncode == 0, result.stderr
    data = json.loads(result.stdout)
    assert set(data) == {"checks", "summary"}
    assert set(data["summary"]) == {"pass", "warn", "fail"}
    counts = {"pass": 0, "warn": 0, "fail": 0}
    for check in data["checks"]:
        assert set(check) == {"name", "status", "detail"}
        assert check["status"] in counts
        counts[check["status"]] += 1
    assert counts == data["summary"]
    names = {check["name"] for check in data["checks"]}
    assert "config cgroups" in names
    assert "newuidmap capabilities" in names
    assert "build readiness" in names


def test_check_text_output(binaries: Path, stub_podman: Path, tmp_path: Path) -> None:
    env = base_env(tmp_path)
    env["PINP_EXPLAIN"] = "0"
    env["PINP_PODMAN"] = str(stub_podman)
    result = run_wrapper(binaries, "pinp-i", ["check", "--no-fail"], env)
    assert result.returncode == 0, result.stderr
    assert "config cgroups" in result.stdout
    assert "pass" in result.stdout.lower()


def test_check_strict_fails_on_warnings(binaries: Path, stub_podman: Path, tmp_path: Path) -> None:
    env = base_env(tmp_path)
    env["PINP_EXPLAIN"] = "0"
    env["PINP_PODMAN"] = str(stub_podman)
    result = run_wrapper(binaries, "pinp-i", ["check", "--json", "--strict"], env)
    assert result.returncode != 0


def test_check_only_filters(binaries: Path, stub_podman: Path, tmp_path: Path) -> None:
    env = config_env(tmp_path, stub_podman)
    result = run_wrapper(
        binaries, "pinp-i", ["check", "--json", "--no-fail", "--only", "config cgroups"], env
    )
    assert result.returncode == 0, result.stderr
    data = json.loads(result.stdout)
    assert len(data["checks"]) == 1
    assert data["checks"][0]["name"] == "config cgroups"
    assert data["summary"] == {"pass": 1, "warn": 0, "fail": 0}
