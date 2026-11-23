from __future__ import annotations

import os
import subprocess
from pathlib import Path

import pytest

pytestmark = [
    pytest.mark.selinux,
    pytest.mark.skipif(
        os.environ.get("PINP_SELINUX_TEST") != "1",
        reason="set PINP_SELINUX_TEST=1 to run the QEMU/SELinux matrix",
    ),
]

REPO_ROOT = Path(__file__).resolve().parents[2]
SEL_DIR = Path(__file__).resolve().parent
RUN_MATRIX = SEL_DIR / "run-matrix.sh"


def test_selinux_matrix() -> None:
    for name in ("pinp-e", "pinp-i"):
        if not (REPO_ROOT / "bin" / name).is_file():
            pytest.fail("run 'make build' first (or use make selinux-test)")

    env = os.environ.copy()
    env["PINP_SELINUX_TEST"] = "1"

    result: subprocess.CompletedProcess[str] = subprocess.run(
        ["bash", str(RUN_MATRIX)],
        cwd=REPO_ROOT,
        env=env,
        check=False,
    )

    assert result.returncode == 0

    results_md = SEL_DIR / "RESULTS.md"
    assert results_md.is_file()
    assert "label=disable" in results_md.read_text()

    for name in "ABCDEFGH":
        assert (SEL_DIR / "results" / name / "meta.txt").is_file()
