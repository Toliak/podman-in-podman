from __future__ import annotations

import json
import subprocess
from pathlib import Path

import pytest

from helpers import (
    HOST_IDENTITY_MAP,
    prepare_env,
    run_wrapper,
)

SUBUID_LINES = ["tester:1:1001", "tester:100000:64531"]
SUBGID_LINES = ["tester:1:1005", "tester:100000:64527"]


def _prepare(
    binaries: Path,
    tmp_path: Path,
    *args: str,
    env: dict[str, str] | None = None,
    subcommand: str = "pinp-prepare",
) -> subprocess.CompletedProcess[str]:
    effective = env if env is not None else prepare_env(tmp_path)
    return run_wrapper(binaries, "pinp-i", [subcommand, *args], effective)


def test_default_prints_both_files_and_caps(binaries: Path, tmp_path: Path) -> None:
    result = _prepare(binaries, tmp_path)
    assert result.returncode == 0, result.stderr
    for line in SUBUID_LINES + SUBGID_LINES:
        assert line in result.stdout
    assert "setcap cap_setuid+ep" in result.stdout
    assert "setcap cap_setgid+ep" in result.stdout
    assert "chmod -s" in result.stdout


def test_default_has_zero_side_effects(binaries: Path, tmp_path: Path) -> None:
    env = prepare_env(tmp_path)
    result = _prepare(binaries, tmp_path, env=env)
    assert result.returncode == 0, result.stderr
    assert not (tmp_path / "gen-subuid").exists()
    assert not (tmp_path / "gen-subgid").exists()
    assert not (tmp_path / "podman-called").exists()  # stub podman never invoked
    assert "warning" not in result.stderr


def test_no_echo_line_on_prepare(binaries: Path, tmp_path: Path) -> None:
    env = prepare_env(tmp_path)
    env["PINP_ECHO"] = "1"
    result = _prepare(binaries, tmp_path, env=env)
    assert result.returncode == 0, result.stderr
    assert "podman-stub" not in result.stderr


def test_explain_does_not_override_prepare(binaries: Path, tmp_path: Path) -> None:
    env = prepare_env(tmp_path)
    env["PINP_EXPLAIN"] = "1"
    result = _prepare(binaries, tmp_path, env=env)
    assert result.returncode == 0, result.stderr
    assert "tester:1:1001" in result.stdout
    assert not result.stdout.lstrip().startswith("[")


@pytest.mark.parametrize(
    "selector,uid_present,gid_present",
    [("u", True, False), ("g", False, True), ("ug", True, True)],
)
def test_gen_subxid(
    binaries: Path, tmp_path: Path, selector: str, uid_present: bool, gid_present: bool
) -> None:
    result = _prepare(binaries, tmp_path, "--gen-subxid", selector)
    assert result.returncode == 0, result.stderr
    for line in SUBUID_LINES:
        assert (line in result.stdout) is uid_present
    for line in SUBGID_LINES:
        assert (line in result.stdout) is gid_present


def test_gen_subuid_flag_only(binaries: Path, tmp_path: Path) -> None:
    result = _prepare(binaries, tmp_path, "--gen-subuid")
    assert "tester:1:1001" in result.stdout
    assert "tester:1:1005" not in result.stdout


def test_json_output(binaries: Path, tmp_path: Path) -> None:
    result = _prepare(binaries, tmp_path, "--json")
    assert result.returncode == 0, result.stderr
    data = json.loads(result.stdout)
    assert data["user"] == "tester"
    assert data["subuid"]["lines"] == SUBUID_LINES
    assert data["subgid"]["lines"] == SUBGID_LINES
    assert any("cap_setuid+ep" in c for c in data["caps"])
    assert any("cap_setgid+ep" in c for c in data["caps"])
    assert data["warnings"] == []


def test_max_size(binaries: Path, tmp_path: Path) -> None:
    result = _prepare(binaries, tmp_path, "--max-size", "32768", "--json")
    data = json.loads(result.stdout)
    assert data["subuid"]["lines"] == ["tester:1:1001", "tester:100000:32768"]
    assert data["subgid"]["lines"] == ["tester:1:1005", "tester:100000:32768"]


def test_write_writes_only_temp_files_and_applies_caps(binaries: Path, tmp_path: Path) -> None:
    env = prepare_env(tmp_path)
    result = _prepare(binaries, tmp_path, "--write", env=env)
    assert result.returncode == 0, result.stderr
    assert (tmp_path / "gen-subuid").read_text() == "\n".join(SUBUID_LINES) + "\n"
    assert (tmp_path / "gen-subgid").read_text() == "\n".join(SUBGID_LINES) + "\n"
    assert not (tmp_path / "podman-called").exists()
    log = (tmp_path / "setcap.log").read_text()
    assert "cap_setuid+ep" in log
    assert "cap_setgid+ep" in log


