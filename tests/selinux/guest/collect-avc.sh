#!/usr/bin/env bash
set -uo pipefail

OUT="${1:?output dir}"
START="${2:?start epoch}"

mkdir -p "$OUT"
getenforce > "$OUT/getenforce.txt" 2>&1 || true
sestatus > "$OUT/sestatus.txt" 2>&1 || true
printf '%s\n' "$START" > "$OUT/start.txt"

start_date="$(date -d "@${START}" '+%m/%d/%Y')"
start_time="$(date -d "@${START}" '+%H:%M:%S')"

ausearch -m AVC,USER_AVC -ts "$start_date" "$start_time" > "$OUT/avc.txt" 2>&1 || true
journalctl -k -b --no-pager > "$OUT/kernel.log.txt" 2>&1 || true
ausearch -m AVC,USER_AVC -ts "$start_date" "$start_time" 2>/dev/null | audit2allow > "$OUT/audit2allow.txt" 2>&1 || true
exit 0
