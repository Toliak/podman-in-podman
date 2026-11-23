from __future__ import annotations

from pathlib import Path

from helpers import base_env, explained_argv, run_wrapper


def test_wrapper_config_podman_over_default(binaries: Path, tmp_path: Path) -> None:
    env = base_env(tmp_path)
    cfg = tmp_path / "pinp.toml"
    cfg.write_text('[wrapper]\npodman = "custom-podman"\n')
    env["PINP_CONFIG"] = str(cfg)
    result = run_wrapper(binaries, "pinp-e", ["run", "debian:latest"], env)
    assert explained_argv(result)[0] == "custom-podman"


def test_wrapper_config_overridden_by_env(binaries: Path, tmp_path: Path) -> None:
    env = base_env(tmp_path)
    cfg = tmp_path / "pinp.toml"
    cfg.write_text('[wrapper]\npodman = "custom-podman"\n')
    env["PINP_CONFIG"] = str(cfg)
    env["PINP_PODMAN"] = "env-podman"
    result = run_wrapper(binaries, "pinp-e", ["run", "debian:latest"], env)
    assert explained_argv(result)[0] == "env-podman"


def test_wrapper_config_default_path(binaries: Path, tmp_path: Path) -> None:
    env = base_env(tmp_path)
    cfg_dir = tmp_path / "home" / ".config" / "pinp"
    cfg_dir.mkdir(parents=True)
    (cfg_dir / "config.toml").write_text('[wrapper]\npodman = "home-podman"\n')
    result = run_wrapper(binaries, "pinp-e", ["run", "debian:latest"], env)
    assert explained_argv(result)[0] == "home-podman"


def test_wrapper_config_xdg_path(binaries: Path, tmp_path: Path) -> None:
    env = base_env(tmp_path)
    xdg = tmp_path / "xdg"
    cfg_dir = xdg / "pinp"
    cfg_dir.mkdir(parents=True)
    (cfg_dir / "config.toml").write_text('[wrapper]\npodman = "xdg-podman"\n')
    env["XDG_CONFIG_HOME"] = str(xdg)
    result = run_wrapper(binaries, "pinp-e", ["run", "debian:latest"], env)
    assert explained_argv(result)[0] == "xdg-podman"


def test_wrapper_config_missing_is_noop(binaries: Path, tmp_path: Path) -> None:
    env = base_env(tmp_path)
    env["PINP_CONFIG"] = str(tmp_path / "does-not-exist.toml")
    result = run_wrapper(binaries, "pinp-e", ["run", "debian:latest"], env)
    assert explained_argv(result)[0] == "podman"


def test_wrapper_config_malformed_errors(binaries: Path, tmp_path: Path) -> None:
    env = base_env(tmp_path)
    cfg = tmp_path / "pinp.toml"
    cfg.write_text("[wrapper\npodman = ???\n")
    env["PINP_CONFIG"] = str(cfg)
    result = run_wrapper(binaries, "pinp-e", ["run", "debian:latest"], env)
    assert result.returncode != 0
    assert "wrapper config" in result.stderr


def test_wrapper_config_bad_type_errors(binaries: Path, tmp_path: Path) -> None:
    env = base_env(tmp_path)
    cfg = tmp_path / "pinp.toml"
    cfg.write_text('[host]\nuid = "not-an-int"\n')
    env["PINP_CONFIG"] = str(cfg)
    result = run_wrapper(binaries, "pinp-e", ["run", "debian:latest"], env)
    assert result.returncode != 0
    assert "wrapper config" in result.stderr


def test_wrapper_config_inner_uidmaps(binaries: Path, tmp_path: Path) -> None:
    env = base_env(tmp_path)
    cfg = tmp_path / "pinp.toml"
    cfg.write_text('[l1]\ninner_uidmaps = "0:0:10 65534:10:2"\n')
    env["PINP_CONFIG"] = str(cfg)
    result = run_wrapper(binaries, "pinp-i", ["run", "debian:trixie"], env)
    assert explained_argv(result) == [
        "podman",
        "run",
        "--uidmap",
        "0:0:10",
        "--uidmap",
        "65534:10:2",
        "debian:trixie",
    ]


def test_wrapper_config_inner_uidmaps_env_wins(binaries: Path, tmp_path: Path) -> None:
    env = base_env(tmp_path)
    cfg = tmp_path / "pinp.toml"
    cfg.write_text('[l1]\ninner_uidmaps = "0:0:10 65534:10:2"\n')
    env["PINP_CONFIG"] = str(cfg)
    env["PINP_INNER_UIDMAPS"] = "0:0:99"
    result = run_wrapper(binaries, "pinp-i", ["run", "debian:trixie"], env)
    assert explained_argv(result) == ["podman", "run", "--uidmap", "0:0:99", "debian:trixie"]
