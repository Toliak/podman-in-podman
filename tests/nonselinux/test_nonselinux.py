from __future__ import annotations

import json
import os
import subprocess
from pathlib import Path

import pytest

from helpers import assert_portable_checks

pytestmark = [
    pytest.mark.nonselinux,
    pytest.mark.skipif(
        os.environ.get("PINP_NONSELINUX_TEST") != "1",
        reason="set PINP_NONSELINUX_TEST=1 to run the QEMU non-SELinux golden path",
    ),
]

REPO_ROOT = Path(__file__).resolve().parents[2]
NS_DIR = Path(__file__).resolve().parent
PODMAN_E = "/home/devuser/bin/pinp-e"
PODMAN_I = "/home/devuser/bin/pinp-i"


def _run_cache() -> Path:
    override = os.environ.get("PINP_NONSELINUX_CACHE")
    return Path(override) if override else REPO_ROOT / ".nonselinux-cache"


class Guest:
    def __init__(self) -> None:
        self.run_cache = _run_cache()
        self.out_dir = self.run_cache / "pytest-out"

    def run(self, argv: list[str], env: dict[str, str]) -> subprocess.CompletedProcess[str]:
        cmd = ["bash", str(NS_DIR / "vm-exec.sh")]
        for key, value in env.items():
            cmd += ["--env", f"{key}={value}"]
        cmd += ["--", *argv]
        return subprocess.run(cmd, cwd=REPO_ROOT, capture_output=True, text=True, check=False)

    def collect_golden_path(self, name: str = "golden") -> Path:
        subprocess.run(
            ["bash", str(NS_DIR / "collect-case.sh"), name, str(self.out_dir)],
            cwd=REPO_ROOT,
            check=True,
        )
        return self.out_dir


@pytest.fixture(scope="session")
def guest() -> Guest:
    for binary in ("pinp-e", "pinp-i"):
        if not (REPO_ROOT / "bin" / binary).is_file():
            pytest.fail("run 'make build' first (or use make nonselinux-test)")

    run_cache = _run_cache()
    run_cache.mkdir(parents=True, exist_ok=True)
    overlay = run_cache / "pytest.qcow2"
    try:
        subprocess.run(
            ["bash", str(NS_DIR / "up.sh"), str(overlay)],
            cwd=REPO_ROOT,
            check=True,
        )
        yield Guest()
    finally:
        subprocess.run(
            ["bash", str(NS_DIR / "boot.sh"), "stop"],
            cwd=REPO_ROOT,
            check=False,
        )
        overlay.unlink(missing_ok=True)


def test_portable_checks(guest: Guest) -> None:
    assert_portable_checks(guest.run, {"PINP_EXPLAIN": "1"}, PODMAN_E, PODMAN_I)


def test_golden_path(guest: Guest) -> None:
    out = guest.collect_golden_path("golden")

    assert (out / "selinuxfs.txt").read_text().strip() == "0", (out / "selinuxfs.txt").read_text()
    assert (out / "run.rc.txt").read_text().strip() == "0", (out / "run.stderr.txt").read_text()
    assert (out / "hello.rc.txt").read_text().strip() == "0", (out / "hello.txt").read_text()
    assert "hello world" in (out / "hello.txt").read_text()

    check = json.loads((out / "check.json").read_text())
    assert check["summary"]["fail"] == 0, (out / "check.json").read_text()

    images = (out / "images.txt").read_text().splitlines()
    assert any(line.endswith(" true") for line in images), images

    assert (out / "build.rc.txt").read_text().strip() == "0", (out / "build.txt").read_text()
