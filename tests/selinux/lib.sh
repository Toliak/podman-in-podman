#!/usr/bin/env bash
set -euo pipefail

SEL_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
REPO_DIR="$(cd -- "${SEL_DIR}/../.." && pwd)"
CACHE_DIR="${PINP_SELINUX_CACHE:-${REPO_DIR}/.selinux-cache}"

BASE_IMAGE="Fedora-Cloud-Base-Generic-45-1.4.x86_64.qcow2"
CHECKSUM_FILE="Fedora-Cloud-45-1.4-x86_64-CHECKSUM"
IMAGE_URL_BASE="https://download.fedoraproject.org/pub/fedora/linux/releases/45/Cloud/x86_64/images"

SSH_PORT="${PINP_SELINUX_SSH_PORT:-2222}"
SSH_KEY="${CACHE_DIR}/id_ed25519"
GUEST_USER="devuser"
VM_RAM="${PINP_SELINUX_RAM:-1536}"
VM_CPUS="${PINP_SELINUX_CPUS:-2}"
VM_PIDFILE="${CACHE_DIR}/qemu.pid"
VM_SERIAL="${CACHE_DIR}/serial.log"

SSH_OPTS=(-o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o LogLevel=ERROR -o ConnectTimeout=10 -o BatchMode=yes)

log() {
  printf '[selinux] %s\n' "$*" >&2
}

die() {
  log "ERROR: $*"
  exit 1
}

need() {
  command -v "$1" >/dev/null 2>&1 || die "missing command: $1"
}

qemu_accel() {
  if [[ -r /dev/kvm && -w /dev/kvm ]]; then
    printf 'kvm\n'
  else
    printf 'tcg\n'
  fi
}

ssh_root() {
  ssh -n -i "$SSH_KEY" -p "$SSH_PORT" "${SSH_OPTS[@]}" root@127.0.0.1 "$@"
}

ssh_guest() {
  local cmd="$*"
  local inner="cd /home/${GUEST_USER} && ${cmd}"
  ssh_root "runuser -u ${GUEST_USER} -- env -i HOME=/home/${GUEST_USER} USER=${GUEST_USER} LOGNAME=${GUEST_USER} PATH=/usr/bin:/bin XDG_RUNTIME_DIR=/run/user/1002 bash -c $(printf '%q' "$inner")"
}

scp_push() {
  local src="$1" dst="$2"
  scp -i "$SSH_KEY" -P "$SSH_PORT" "${SSH_OPTS[@]}" "$src" "root@127.0.0.1:${dst}"
}

scp_push_tree() {
  local src="$1" dst="$2"
  scp -r -i "$SSH_KEY" -P "$SSH_PORT" "${SSH_OPTS[@]}" "$src" "root@127.0.0.1:${dst}"
}

wait_ssh() {
  local _
  for _ in $(seq 1 150); do
    if ssh_root true >/dev/null 2>&1; then
      return 0
    fi
    sleep 2
  done
  die "guest did not become reachable over SSH (serial log: ${VM_SERIAL})"
}

vm_running() {
  [[ -f "$VM_PIDFILE" ]] && kill -0 "$(<"${VM_PIDFILE}")" 2>/dev/null
}

vm_stop() {
  [[ -f "$VM_PIDFILE" ]] || return 0
  local pid
  pid="$(<"${VM_PIDFILE}")"
  if kill -0 "$pid" 2>/dev/null; then
    ssh_root poweroff >/dev/null 2>&1 || true
    local _
    for _ in $(seq 1 30); do
      kill -0 "$pid" 2>/dev/null || break
      sleep 1
    done
    if kill -0 "$pid" 2>/dev/null; then
      kill "$pid" 2>/dev/null || true
      sleep 1
    fi
  fi
  rm -f "$VM_PIDFILE"
}

export SEL_DIR REPO_DIR CACHE_DIR
export BASE_IMAGE CHECKSUM_FILE IMAGE_URL_BASE
export SSH_PORT SSH_KEY GUEST_USER VM_RAM VM_CPUS VM_PIDFILE VM_SERIAL
