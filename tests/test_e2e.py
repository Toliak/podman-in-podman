from __future__ import annotations

import os
import shutil
import subprocess
from pathlib import Path

import pytest

from helpers import assert_portable_checks, base_env, run_wrapper


@pytest.mark.e2e
def test_podman_run_without_selinux_label() -> None:
    if os.environ.get("PINP_E2E") != "1":
        pytest.skip("set PINP_E2E=1 to run e2e tests")
    podman = shutil.which("podman")
    if podman is None:
        pytest.skip("podman is not on PATH")
    if os.path.exists("/sys/fs/selinux"):
        pytest.skip("host has SELinux; this test targets non-SELinux hosts")
    result = subprocess.run(
        [podman, "run", "--rm", "docker.io/library/alpine:latest", "true"],
        capture_output=True,
        text=True,
        check=False,
    )
    assert result.returncode == 0, result.stderr
    assert "--security-opt" not in result.args


@pytest.mark.e2e
def test_portable_checks_without_selinux(tmp_path: Path, binaries: Path, stub_podman: Path) -> None:
    if os.environ.get("PINP_E2E") != "1":
        pytest.skip("set PINP_E2E=1 to run e2e tests")
    env = base_env(tmp_path)
    env["PINP_PODMAN"] = str(stub_podman)

    def run(argv: list[str], extra_env: dict[str, str]) -> subprocess.CompletedProcess[str]:
        return run_wrapper(binaries, Path(argv[0]).name, argv[1:], extra_env)

    assert_portable_checks(run, env, str(binaries / "pinp-e"), str(binaries / "pinp-i"))
