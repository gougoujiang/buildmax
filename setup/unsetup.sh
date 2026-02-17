#!/usr/bin/env bash
# Tear down local dev: delete kind cluster and stop MinIO/MySQL port-forwards.
# Brew-installed tools (kind, helm, kubectl, awscli) are not removed.
set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]:-$0}")" && pwd)"
CLUSTER_NAME="${BUILDMAX_KIND_CLUSTER:-buildmaxdev}"
PID_FILE="$SCRIPT_DIR/.port-forward.pid"
PID_FILE_MYSQL="$SCRIPT_DIR/.port-forward-mysql.pid"

log() { echo "[unsetup] $*"; }

# Stop port-forwards if we started them
for pf in "$PID_FILE" "$PID_FILE_MYSQL"; do
  if [[ -f "$pf" ]]; then
    pid=$(cat "$pf")
    if kill -0 "$pid" 2>/dev/null; then
      log "Stopping port-forward (PID $pid)..."
      kill "$pid" 2>/dev/null || true
    fi
    rm -f "$pf"
  fi
done
log "Port-forwards stopped."

# Delete kind cluster
if kind get clusters 2>/dev/null | grep -qx "$CLUSTER_NAME"; then
  log "Deleting kind cluster '$CLUSTER_NAME'..."
  kind delete cluster --name "$CLUSTER_NAME"
  log "Cluster deleted."
else
  log "Kind cluster '$CLUSTER_NAME' does not exist (already clean)."
fi

log "Unsetup done. Brew tools (kind, helm, kubectl, awscli) were not removed."
