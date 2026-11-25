#!/usr/bin/env bash
set -uo pipefail

# Golden-path e2e for a non-SELinux guest. It drives the shipped wrappers
# end-to-end: pinp-e creates L1, pinp-i runs the inner container/build and the
# preflight check inside L1. Bare --userns=keep-id keeps /var/lib/shared owned
# by devuser (pinp-e's uid=0,gid=0 default would make it root-owned and unusable;
# see INFO.md's shared-store caveat). Writes per-check rc files, never aborts.
#
# Usage: run-case.sh <name> [outdir]

NAME="${1:-golden}"
OUT="${2:-/tmp/pinp-out}"
mkdir -p "$OUT"

PINP_E="/home/devuser/bin/pinp-e"
PINP_I="/home/devuser/bin/pinp-i"
STORE="${HOME}/.local/share/containers/storage"
WORK="${HOME}/workdir"
CNAME="pinp-nonselinux-${NAME}"

printf 'name=%s uid=%s gid=%s\n' "$NAME" "$(id -u)" "$(id -g)" > "$OUT/meta.txt"
{ getenforce 2>/dev/null || echo Disabled; } > "$OUT/getenforce.txt"
if [[ -d /sys/fs/selinux ]]; then echo 1; else echo 0; fi > "$OUT/selinuxfs.txt"

ls -Zd "$WORK" "$STORE" > "$OUT/labels.before.txt" 2>&1 || true
podman rm -f "$CNAME" >/dev/null 2>&1 || true

"$PINP_E" run -d --rm --name "$CNAME" --userns=keep-id \
  -v "${WORK}:/opt/workdir" \
  pinp-outer sleep infinity > "$OUT/run.stdout.txt" 2> "$OUT/run.stderr.txt"
printf '%s\n' "$?" > "$OUT/run.rc.txt"

if [[ "$(<"${OUT}/run.rc.txt")" == "0" ]]; then
  podman inspect "$CNAME" --format '{{.ProcessLabel}} {{.MountLabel}}' > "$OUT/outer-labels.txt" 2>&1 || true
  podman exec "$CNAME" podman image ls --format '{{.Repository}} {{.ReadOnly}}' > "$OUT/images.txt" 2>&1
  printf '%s\n' "$?" > "$OUT/images.rc.txt"
  podman exec "$CNAME" podman info --format '{{.Store.GraphDriverName}} {{.Host.OCIRuntime.Name}}' > "$OUT/inner-info.txt" 2>&1 || true
  podman cp "$PINP_I" "${CNAME}:/tmp/pinp-i" >/dev/null 2>&1 || true
  podman exec "$CNAME" /tmp/pinp-i pinp-check --json --no-fail > "$OUT/check.json" 2>&1 || true
  podman exec "$CNAME" /tmp/pinp-i run --rm debian:trixie echo 'hello world' > "$OUT/hello.txt" 2>&1
  printf '%s\n' "$?" > "$OUT/hello.rc.txt"
  podman exec "$CNAME" /tmp/pinp-i build -t pinp-build /opt/workdir/build > "$OUT/build.txt" 2>&1
  printf '%s\n' "$?" > "$OUT/build.rc.txt"
else
  for f in images.txt hello.txt build.txt check.json; do
    printf 'skipped: outer container did not start\n' > "${OUT}/${f}"
  done
  for f in images.rc.txt hello.rc.txt build.rc.txt; do
    printf '1\n' > "${OUT}/${f}"
  done
fi

ls -Zd "$WORK" "$STORE" > "$OUT/labels.after.txt" 2>&1 || true
podman rm -f "$CNAME" >/dev/null 2>&1 || true
exit 0
