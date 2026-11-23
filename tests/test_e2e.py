from __future__ import annotations

import os
import shutil
import subprocess

import pytest


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
