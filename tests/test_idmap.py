from __future__ import annotations

from pathlib import Path

import pytest

from helpers import base_env, explained_argv, run_wrapper


@pytest.mark.parametrize(
    ("uid", "gid", "uid_extent", "gid_extent"),
    [
        ("1002", "1006", "100000:1003:64531", "100000:1007:64527"),
        ("2000", "3000", "100000:2001:63533", "100000:3001:62533"),
        ("1", "1", "100000:2:65532", "100000:2:65532"),
    ],
)
def test_outer_map_math(
    binaries: Path,
    tmp_path: Path,
    uid: str,
    gid: str,
    uid_extent: str,
    gid_extent: str,
) -> None:
    env = base_env(tmp_path)
    env["PINP_UID"] = uid
    env["PINP_GID"] = gid
    result = run_wrapper(binaries, "pinp-e", ["run", "debian:latest"], env)
    argv = explained_argv(result)
    assert f"{uid}:0:1" in argv
    assert f"0:1:{uid}" in argv
    assert uid_extent in argv
    assert f"{gid}:0:1" in argv
    assert f"0:1:{gid}" in argv
    assert gid_extent in argv


def test_uid_and_gid_maps_stay_separate(binaries: Path, tmp_path: Path) -> None:
    env = base_env(tmp_path)
    result = run_wrapper(binaries, "pinp-e", ["run", "debian:latest"], env)
    argv = explained_argv(result)
    assert argv.count("--uidmap") == 4
    assert argv.count("--gidmap") == 4
    assert argv[argv.index("--uidmap") + 1] == "1002:0:1"
    assert argv[argv.index("--gidmap") + 1] == "1006:0:1"
