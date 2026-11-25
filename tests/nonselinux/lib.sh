#!/usr/bin/env bash
set -euo pipefail

# Non-SELinux sibling of tests/selinux. It reuses the SELinux harness's image
# preparation (download, baked overlay, kernel/initrd, vm.env, ssh key) from the
# shared CACHE_DIR, but keeps its own runtime state (prepared image, pidfile,
# serial log, case overlays) and SSH port so the two VMs never collide. SELinux
# is disabled purely via the kernel cmdline (PINP_VM_KERNEL_EXTRA=selinux=0).

NS_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
REPO_DIR="$(cd -- "${NS_DIR}/../.." && pwd)"

export PINP_VM_RUN_CACHE="${PINP_NONSELINUX_CACHE:-${REPO_DIR}/.nonselinux-cache}"
export PINP_VM_PIDFILE="${PINP_VM_RUN_CACHE}/qemu.pid"
export PINP_VM_SERIAL="${PINP_VM_RUN_CACHE}/serial.log"
export PINP_VM_SSH_PORT="${PINP_NONSELINUX_SSH_PORT:-2223}"
export PINP_VM_LOG_PREFIX="nonselinux"
export PINP_VM_KERNEL_EXTRA="selinux=0"

# shellcheck source=../selinux/lib.sh
source "${NS_DIR}/../selinux/lib.sh"

# selinux/lib.sh recomputes SEL_DIR/REPO_DIR from its own location; restore ours.
SEL_DIR="$NS_DIR"
REPO_DIR="$(cd -- "${SEL_DIR}/../.." && pwd)"
export SEL_DIR REPO_DIR

mkdir -p "$RUN_CACHE"
