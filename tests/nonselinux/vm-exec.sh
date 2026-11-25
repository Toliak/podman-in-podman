#!/usr/bin/env bash
# Execute one command inside the booted non-SELinux guest as devuser, over SSH.
# Usage: vm-exec.sh [--env KEY=VAL ...] -- ARGV...
# Propagates the remote exit code, stdout and stderr. Used by the pytest fixture.
set -uo pipefail
source "$(dirname "$0")/lib.sh"

envs=()
while [[ $# -gt 0 ]]; do
  case "$1" in
    --env) envs+=("$2"); shift 2 ;;
    --) shift; break ;;
    *) break ;;
  esac
done

declare -a quoted=()
for arg in "$@"; do
  quoted+=("$(printf '%q' "$arg")")
done
command="${quoted[*]}"

env_prefix=""
if [[ ${#envs[@]} -gt 0 ]]; then
  declare -a envparts=()
  for kv in "${envs[@]}"; do
    envparts+=("${kv%%=*}=$(printf '%q' "${kv#*=}")")
  done
  env_prefix="${envparts[*]} "
fi

inner="cd /home/${GUEST_USER} && env -i HOME=/home/${GUEST_USER} USER=${GUEST_USER} LOGNAME=${GUEST_USER} PATH=/usr/bin:/bin:/usr/sbin:/sbin XDG_RUNTIME_DIR=/run/user/1002 ${env_prefix}bash -c $(printf '%q' "$command")"
ssh_root "runuser -u ${GUEST_USER} -- bash -c $(printf '%q' "$inner")"