def test_write_clears_setuid_and_setgid(binaries: Path, tmp_path: Path) -> None:
    env = prepare_env(tmp_path)
    (tmp_path / "newuidmap").chmod(0o6755)
    _prepare(binaries, tmp_path, "--write", env=env)
    assert (tmp_path / "newuidmap").stat().st_mode & 0o6000 == 0


def test_passthrough_still_prepares(binaries: Path, tmp_path: Path) -> None:
    env = prepare_env(tmp_path)
    env["PINP_PASSTHROUGH"] = "1"
    result = _prepare(binaries, tmp_path, env=env)
    assert result.returncode == 0, result.stderr
    assert "tester:1:1001" in result.stdout
    assert not (tmp_path / "podman-called").exists()


@pytest.mark.parametrize("subcommand", ["pinp-prepare", "pinp-gen-sub", "prepare", "gen-sub"])
def test_prepare_aliases(binaries: Path, tmp_path: Path, subcommand: str) -> None:
    result = _prepare(binaries, tmp_path, subcommand=subcommand)
    assert result.returncode == 0, result.stderr
    assert "tester:1:1001" in result.stdout
    assert not (tmp_path / "podman-called").exists()


def test_host_identity_refused(binaries: Path, tmp_path: Path) -> None:
    env = prepare_env(tmp_path, uid_map=HOST_IDENTITY_MAP, gid_map=HOST_IDENTITY_MAP)
    result = _prepare(binaries, tmp_path, env=env)
    assert result.returncode == 0, result.stderr
    assert "not inside a nested userns" in result.stderr
    assert "tester:100000:64531" not in result.stdout


def test_host_identity_refused_no_write(binaries: Path, tmp_path: Path) -> None:
    env = prepare_env(tmp_path, uid_map=HOST_IDENTITY_MAP, gid_map=HOST_IDENTITY_MAP)
    result = _prepare(binaries, tmp_path, "--write", env=env)
    assert result.returncode == 0, result.stderr
    assert "--write skipped" in result.stderr
    assert not (tmp_path / "gen-subuid").exists()
    assert not (tmp_path / "gen-subgid").exists()


def test_cap_newuidmap_only(binaries: Path, tmp_path: Path) -> None:
    result = _prepare(binaries, tmp_path, "--cap-newuidmap", "--json")
    data = json.loads(result.stdout)
    assert any("cap_setuid+ep" in c for c in data["caps"])
    assert not any("cap_setgid+ep" in c for c in data["caps"])


def test_cap_newxidmap_ug(binaries: Path, tmp_path: Path) -> None:
    result = _prepare(binaries, tmp_path, "--cap-newxidmap=ug", "--json")
    data = json.loads(result.stdout)
    assert any("cap_setuid+ep" in c for c in data["caps"])
    assert any("cap_setgid+ep" in c for c in data["caps"])


def test_write_only_selected_file(binaries: Path, tmp_path: Path) -> None:
    env = prepare_env(tmp_path)
    result = _prepare(binaries, tmp_path, "--gen-subxid=u", "--write", env=env)
    assert result.returncode == 0, result.stderr
    assert (tmp_path / "gen-subuid").read_text() == "\n".join(SUBUID_LINES) + "\n"
    assert not (tmp_path / "gen-subgid").exists()


def test_write_uses_user_override_without_identity_hooks(binaries: Path, tmp_path: Path) -> None:
    env = prepare_env(tmp_path, with_identity=False)
    result = _prepare(binaries, tmp_path, "--user", "devuser", "--write", env=env)
    assert result.returncode == 0, result.stderr
    assert "warning" not in result.stderr
    assert (tmp_path / "gen-subuid").read_text() == "devuser:1:1001\ndevuser:100000:64531\n"
    assert (tmp_path / "gen-subgid").read_text() == "devuser:1:1005\ndevuser:100000:64527\n"
    assert not (tmp_path / "podman-called").exists()


def test_write_uses_pinp_user_override_without_identity_hooks(
    binaries: Path, tmp_path: Path
) -> None:
    env = prepare_env(tmp_path, with_identity=False)
    env["PINP_USER"] = "devuser"
    result = _prepare(binaries, tmp_path, "--write", env=env)
    assert result.returncode == 0, result.stderr
    assert (tmp_path / "gen-subuid").read_text() == "devuser:1:1001\ndevuser:100000:64531\n"
    assert (tmp_path / "gen-subgid").read_text() == "devuser:1:1005\ndevuser:100000:64527\n"


