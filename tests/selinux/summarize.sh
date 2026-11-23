#!/usr/bin/env bash
set -euo pipefail
source "$(dirname "$0")/lib.sh"

RESULTS_DIR="${SEL_DIR}/results"
OUT="${SEL_DIR}/RESULTS.md"

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

read_rc() {
  local f="$1"
  if [[ -f "$f" ]]; then
    tr -d '\r\n' < "$f"
  else
    printf '?'
  fi
}

count_avc() {
  local f="$1" alt="$2" n
  n="$(grep -c 'avc:.*denied' "$f" 2>/dev/null || true)"
  if [[ "${n:-0}" == "0" && -f "$alt" ]]; then
    n="$(grep -c 'avc:.*denied' "$alt" 2>/dev/null || true)"
  fi
  printf '%s' "${n:-0}"
}

{
  printf '# SELinux nested-podman experiment\n\n'
  printf 'Guest: `Fedora-Cloud-Base-Generic-45-1.4.x86_64.qcow2`, SELinux enforcing, direct kernel boot.\n'
  printf 'Topology: VM (SELinux host) -> outer `podman run` (L1, `pinp-outer`) -> inner podman (L2).\n\n'
  printf '## Case matrix\n\n'
  printf '| Case | label | suffix | outer rc | images rc | hello rc | build rc | AVC denials |\n'
  printf '|---|---|---|---|---|---|---|---|\n'
  for spec in "${CASES[@]}"; do
    IFS=: read -r name label suffix <<< "$spec"
    d="${RESULTS_DIR}/${name}"
    printf '| %s | %s | %s | %s | %s | %s | %s | %s |\n' \
      "$name" "$label" "$suffix" \
      "$(read_rc "${d}/run.rc.txt")" \
      "$(read_rc "${d}/images.rc.txt")" \
      "$(read_rc "${d}/hello.rc.txt")" \
      "$(read_rc "${d}/build.rc.txt")" \
      "$(count_avc "${d}/avc.txt" "${d}/kernel.log.txt")"
  done
  printf '\n## Artifacts\n\n'
  printf 'Per-case evidence is in `tests/selinux/results/<case>/`: `run.stderr.txt`, `images.txt`, `inner-info.txt`, `hello.txt`, `build.txt`, `check.json`, `outer-labels.txt`, `labels.before.txt`, `labels.after.txt`, `avc.txt`, `audit2allow.txt`, `kernel.log.txt`, `sestatus.txt`.\n\n'
  printf '## Findings (reference run: Fedora 45, podman 6.1.3, container-selinux 2.251.0)\n\n'
  printf '%s\n' '- Case A (`label=disable`): L1 sees host images with `R/O=true`; inner `echo "hello world"` and inner `podman build` both succeed; no AVCs.'
  printf '%s\n' '- Cases B-F (no label option / `label=nested`, with or without `:z`/`:Z`): the outer container still starts and host images are still visible `R/O=true`, but inner `podman run` and `podman build` fail with `crun: mount devpts to dev/pts: Permission denied`.'
  printf '%s\n' '- `:z`/`:Z` do relabel the bind sources (`system_u:object_r:container_file_t`, see `labels.after.txt`) but do not rescue nested containers.'
  printf '%s\n' '- Zero AVC/USER_AVC events in every case, even with `semodule -DB` (dontaudit disabled). The devpts failure is therefore not an SELinux policy denial; it correlates with L1 being confined (`container_t`) versus unconfined (`label=disable` yields an empty `ProcessLabel`, see `outer-labels.txt`). Likely crun applying a `context=` devpts mount option inside a nested userns.'
  printf '%s\n' '- `label=nested` produced the same `container_t:s0:cX,cY` ProcessLabel as the default, so it did not relax confinement on this guest.'
  printf '%s\n' '- Cases G/H: `label=disable` plus `:z`/`:Z` still works, so relabeling alone is harmless to the current PoC.'
  printf '\n'
  printf '## TODO.md decision\n\n'
  printf '`:z`/`:Z` + `label=nested` are **not** drop-in replacements for `--security-opt label=disable` on this guest. `label=disable` is required on SELinux-enforcing hosts and was already confirmed a no-op on the non-SELinux Arch host (`pinp-nolabel`), so the "SELinux-only" qualification is supported. Recommendation: keep `label=disable` in `pinp-e`; keep the TODO item open only if a different SELinux-preserving mechanism (e.g. a custom policy permitting nested `devpts` mounts) is worth investigating.\n\n'
  printf 'This harness records findings only; it does not edit `INFO.md` or `pinp-e`.\n'
} > "$OUT"

log "wrote ${OUT}"
