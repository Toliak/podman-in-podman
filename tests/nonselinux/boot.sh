#!/usr/bin/env bash
set -euo pipefail
source "$(dirname "$0")/lib.sh"

# The shared boot.sh honours PINP_VM_KERNEL_EXTRA / PINP_VM_PIDFILE / PINP_VM_SERIAL
# / PINP_VM_SSH_PORT (exported by lib.sh) and reads the shared ${CACHE_DIR}/vm.env.
exec bash "${SEL_DIR}/../selinux/boot.sh" "$@"
