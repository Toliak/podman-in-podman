#!/usr/bin/env bash
set -euo pipefail
source "$(dirname "$0")/lib.sh"

# Manual harness: prepare the guest, run the golden-path case on a fresh COW
# overlay, collect artifacts and render RESULTS.md. The pytest entry point
# (test_nonselinux.py) drives the same guest scripts through a session fixture.
"${SEL_DIR}/../selinux/prepare-image.sh"
"${SEL_DIR}/prepare-guest.sh"
bash "${SEL_DIR}/boot.sh" stop

RESULTS_DIR="${SEL_DIR}/results"
mkdir -p "$RESULTS_DIR"

CASES=( A )

for name in "${CASES[@]}"; do
  log "case ${name}"
  overlay="${RUN_CACHE}/case-${name}.qcow2"
  rm -f "$overlay"
  qemu-img create -f qcow2 -F qcow2 -b "${RUN_CACHE}/prepared.qcow2" "$overlay"
  "${SEL_DIR}/boot.sh" start "$overlay"
  "${SEL_DIR}/push-binaries.sh"
  "${SEL_DIR}/collect-case.sh" "$name" "${RESULTS_DIR}/${name}"
  "${SEL_DIR}/boot.sh" stop
  rm -f "$overlay"
done

log "results in ${RESULTS_DIR}"
"${SEL_DIR}/summarize.sh"