def test_json_derived_owner_is_consistent_without_hooks(binaries: Path, tmp_path: Path) -> None:
    env = prepare_env(tmp_path, with_identity=False)
    result = _prepare(binaries, tmp_path, "--json", env=env)
    assert result.returncode == 0, result.stderr
    data = json.loads(result.stdout)
    owner = data["user"]
    assert owner != "root"
    assert owner != ""
    assert data["subuid"]["lines"] == [f"{owner}:1:1001", f"{owner}:100000:64531"]
    assert data["subgid"]["lines"] == [f"{owner}:1:1005", f"{owner}:100000:64527"]


def test_write_warns_but_writes_root_owner(binaries: Path, tmp_path: Path) -> None:
    env = prepare_env(tmp_path)
    env["PINP_USER"] = "root"
    result = _prepare(binaries, tmp_path, "--write", env=env)
    assert result.returncode == 0, result.stderr
    assert "owner resolves to root" in result.stderr
    assert (tmp_path / "gen-subuid").read_text().startswith("root:")
    assert (tmp_path / "gen-subgid").read_text().startswith("root:")


def _storage_conf(tmp_path: Path) -> Path:
    base = tmp_path / "home" / ".config" / "containers"
    return base / "storage.conf.d" / "pinp-shared-store.conf"


def test_default_prints_storage_conf(binaries: Path, tmp_path: Path) -> None:
    result = _prepare(binaries, tmp_path)
    assert result.returncode == 0, result.stderr
    assert str(_storage_conf(tmp_path)) in result.stdout
    assert "additionalimagestores = [" in result.stdout
    assert '"/var/lib/shared"' in result.stdout


def test_gen_storage_conf_only(binaries: Path, tmp_path: Path) -> None:
    result = _prepare(binaries, tmp_path, "--gen-storage-conf")
    assert result.returncode == 0, result.stderr
    assert '"/var/lib/shared"' in result.stdout
    assert "tester:1:1001" not in result.stdout
    assert "tester:1:1005" not in result.stdout


def test_gen_subuid_only_omits_storage_conf(binaries: Path, tmp_path: Path) -> None:
    result = _prepare(binaries, tmp_path, "--gen-subuid")
    assert result.returncode == 0, result.stderr
    assert "tester:1:1001" in result.stdout
    assert "additionalimagestores" not in result.stdout


def test_json_output_has_storage_conf(binaries: Path, tmp_path: Path) -> None:
    result = _prepare(binaries, tmp_path, "--json")
    assert result.returncode == 0, result.stderr
    data = json.loads(result.stdout)
    assert data["storage_conf"]["file"] == str(_storage_conf(tmp_path))
    assert any('"/var/lib/shared"' in line for line in data["storage_conf"]["lines"])
    assert data["info"] == []


def test_write_creates_storage_conf(binaries: Path, tmp_path: Path) -> None:
    env = prepare_env(tmp_path)
    result = _prepare(binaries, tmp_path, "--write", env=env)
    assert result.returncode == 0, result.stderr
    conf = _storage_conf(tmp_path)
    assert conf.exists()
    content = conf.read_text()
    assert "[storage.options]" in content
    assert '"/var/lib/shared"' in content
    assert "warning" not in result.stderr


def test_write_storage_conf_idempotent(binaries: Path, tmp_path: Path) -> None:
    env = prepare_env(tmp_path)
    _prepare(binaries, tmp_path, "--write", env=env)
    first = _storage_conf(tmp_path).read_text()
    result = _prepare(binaries, tmp_path, "--write", env=env)
    assert result.returncode == 0, result.stderr
    assert _storage_conf(tmp_path).read_text() == first


def test_storage_conf_skipped_when_unmounted(binaries: Path, tmp_path: Path) -> None:
    env = prepare_env(tmp_path, shared_mounted=False)
    result = _prepare(binaries, tmp_path, "--write", env=env)
    assert result.returncode == 0, result.stderr
    assert not _storage_conf(tmp_path).exists()
    assert "info:" in result.stderr
    assert "not a mount point" in result.stderr
    assert "warning" not in result.stderr


def test_storage_conf_union_preserves_existing(binaries: Path, tmp_path: Path) -> None:
    env = prepare_env(tmp_path)
    existing = tmp_path / "etc" / "storage.conf"
    existing.write_text('[storage.options]\nadditionalimagestores=["/other/store"]\n')
    result = _prepare(binaries, tmp_path, "--write", env=env)
    assert result.returncode == 0, result.stderr
    content = _storage_conf(tmp_path).read_text()
    assert '"/other/store"' in content
    assert '"/var/lib/shared"' in content
