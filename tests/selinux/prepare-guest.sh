#!/usr/bin/env bash
set -euo pipefail
source "$(dirname "$0")/lib.sh"

[[ -f "${CACHE_DIR}/baked.qcow2" ]] || die "run prepare-image.sh first"
[[ -f "${CACHE_DIR}/vm.env" ]] || die "run prepare-image.sh first"
[[ -f "${REPO_DIR}/bin/pinp-e" && -f "${REPO_DIR}/bin/pinp-i" ]] || die "run make build first"

if [[ -f "${CACHE_DIR}/prepared.qcow2" ]]; then
  log "prepared.qcow2 already exists"
  exit 0
fi

trap 'vm_stop' EXIT
bash "${SEL_DIR}/boot.sh" stop

log "creating prep overlay"
if [[ ! -f "${CACHE_DIR}/prep.qcow2" ]]; then
  qemu-img create -f qcow2 -F qcow2 -b "${CACHE_DIR}/baked.qcow2" "${CACHE_DIR}/prep.qcow2"
fi
"${SEL_DIR}/boot.sh" start "${CACHE_DIR}/prep.qcow2"

ssh_root 'mkdir -p /root/pinp-bin /root/pinp-scripts'
scp_push "${REPO_DIR}/bin/pinp-e" /root/pinp-bin/pinp-e
scp_push "${REPO_DIR}/bin/pinp-i" /root/pinp-bin/pinp-i
scp_push_tree "${SEL_DIR}/guest/image" /root/pinp-image
scp_push "${SEL_DIR}/guest/setup.sh" /root/pinp-scripts/setup.sh
scp_push "${SEL_DIR}/guest/run-case.sh" /root/pinp-scripts/run-case.sh
scp_push "${SEL_DIR}/guest/collect-avc.sh" /root/pinp-scripts/collect-avc.sh

ssh_root 'bash /root/pinp-scripts/setup.sh'
ssh_root 'cp /root/pinp-scripts/run-case.sh /home/devuser/run-case.sh && chown 1002:1006 /home/devuser/run-case.sh && cp /root/pinp-scripts/collect-avc.sh /root/collect-avc.sh'

vm_stop
trap - EXIT
mv "${CACHE_DIR}/prep.qcow2" "${CACHE_DIR}/prepared.qcow2"
log "prepared.qcow2 ready"
