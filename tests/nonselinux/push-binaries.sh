#!/usr/bin/env bash
set -euo pipefail
source "$(dirname "$0")/lib.sh"

# Refresh the guest-side wrappers from bin/ so a cached prepared.qcow2 cannot keep
# stale pinp-e/pinp-i around after they change. The L1 image is unaffected (pinp-i
# is copied into L1 at run time).
scp_push "${REPO_DIR}/bin/pinp-e" /home/devuser/bin/pinp-e
scp_push "${REPO_DIR}/bin/pinp-i" /home/devuser/bin/pinp-i
ssh_root 'chown 1002:1006 /home/devuser/bin/pinp-e /home/devuser/bin/pinp-i && chmod +x /home/devuser/bin/pinp-e /home/devuser/bin/pinp-i'
log "pushed current pinp binaries"
