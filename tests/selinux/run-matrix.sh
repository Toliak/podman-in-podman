#!/usr/bin/env bash
set -euo pipefail
source "$(dirname "$0")/lib.sh"

"${SEL_DIR}/prepare-image.sh"
"${SEL_DIR}/prepare-guest.sh"
bash "${SEL_DIR}/boot.sh" stop

CASES=(
  "A:disable:none"
  "B:none:none"
  "C:nested:none"
  "D:nested:z"
  "E:nested:Z"
  "F:none:z"
  "G:disable:z"
  "H:disable:Z"
)

RESULTS_DIR="${SEL_DIR}/results"
mkdir -p "$RESULTS_DIR"

for spec in "${CASES[@]}"; do
  IFS=: read -r name label suffix <<< "$spec"
  log "case ${name} (label=${label}, suffix=${suffix})"
  overlay="${CACHE_DIR}/case-${name}.qcow2"
  rm -f "$overlay"
  qemu-img create -f qcow2 -F qcow2 -b "${CACHE_DIR}/prepared.qcow2" "$overlay"
  "${SEL_DIR}/boot.sh" start "$overlay"
  ssh_root 'semodule -DB' || true
  scp_push "${SEL_DIR}/guest/run-case.sh" /home/devuser/run-case.sh
  ssh_root 'chown 1002:1006 /home/devuser/run-case.sh'
  scp_push "${SEL_DIR}/guest/collect-avc.sh" /root/collect-avc.sh
  outdir="${RESULTS_DIR}/${name}"
  rm -rf "$outdir"
  mkdir -p "$outdir"
  ssh_root 'rm -rf /tmp/pinp-out && mkdir -p /tmp/pinp-out && chmod 777 /tmp/pinp-out'
  start="$(ssh_root 'date +%s' | tr -d '\r')"
  ssh_guest "bash /home/devuser/run-case.sh ${name} ${label} ${suffix} /tmp/pinp-out" || true
  ssh_root "bash /root/collect-avc.sh /tmp/pinp-out '${start}'" || true
  ssh_root 'tar -C /tmp/pinp-out -cf - .' > "${outdir}/results.tar"
  tar -C "$outdir" -mxf "${outdir}/results.tar"
  rm -f "${outdir}/results.tar"
  "${SEL_DIR}/boot.sh" stop
  rm -f "$overlay"
done

log "results in ${RESULTS_DIR}"
"${SEL_DIR}/summarize.sh"
