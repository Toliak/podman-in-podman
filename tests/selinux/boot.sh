#!/usr/bin/env bash
set -euo pipefail
source "$(dirname "$0")/lib.sh"

need qemu-system-x86_64

action="${1:-start}"
case "$action" in
  start)
    overlay="${2:-${CACHE_DIR}/prepared.qcow2}"
    [[ -f "${CACHE_DIR}/vm.env" ]] || die "run prepare-image.sh first"
    source "${CACHE_DIR}/vm.env"
    [[ -f "$overlay" ]] || die "overlay not found: ${overlay}"
    accel="$(qemu_accel)"
    log "booting ${overlay} (accel=${accel}, root=${ROOT_ARG}, rootflags=${ROOTFLAGS:-none})"
    kernel_cmdline="root=${ROOT_ARG} ro console=ttyS0,115200 selinux=1 enforcing=1"
    if [[ -n "${ROOTFLAGS:-}" ]]; then
      kernel_cmdline="${kernel_cmdline} rootflags=${ROOTFLAGS}"
    fi
    : > "$VM_SERIAL"
    qemu-system-x86_64 \
      -accel "$accel" \
      -M pc \
      -m "$VM_RAM" -smp "$VM_CPUS" \
      -kernel "$KERNEL" -initrd "$INITRD" \
      -append "$kernel_cmdline" \
      -drive "file=${overlay},format=qcow2,if=virtio,cache=writeback" \
      -netdev "user,id=n0,hostfwd=tcp::${SSH_PORT}-:22" \
      -device virtio-net-pci,netdev=n0 \
      -display none -serial "file:${VM_SERIAL}" \
      -pidfile "$VM_PIDFILE" \
      -daemonize
    wait_ssh
    log "guest is up"
    ;;
  stop)
    vm_stop
    log "guest stopped"
    ;;
  status)
    if vm_running; then log running; else log stopped; fi
    ;;
  *)
    die "usage: boot.sh [start|stop|status] [overlay.qcow2]"
    ;;
esac
