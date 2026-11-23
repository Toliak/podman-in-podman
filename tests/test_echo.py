from __future__ import annotations

from pathlib import Path

from helpers import base_env, explained_argv, run_wrapper


def _exec_env(tmp_path: Path, stub_podman: Path) -> dict[str, str]:
    env = base_env(tmp_path)
    env["PINP_EXPLAIN"] = "0"
    env["PINP_PODMAN"] = str(stub_podman)
    return env


def test_echo_default_on(binaries: Path, stub_podman: Path, tmp_path: Path) -> None:
    env = _exec_env(tmp_path, stub_podman)
    result = run_wrapper(binaries, "pinp-e", ["run", "debian:latest"], env)
    assert result.returncode == 0, result.stderr
    assert "podman run" in result.stderr
    assert "debian:latest" in result.stderr


def test_echo_disabled(binaries: Path, stub_podman: Path, tmp_path: Path) -> None:
    env = _exec_env(tmp_path, stub_podman)
    env["PINP_ECHO"] = "0"
    result = run_wrapper(binaries, "pinp-e", ["run", "debian:latest"], env)
    assert result.returncode == 0, result.stderr
    assert "podman run" not in result.stderr


def test_echo_quotes_spaces(binaries: Path, stub_podman: Path, tmp_path: Path) -> None:
    env = _exec_env(tmp_path, stub_podman)
    result = run_wrapper(binaries, "pinp-e", ["run", "debian:latest", "echo", "hello world"], env)
    assert result.returncode == 0, result.stderr
    assert "'hello world'" in result.stderr


def test_echo_escapes_single_quote(binaries: Path, stub_podman: Path, tmp_path: Path) -> None:
    env = _exec_env(tmp_path, stub_podman)
    result = run_wrapper(binaries, "pinp-e", ["ps", "it's fine"], env)
    assert result.returncode == 0, result.stderr
    expected = "'" + "it's fine".replace("'", "'\\''") + "'"
    assert expected in result.stderr


def test_echo_on_passthrough(binaries: Path, stub_podman: Path, tmp_path: Path) -> None:
    env = _exec_env(tmp_path, stub_podman)
    env["PINP_PASSTHROUGH"] = "1"
    result = run_wrapper(binaries, "pinp-e", ["run", "debian:latest"], env)
    assert result.returncode == 0, result.stderr
    assert "podman run" in result.stderr


def test_no_echo_on_explain(binaries: Path, tmp_path: Path) -> None:
    env = base_env(tmp_path)
    result = run_wrapper(binaries, "pinp-e", ["run", "debian:latest"], env)
    explained_argv(result)
    assert "podman" not in result.stderr


def test_no_echo_on_check(binaries: Path, stub_podman: Path, tmp_path: Path) -> None:
    env = _exec_env(tmp_path, stub_podman)
    result = run_wrapper(binaries, "pinp-i", ["check", "--json", "--no-fail"], env)
    assert result.returncode == 0, result.stderr
    assert "podman" not in result.stderr
