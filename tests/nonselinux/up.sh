#!/usr/bin/env bash
set -euo pipefail
source "$(dirname "$0")/lib.sh"

# Bring the non-SELinux guest up on a fresh COW overlay so prepared.qcow2 stays
# pristine. Used by run-matrix.sh and by the pytest fixture. Optional arg: overlay
# path (defaults to a throwaway overlay under RUN_CACHE).
overlay="${1:-${RUN_CACHE}/pytest.qcow2}"

"${SEL_DIR}/../selinux/prepare-image.sh"
"${SEL_DIR}/prepare-guest.sh"
bash "${SEL_DIR}/boot.sh" stop

rm -f "$overlay"
qemu-img create -f qcow2 -F qcow2 -b "${RUN_CACHE}/prepared.qcow2" "$overlay"
"${SEL_DIR}/boot.sh" start "$overlay"
"${SEL_DIR}/push-binaries.sh"
log "guest is up on ${overlay}"
