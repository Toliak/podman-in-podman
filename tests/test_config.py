from __future__ import annotations

import json
from pathlib import Path

from helpers import config_env, run_wrapper


def check_map(binaries: Path, env: dict[str, str]) -> dict[str, dict[str, str]]:
    result = run_wrapper(
        binaries, "pinp-i", ["pinp-check", "--json", "--no-fail", "--only", "config"], env
    )
    assert result.returncode == 0, result.stderr
    data = json.loads(result.stdout)
    return {check["name"]: check for check in data["checks"]}


def test_cgroups_override_trap(binaries: Path, stub_podman: Path, tmp_path: Path) -> None:
    env = config_env(tmp_path, stub_podman)
    checks = check_map(binaries, env)
    assert checks["config cgroups"]["status"] == "pass"
    assert "enabled" in checks["config cgroups"]["detail"]
    assert checks["config netns"]["status"] == "pass"
    assert checks["config ipcns"]["status"] == "pass"
    assert checks["config runtime"]["status"] == "pass"


def test_containers_conf_override_loaded_last(
    binaries: Path, stub_podman: Path, tmp_path: Path
) -> None:
    env = config_env(tmp_path, stub_podman)
    override = tmp_path / "override.conf"
    override.write_text('[containers]\ncgroups="disabled"\n')
    env["CONTAINERS_CONF_OVERRIDE"] = str(override)
    result = run_wrapper(
        binaries,
        "pinp-i",
        ["pinp-check", "--json", "--no-fail", "--only", "config cgroups"],
        env,
    )
    assert result.returncode == 0, result.stderr
    data = json.loads(result.stdout)
    assert data["checks"][0]["status"] == "warn"
    assert "disabled" in data["checks"][0]["detail"]


def test_containers_conf_ignores_other_main_files(
    binaries: Path, stub_podman: Path, tmp_path: Path
) -> None:
    env = config_env(tmp_path, stub_podman)
    single = tmp_path / "single.conf"
    single.write_text('[containers]\ncgroups="enabled"\n')
    env["CONTAINERS_CONF"] = str(single)
    checks = check_map(binaries, env)
    assert checks["config cgroups"]["status"] == "pass"
    assert checks["config netns"]["status"] == "warn"


def test_dropins_are_sorted_and_override_main(
    binaries: Path, stub_podman: Path, tmp_path: Path
) -> None:
    env = config_env(tmp_path, stub_podman)
    dropin = Path(env["XDG_CONFIG_HOME"]) / "containers" / "containers.conf.d"
    dropin.mkdir(parents=True)
    (dropin / "20.conf").write_text('[containers]\ncgroups="enabled"\n')
    (dropin / "10.conf").write_text('[containers]\ncgroups="disabled"\n')
    checks = check_map(binaries, env)
    assert checks["config cgroups"]["status"] == "pass"
    assert "enabled" in checks["config cgroups"]["detail"]


def test_storage_config_is_read(binaries: Path, stub_podman: Path, tmp_path: Path) -> None:
    env = config_env(tmp_path, stub_podman)
    storage = tmp_path / "storage.conf"
    storage.write_text(
        '[storage]\ndriver = "overlay"\n'
        '[storage.options]\nadditionalimagestores = ["/var/lib/shared"]\n'
        '[storage.options.overlay]\nmount_program = "/usr/bin/fuse-overlayfs"\n'
    )
    env["CONTAINERS_STORAGE_CONF"] = str(storage)
    checks = check_map(binaries, env)
    assert checks["config driver"]["status"] == "pass"
    assert checks["config mount_program"]["status"] == "pass"
    assert checks["config additionalimagestores"]["status"] == "pass"
