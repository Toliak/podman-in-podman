#!/usr/bin/env bash
set -euo pipefail
source "$(dirname "$0")/lib.sh"

need curl
need qemu-img
need virt-customize
need guestfish
need ssh-keygen
need sha256sum
need python3

mkdir -p "$CACHE_DIR" "${CACHE_DIR}/kernel"

download() {
  local name="$1"
  if [[ -f "${CACHE_DIR}/${name}" ]]; then
    log "already present: ${name}"
    return 0
  fi
  log "downloading ${name}"
  curl -fL --retry 3 --retry-delay 2 -o "${CACHE_DIR}/${name}.part" "${IMAGE_URL_BASE}/${name}"
  mv "${CACHE_DIR}/${name}.part" "${CACHE_DIR}/${name}"
}

download "$BASE_IMAGE"
download "$CHECKSUM_FILE"

expected="$(python3 - "${CACHE_DIR}/${CHECKSUM_FILE}" "$BASE_IMAGE" <<'PY'
import re
import sys

path, name = sys.argv[1], sys.argv[2]
with open(path, encoding="utf-8") as handle:
    for line in handle:
        match = re.match(r"^SHA256 \(([^)]+)\) = ([0-9A-Fa-f]{64})\s*$", line.strip())
        if match and match.group(1) == name:
            print(match.group(2))
            break
    else:
        raise SystemExit(f"no SHA256 entry for {name} in {path}")
PY
)"
actual="$(sha256sum "${CACHE_DIR}/${BASE_IMAGE}" | cut -d' ' -f1)"
[[ "$actual" == "$expected" ]] || die "checksum mismatch for ${BASE_IMAGE}"
log "checksum OK"

if [[ ! -f "$SSH_KEY" ]]; then
  ssh-keygen -q -t ed25519 -N '' -f "$SSH_KEY"
fi

if [[ ! -f "${CACHE_DIR}/baked.qcow2" ]]; then
  log "creating baked overlay"
  qemu-img create -f qcow2 -F qcow2 -b "${CACHE_DIR}/${BASE_IMAGE}" "${CACHE_DIR}/baked.qcow2"
  virt-customize -a "${CACHE_DIR}/baked.qcow2" \
    --ssh-inject "root:file:${SSH_KEY}.pub" \
    --run-command 'mkdir -p /etc/cloud /etc/ssh/sshd_config.d && touch /etc/cloud/cloud-init.disabled && echo "PermitRootLogin yes" > /etc/ssh/sshd_config.d/99-pinp.conf' \
    --selinux-relabel
fi

KERNEL="${CACHE_DIR}/kernel/vmlinuz"
INITRD="${CACHE_DIR}/kernel/initrd.img"

if [[ ! -f "$KERNEL" || ! -f "$INITRD" ]]; then
  log "extracting kernel and initramfs"
  mapfile -t entries < <(guestfish --ro -a "${CACHE_DIR}/${BASE_IMAGE}" -i ls /boot/)
  vmlinuz="$(printf '%s\n' "${entries[@]}" | grep '^vmlinuz-' | sort -V | tail -n 1)"
  version="${vmlinuz#vmlinuz-}"
  initramfs="initramfs-${version}.img"
  if ! printf '%s\n' "${entries[@]}" | grep -qx "$initramfs"; then
    initramfs="$(printf '%s\n' "${entries[@]}" | grep '^initramfs-.*\.img$' | sort -V | tail -n 1)"
  fi
  [[ -n "$vmlinuz" && -n "$initramfs" ]] || die "no vmlinuz/initramfs found in guest /boot"
  guestfish --ro -a "${CACHE_DIR}/${BASE_IMAGE}" -i <<EOF
download /boot/${vmlinuz} ${KERNEL}
download /boot/${initramfs} ${INITRD}
EOF
fi

fstab="$(guestfish --ro -a "${CACHE_DIR}/${BASE_IMAGE}" -i cat /etc/fstab)"
mapfile -t root_info < <(python3 - "$fstab" <<'PY'
import sys

for line in sys.argv[1].splitlines():
    fields = line.split()
    if len(fields) >= 2 and fields[1] == "/":
        print(fields[0])
        print(fields[3] if len(fields) > 3 else "")
        break
else:
    raise SystemExit("no root mount in /etc/fstab")
PY
)
ROOT_ARG="${root_info[0]}"
ROOTFLAGS="${root_info[1]:-}"

printf 'KERNEL=%s\nINITRD=%s\nROOT_ARG=%s\nROOTFLAGS=%s\n' "$KERNEL" "$INITRD" "$ROOT_ARG" "$ROOTFLAGS" > "${CACHE_DIR}/vm.env"
log "prepared: kernel=${KERNEL} initrd=${INITRD} root=${ROOT_ARG} rootflags=${ROOTFLAGS}"
