from __future__ import annotations

from pathlib import Path

import pytest

from helpers import base_env, explained_argv, run_wrapper


def test_label_present_when_selinux_on(binaries: Path, tmp_path: Path) -> None:
    env = base_env(tmp_path)
    env["PINP_SELINUX"] = "on"
    result = run_wrapper(binaries, "pinp-e", ["run", "debian:latest"], env)
    argv = explained_argv(result)
    assert "label=disable" in argv
    assert argv[argv.index("--security-opt") + 1] == "label=disable"


def test_label_absent_when_selinux_off(binaries: Path, tmp_path: Path) -> None:
    env = base_env(tmp_path)
    env["PINP_SELINUX"] = "off"
    result = run_wrapper(binaries, "pinp-e", ["run", "debian:latest"], env)
    assert "label=disable" not in explained_argv(result)


def test_auto_selinux_injects_when_fs_present(binaries: Path, tmp_path: Path) -> None:
    env = base_env(tmp_path)
    selinuxfs = tmp_path / "selinuxfs"
    selinuxfs.mkdir()
    env["PINP_SELINUX"] = "auto"
    env["PINP_SELINUXFS"] = str(selinuxfs)
    result = run_wrapper(binaries, "pinp-e", ["run", "debian:latest"], env)
    assert "label=disable" in explained_argv(result)


def test_auto_selinux_absent_when_fs_missing(binaries: Path, tmp_path: Path) -> None:
    env = base_env(tmp_path)
    result = run_wrapper(binaries, "pinp-e", ["run", "debian:latest"], env)
    assert "label=disable" not in explained_argv(result)


def test_config_selinux_overridden_by_env(binaries: Path, tmp_path: Path) -> None:
    env = base_env(tmp_path)
    cfg = tmp_path / "pinp.toml"
    cfg.write_text('[host]\nselinux = "on"\n')
    env["PINP_CONFIG"] = str(cfg)
    env["PINP_SELINUX"] = "off"
    result = run_wrapper(binaries, "pinp-e", ["run", "debian:latest"], env)
    assert "label=disable" not in explained_argv(result)


@pytest.mark.parametrize(
    "opt_args",
    [
        ["--security-opt", "label=disable"],
        ["--security-opt=label=disable"],
        ["--security-opt", "label=type:container_t"],
    ],
)
def test_security_opt_label_conflict_when_injection_on(
    binaries: Path, tmp_path: Path, opt_args: list[str]
) -> None:
    env = base_env(tmp_path)
    env["PINP_SELINUX"] = "on"
    result = run_wrapper(binaries, "pinp-e", ["run", *opt_args, "debian:latest"], env)
    assert result.returncode != 0
    assert "conflicting arguments" in result.stderr


def test_security_opt_label_allowed_when_injection_off(binaries: Path, tmp_path: Path) -> None:
    env = base_env(tmp_path)
    env["PINP_SELINUX"] = "off"
    result = run_wrapper(
        binaries, "pinp-e", ["run", "--security-opt", "label=disable", "debian:latest"], env
    )
    argv = explained_argv(result)
    assert argv.count("--security-opt") == 1
    assert "label=disable" in argv


def test_pinp_i_does_not_reject_security_opt(binaries: Path, tmp_path: Path) -> None:
    env = base_env(tmp_path)
    result = run_wrapper(
        binaries, "pinp-i", ["run", "--security-opt", "label=disable", "debian:latest"], env
    )
    assert "label=disable" in explained_argv(result)
