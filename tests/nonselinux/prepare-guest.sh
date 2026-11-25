#!/usr/bin/env bash
set -euo pipefail
source "$(dirname "$0")/lib.sh"

[[ -f "${CACHE_DIR}/baked.qcow2" ]] || die "run prepare-image.sh first"
[[ -f "${CACHE_DIR}/vm.env" ]] || die "run prepare-image.sh first"
[[ -f "${REPO_DIR}/bin/pinp-e" && -f "${REPO_DIR}/bin/pinp-i" ]] || die "run make build first"

if [[ -f "${RUN_CACHE}/prepared.qcow2" ]]; then
  log "prepared.qcow2 already exists"
  exit 0
fi

trap 'vm_stop' EXIT
bash "${SEL_DIR}/boot.sh" stop

log "creating prep overlay"
rm -f "${RUN_CACHE}/prep.qcow2"
qemu-img create -f qcow2 -F qcow2 -b "${CACHE_DIR}/baked.qcow2" "${RUN_CACHE}/prep.qcow2"
"${SEL_DIR}/boot.sh" start "${RUN_CACHE}/prep.qcow2"

ssh_root 'mkdir -p /root/pinp-bin /root/pinp-scripts'
scp_push "${REPO_DIR}/bin/pinp-e" /root/pinp-bin/pinp-e
scp_push "${REPO_DIR}/bin/pinp-i" /root/pinp-bin/pinp-i
# Reuse the SELinux harness's L1 build context verbatim (Containerfile + confs).
scp_push_tree "${SEL_DIR}/../selinux/guest/image" /root/pinp-image
scp_push "${SEL_DIR}/guest/setup.sh" /root/pinp-scripts/setup.sh

ssh_root 'bash /root/pinp-scripts/setup.sh'

vm_stop
trap - EXIT
mv "${RUN_CACHE}/prep.qcow2" "${RUN_CACHE}/prepared.qcow2"
log "prepared.qcow2 ready"
