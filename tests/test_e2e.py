from __future__ import annotations

import json
import os
import pwd
import shutil
import subprocess
from pathlib import Path

import pytest

from helpers import ROOT, assert_portable_checks, base_env, run_wrapper

CONTEXT_DIR = Path(__file__).resolve().parent / "e2e-build"
L1_IMAGE_CONTEXT = Path(__file__).resolve().parent / "selinux" / "guest" / "image"


def _subid_total(path: str, user: str) -> int:
    try:
        lines = Path(path).read_text().splitlines()
    except OSError:
        return 0
    total = 0
    for line in lines:
        parts = line.split(":")
        if len(parts) == 3 and parts[0] == user:
            try:
                total += int(parts[2])
            except ValueError:
                continue
    return total


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


@pytest.mark.e2e
def test_pinp_i_build_and_run_hello_in_l1(binaries: Path) -> None:
    if os.environ.get("PINP_E2E_BUILD") != "1":
        pytest.skip("set PINP_E2E_BUILD=1 to run the nested L1 build e2e test")
    podman = shutil.which("podman")
    if podman is None:
        pytest.skip("podman is not on PATH")
    user = pwd.getpwuid(os.getuid()).pw_name
    for path in ("/etc/subuid", "/etc/subgid"):
        if _subid_total(path, user) < 65536:
            pytest.skip(f"{user} needs >= 65536 subordinate IDs in {path}")

    suffix = str(os.getpid())
    cname = f"pinp-e2e-l1-{suffix}"
    l1_tag = f"pinp-e2e-l1-image-{suffix}"
    tag = f"pinp-e2e-hello-{suffix}"
    l1_image = os.environ.get("PINP_E2E_L1_IMAGE") or l1_tag
    built_l1 = l1_image == l1_tag

    host_env = os.environ.copy()
    host_env.update(
        {
            "PINP_PODMAN": podman,
            "PINP_EXPLAIN": "0",
            "PINP_ECHO": "0",
        }
    )

    def podman_host(*args: str) -> subprocess.CompletedProcess[str]:
        return subprocess.run(
            [podman, *args], cwd=ROOT, env=host_env, capture_output=True, text=True, check=False
        )

    def l1(*args: str) -> subprocess.CompletedProcess[str]:
        return podman_host("exec", cname, *args)

    try:
        if built_l1:
            result = podman_host("build", "-t", l1_tag, str(L1_IMAGE_CONTEXT))
            assert result.returncode == 0, result.stderr

        result = run_wrapper(
            binaries,
            "pinp-e",
            [
                "run",
                "-d",
                "--name",
                cname,
                "--userns=keep-id",
                "-v",
                f"{CONTEXT_DIR}:/opt/workdir",
                l1_image,
                "sleep",
                "infinity",
            ],
            host_env,
        )
        assert result.returncode == 0, result.stderr

        result = podman_host("cp", str(binaries / "pinp-i"), f"{cname}:/tmp/pinp-i")
        assert result.returncode == 0, result.stderr
        result = l1("chmod", "755", "/tmp/pinp-i")
        assert result.returncode == 0, result.stderr

        result = l1("/tmp/pinp-i", "pinp-check", "--json", "--no-fail")
        assert result.returncode == 0, result.stderr
        report = json.loads(result.stdout)
        assert report["summary"]["fail"] == 0, result.stdout

        result = l1("/tmp/pinp-i", "build", "-t", tag, "/opt/workdir")
        assert result.returncode == 0, result.stderr

        result = l1("/tmp/pinp-i", "run", "--rm", tag)
        assert result.returncode == 0, result.stderr
        assert "hello world" in result.stdout

        result = l1("/tmp/pinp-i", "run", "--rm", tag, "id", "-u")
        assert result.returncode == 0, result.stderr
        assert result.stdout.strip() == "1000"
    finally:
        podman_host("exec", cname, "podman", "rmi", "-f", tag)
        podman_host("rm", "-f", cname)
        if built_l1:
            podman_host("rmi", "-f", l1_tag)
