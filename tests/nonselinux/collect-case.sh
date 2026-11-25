#!/usr/bin/env bash
set -euo pipefail
source "$(dirname "$0")/lib.sh"

# Run the golden-path case on the already-booted guest and pull its artifacts
# back. Used by the pytest fixture (which manages the VM lifecycle) and by
# run-matrix.sh. Usage: collect-case.sh <name> <outdir>
name="${1:-golden}"
outdir="${2:-${RUN_CACHE}/pytest-out}"

scp_push "${SEL_DIR}/guest/run-case.sh" /home/devuser/run-case.sh
ssh_root 'chown 1002:1006 /home/devuser/run-case.sh'
ssh_root 'rm -rf /tmp/pinp-out && mkdir -p /tmp/pinp-out && chmod 777 /tmp/pinp-out'
ssh_guest "bash /home/devuser/run-case.sh '${name}' /tmp/pinp-out" || true

rm -rf "$outdir"
mkdir -p "$outdir"
ssh_root 'tar -C /tmp/pinp-out -cf - .' > "${outdir}/results.tar"
tar -C "$outdir" -mxf "${outdir}/results.tar"
rm -f "${outdir}/results.tar"
log "artifacts in ${outdir}"
