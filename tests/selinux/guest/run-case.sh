#!/usr/bin/env bash
set -uo pipefail

NAME="${1:?case name}"
LABEL_MODE="${2:?label mode}"
SUFFIX_MODE="${3:?suffix mode}"
OUT="${4:?output dir}"

mkdir -p "$OUT"

case "$LABEL_MODE" in
  disable) LABEL_ARGS=(--security-opt label=disable) ;;
  nested) LABEL_ARGS=(--security-opt label=nested) ;;
  none) LABEL_ARGS=() ;;
  *) printf 'unknown label mode: %s\n' "$LABEL_MODE" >&2; exit 2 ;;
esac

case "$SUFFIX_MODE" in
  z) SUFFIX=':z' ;;
  Z) SUFFIX=':Z' ;;
  none) SUFFIX='' ;;
  *) printf 'unknown suffix mode: %s\n' "$SUFFIX_MODE" >&2; exit 2 ;;
esac

U="$(id -u)"
G="$(id -g)"
UIDMAP=(--uidmap "${U}:0:1" --uidmap "0:1:${U}" --uidmap "100000:$((U + 1)):$((65536 - U - 3))" --uidmap '65534:65534:2')
GIDMAP=(--gidmap "${G}:0:1" --gidmap "0:1:${G}" --gidmap "100000:$((G + 1)):$((65536 - G - 3))" --gidmap '65534:65534:2')
STORE="${HOME}/.local/share/containers/storage"
WORK="${HOME}/workdir"
CNAME="pinp-selinux-${NAME}"

printf 'name=%s label_mode=%s suffix_mode=%s uid=%s gid=%s\n' "$NAME" "$LABEL_MODE" "$SUFFIX_MODE" "$U" "$G" > "$OUT/meta.txt"

ls -Zd "$WORK" "$STORE" > "$OUT/labels.before.txt" 2>&1 || true
podman rm -f "$CNAME" >/dev/null 2>&1 || true

podman run -d --rm --name "$CNAME" \
  "${LABEL_ARGS[@]}" \
  --device /dev/fuse \
  -v "${WORK}:/opt/workdir${SUFFIX}" \
  -v "${STORE}:/var/lib/shared${SUFFIX}" \
  "${UIDMAP[@]}" "${GIDMAP[@]}" \
  pinp-outer sleep infinity > "$OUT/run.stdout.txt" 2> "$OUT/run.stderr.txt"
printf '%s\n' "$?" > "$OUT/run.rc.txt"

if [[ "$(<"${OUT}/run.rc.txt")" == "0" ]]; then
  podman inspect "$CNAME" --format '{{.ProcessLabel}} {{.MountLabel}}' > "$OUT/outer-labels.txt" 2>&1
  podman exec "$CNAME" podman image ls --format '{{.Repository}} {{.ReadOnly}}' > "$OUT/images.txt" 2>&1
  printf '%s\n' "$?" > "$OUT/images.rc.txt"
  podman exec "$CNAME" podman info --format '{{.Store.GraphDriverName}} {{.Host.OCIRuntime.Name}}' > "$OUT/inner-info.txt" 2>&1
  podman exec "$CNAME" podman run --rm --uidmap 0:0:1500 --uidmap 65534:1500:2 docker.io/library/debian:trixie echo 'hello world' > "$OUT/hello.txt" 2>&1
  printf '%s\n' "$?" > "$OUT/hello.rc.txt"
  podman cp /home/devuser/bin/pinp-i "${CNAME}:/tmp/pinp-i" >/dev/null 2>&1 || true
  podman exec "$CNAME" /tmp/pinp-i pinp-check --json --no-fail > "$OUT/check.json" 2>&1 || true
  podman exec "$CNAME" podman build -t pinp-build /opt/workdir/build > "$OUT/build.txt" 2>&1
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
