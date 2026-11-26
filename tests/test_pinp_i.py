from __future__ import annotations

import json
from pathlib import Path

import pytest

from helpers import base_env, build_maps_env, config_env, explained_argv, run_wrapper


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


def test_build_injects_derived_userns_maps(binaries: Path, tmp_path: Path) -> None:
    env = build_maps_env(tmp_path)
    result = run_wrapper(binaries, "pinp-i", ["build", "-t", "x", "."], env)
    assert explained_argv(result) == [
        "podman",
        "build",
        "--userns-uid-map",
        "0:1:1001",
        "--userns-uid-map",
        "1001:1002:32766",
        "--userns-uid-map",
        "65534:33768:2",
        "--userns-gid-map",
        "0:1:1001",
        "--userns-gid-map",
        "1001:1002:32766",
        "--userns-gid-map",
        "65534:33768:2",
        "-t",
        "x",
        ".",
    ]
    assert "spike" not in result.stderr.lower()


def test_build_wide_pool_is_quiet(binaries: Path, tmp_path: Path) -> None:
    env = base_env(tmp_path)
    result = run_wrapper(binaries, "pinp-i", ["build", "-t", "x", "."], env)
    assert result.returncode == 0
    assert explained_argv(result) == ["podman", "build", "-t", "x", "."]
    assert "--userns-uid-map" not in " ".join(explained_argv(result))
    assert "spike" not in result.stderr.lower()


def test_pinp_build_maps_off(binaries: Path, tmp_path: Path) -> None:
    env = build_maps_env(tmp_path)
    env["PINP_BUILD_MAPS"] = "0"
    result = run_wrapper(binaries, "pinp-i", ["build", "-t", "x", "."], env)
    assert explained_argv(result) == ["podman", "build", "-t", "x", "."]


@pytest.mark.parametrize(
    "args",
    [
        ["--userns-uid-map", "0:1:1001"],
        ["--userns=host"],
        ["--userns-gid-map-group=devuser"],
        ["--isolation=chroot"],
    ],
)
def test_build_conflicting_userns_flags_abort(
    binaries: Path, tmp_path: Path, args: list[str]
) -> None:
    env = build_maps_env(tmp_path)
    result = run_wrapper(binaries, "pinp-i", ["build", *args, "-t", "x", "."], env)
    assert result.returncode != 0
    assert "conflicting arguments already supplied" in result.stderr
    assert "PINP_PASSTHROUGH" in result.stderr


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
    result = run_wrapper(binaries, "pinp-i", ["pinp-check", "--json", "--no-fail"], env)
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


def test_check_build_readiness_reports_injected_maps(
    binaries: Path, stub_podman: Path, tmp_path: Path
) -> None:
    env = build_maps_env(tmp_path)
    env["PINP_EXPLAIN"] = "0"
    env["PINP_PODMAN"] = str(stub_podman)
    result = run_wrapper(
        binaries, "pinp-i", ["pinp-check", "--json", "--no-fail", "--only", "build readiness"], env
    )
    assert result.returncode == 0, result.stderr
    data = json.loads(result.stdout)
    assert len(data["checks"]) == 1
    check = data["checks"][0]
    assert check["status"] == "pass", check
    assert "--userns-uid-map 0:1:1001" in check["detail"]


def test_check_text_output(binaries: Path, stub_podman: Path, tmp_path: Path) -> None:
    env = base_env(tmp_path)
    env["PINP_EXPLAIN"] = "0"
    env["PINP_PODMAN"] = str(stub_podman)
    result = run_wrapper(binaries, "pinp-i", ["pinp-check", "--no-fail"], env)
    assert result.returncode == 0, result.stderr
    assert "config cgroups" in result.stdout
    assert "pass" in result.stdout.lower()


def test_check_strict_fails_on_warnings(binaries: Path, stub_podman: Path, tmp_path: Path) -> None:
    env = base_env(tmp_path)
    env["PINP_EXPLAIN"] = "0"
    env["PINP_PODMAN"] = str(stub_podman)
    result = run_wrapper(binaries, "pinp-i", ["pinp-check", "--json", "--strict"], env)
    assert result.returncode != 0


def test_check_only_filters(binaries: Path, stub_podman: Path, tmp_path: Path) -> None:
    env = config_env(tmp_path, stub_podman)
    result = run_wrapper(
        binaries, "pinp-i", ["pinp-check", "--json", "--no-fail", "--only", "config cgroups"], env
    )
    assert result.returncode == 0, result.stderr
    data = json.loads(result.stdout)
    assert len(data["checks"]) == 1
    assert data["checks"][0]["name"] == "config cgroups"
    assert data["summary"] == {"pass": 1, "warn": 0, "fail": 0}


def test_bare_check_is_forwarded_to_podman(binaries: Path, tmp_path: Path) -> None:
    env = base_env(tmp_path)  # PINP_EXPLAIN=1
    result = run_wrapper(binaries, "pinp-i", ["check", "--json", "--no-fail"], env)
    assert result.returncode == 0, result.stderr
    # bare "check" is a clean pass-through, never pinp's preflight report
    assert explained_argv(result) == ["podman", "check", "--json", "--no-fail"]


def test_unknown_pinp_subcommand_is_rejected(binaries: Path, tmp_path: Path) -> None:
    env = base_env(tmp_path)
    result = run_wrapper(binaries, "pinp-i", ["pinp-frobnicate"], env)
    assert result.returncode != 0
    assert "unknown pinp subcommand" in result.stderr
    assert "pinp-check" in result.stderr


def test_pinp_check_dispatched_even_in_passthrough(
    binaries: Path, stub_podman: Path, tmp_path: Path
) -> None:
    env = base_env(tmp_path)
    env["PINP_EXPLAIN"] = "0"
    env["PINP_PODMAN"] = str(stub_podman)
    env["PINP_PASSTHROUGH"] = "1"
    result = run_wrapper(binaries, "pinp-i", ["pinp-check", "--json", "--no-fail"], env)
    assert result.returncode == 0, result.stderr
    assert "checks" in json.loads(result.stdout)
