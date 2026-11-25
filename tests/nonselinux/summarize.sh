#!/usr/bin/env bash
set -euo pipefail
source "$(dirname "$0")/lib.sh"

RESULTS_DIR="${SEL_DIR}/results"
OUT="${SEL_DIR}/RESULTS.md"
CASES=( A )

read_val() {
  if [[ -f "$1" ]]; then
    tr -d '\r\n' < "$1"
  else
    printf '?'
  fi
}

{
  printf '# Non-SELinux nested-podman golden path\n\n'
  printf 'Guest: `%s`, direct kernel boot with `selinux=0`.\n' "$BASE_IMAGE"
  printf 'Topology: VM (non-SELinux host) -> `pinp-e run` (L1, `pinp-outer`) -> `pinp-i` -> L2.\n\n'
  printf '## Cases\n\n'
  printf '| Case | outer rc | images rc | hello rc | build rc | selinuxfs | getenforce |\n'
  printf '|---|---|---|---|---|---|---|\n'
  for name in "${CASES[@]}"; do
    d="${RESULTS_DIR}/${name}"
    printf '| %s | %s | %s | %s | %s | %s | %s |\n' "$name" \
      "$(read_val "${d}/run.rc.txt")" \
      "$(read_val "${d}/images.rc.txt")" \
      "$(read_val "${d}/hello.rc.txt")" \
      "$(read_val "${d}/build.rc.txt")" \
      "$(read_val "${d}/selinuxfs.txt")" \
      "$(read_val "${d}/getenforce.txt")"
  done
  printf '\n## Artifacts\n\n'
  printf 'Per-case evidence is in `tests/nonselinux/results/<case>/`: `run.stderr.txt`, `images.txt`, `inner-info.txt`, `hello.txt`, `build.txt`, `check.json`, `outer-labels.txt`, `getenforce.txt`, `selinuxfs.txt`.\n\n'
  printf 'This harness records a pass/fail golden path. `test_nonselinux.py` asserts the same rc files plus the shared portable checks.\n'
} > "$OUT"

log "wrote ${OUT}"
