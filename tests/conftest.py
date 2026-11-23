from __future__ import annotations

import json
import subprocess
from pathlib import Path

import pytest

from helpers import CANNED_INFO, ROOT


@pytest.fixture(scope="session")
def binaries(tmp_path_factory: pytest.TempPathFactory) -> Path:
    out = tmp_path_factory.mktemp("pinp-bin")
    for name in ("pinp-e", "pinp-i"):
        result: subprocess.CompletedProcess[str] = subprocess.run(
            ["go", "build", "-o", str(out / name), f"./cmd/{name}"],
            cwd=ROOT,
            capture_output=True,
            text=True,
            check=False,
        )
        assert result.returncode == 0, result.stderr
    return out


@pytest.fixture
def stub_podman(tmp_path: Path) -> Path:
    info = tmp_path / "info.json"
    info.write_text(json.dumps(CANNED_INFO))
    stub = tmp_path / "podman"
    stub.write_text(f'#!/bin/sh\nif [ "$1" = "info" ]; then cat "{info}"; exit 0; fi\nexit 0\n')
    stub.chmod(0o755)
    return stub
