#!/usr/bin/env bash
# One-click setup for local dev: kind cluster, MinIO, awscli, and test job.
# Idempotent: safe to run multiple times.
set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]:-$0}")" && pwd)"
CLUSTER_NAME="${BUILDMAX_KIND_CLUSTER:-buildmaxdev}"
PID_FILE="$SCRIPT_DIR/.port-forward.pid"
PID_FILE_MYSQL="$SCRIPT_DIR/.port-forward-mysql.pid"

log() { echo "[setup] $*"; }

# --- Brew: kind, helm, kubectl, awscli ---
ensure_brew() {
  if ! command -v brew &>/dev/null; then
    log "Homebrew not found. Install from https://brew.sh and re-run."
    exit 1
  fi
  for pkg in kind helm kubectl awscli; do
    if brew list "$pkg" &>/dev/null; then
      log "$pkg already installed (brew)"
    else
      log "Installing $pkg..."
      brew install "$pkg"
    fi
  done
}

# --- Wails CLI (for desktop app: cmd/buildmax-desktop) ---
ensure_wails() {
  if ! command -v go &>/dev/null; then
    log "Go not found. Install Go (https://go.dev) and re-run setup."
    exit 1
  fi
  if command -v wails &>/dev/null; then
    log "wails already installed ($(wails version 2>/dev/null || echo 'in PATH'))"
    return 0
  fi
  log "Installing Wails CLI (go install)..."
  go install github.com/wailsapp/wails/v2/cmd/wails@latest
  local gobin
  gobin=$(go env GOPATH 2>/dev/null)/bin
  if command -v wails &>/dev/null; then
    log "Wails CLI ready. For desktop: cd cmd/buildmax-desktop && wails dev (or ./make run desktop)."
  elif [[ -x "${gobin}/wails" ]]; then
    log "Wails installed at ${gobin}/wails but not in PATH. Add to PATH: export PATH=\"${gobin}:\$PATH\""
  else
    log "Wails install may have failed. Ensure \$GOPATH/bin or \$HOME/go/bin is in PATH, then: go install github.com/wailsapp/wails/v2/cmd/wails@latest"
  fi
}

# --- Kind cluster ---
ensure_kind_cluster() {
  if kind get clusters 2>/dev/null | grep -qx "$CLUSTER_NAME"; then
    log "Kind cluster '$CLUSTER_NAME' already exists"
    return 0
  fi
  log "Creating kind cluster '$CLUSTER_NAME'..."
  kind create cluster --name "$CLUSTER_NAME" --config "$SCRIPT_DIR/kind-config.yaml"
  log "Waiting for nodes to be ready..."
  kubectl wait --for=condition=Ready nodes --all --timeout=120s 2>/dev/null || true
}

# --- Ingress controller (ingress-nginx for kind) ---
# Use local manifest so setup works when raw.githubusercontent.com is unreachable.
ensure_ingress() {
  if kubectl get deployment ingress-nginx-controller -n ingress-nginx &>/dev/null; then
    log "Ingress controller already present"
    return 0
  fi
  log "Applying ingress-nginx (kind)..."
  kubectl apply -f "$SCRIPT_DIR/kind-ingress-nginx.yaml"
  log "Waiting for ingress-nginx controller to be ready..."
  kubectl wait --for=condition=Available deployment/ingress-nginx-controller -n ingress-nginx --timeout=120s
  log "Ingress controller ready"
}

# --- Whoami (ingress test app in namespace test) ---
ensure_whoami_ingress_test() {
  if kubectl get deployment whoami -n test &>/dev/null; then
    log "Whoami (ingress test) already deployed in namespace test"
    return 0
  fi
  log "Deploying whoami for ingress test (namespace test)..."
  kubectl apply -f "$SCRIPT_DIR/test-ingress-whoami.yaml"
  log "Waiting for whoami to be ready..."
  kubectl wait --for=condition=Available deployment/whoami -n test --timeout=60s
  log "Whoami ready. Add '127.0.0.1 whoami.kind.local' to /etc/hosts, then: curl http://whoami.kind.local"
}

# --- Namespace and MinIO ---
ensure_storage() {
  if kubectl get namespace storage &>/dev/null; then
    log "Namespace 'storage' already exists"
  else
    log "Creating namespace storage..."
    kubectl create namespace storage
  fi

  log "Applying MinIO manifest..."
  kubectl apply -f "$SCRIPT_DIR/minio.yaml"

  log "Waiting for MinIO deployment to be ready..."
  kubectl wait --for=condition=Available deployment/minio -n storage --timeout=120s
}

# --- MySQL in cluster ---
ensure_mysql() {
  log "Applying MySQL manifest..."
  kubectl apply -f "$SCRIPT_DIR/mysql.yaml"

  log "Waiting for MySQL deployment to be ready..."
  kubectl wait --for=condition=Available deployment/mysql -n db --timeout=120s

  # Port-forward MySQL so local apps can use localhost:3306
  if [[ -f "$PID_FILE_MYSQL" ]]; then
    local pid
    pid=$(cat "$PID_FILE_MYSQL")
    if kill -0 "$pid" 2>/dev/null; then
      log "MySQL port-forward already running (PID $pid)"
      return 0
    fi
    rm -f "$PID_FILE_MYSQL"
  fi
  log "Starting port-forward for MySQL (3306)..."
  kubectl port-forward svc/mysql 3306:3306 -n db &>/dev/null &
  echo $! > "$PID_FILE_MYSQL"
  sleep 1
  if ! kill -0 "$(cat "$PID_FILE_MYSQL")" 2>/dev/null; then
    log "MySQL port-forward failed to start"
    rm -f "$PID_FILE_MYSQL"
    return 1
  fi
}

# --- Port-forward and S3 bucket (local) ---
ensure_bucket_via_portforward() {
  # Stop any existing port-forward we started
  if [[ -f "$PID_FILE" ]]; then
    local pid
    pid=$(cat "$PID_FILE")
    if kill -0 "$pid" 2>/dev/null; then
      log "Stopping previous port-forward (PID $pid)..."
      kill "$pid" 2>/dev/null || true
    fi
    rm -f "$PID_FILE"
  fi

  log "Starting port-forward for MinIO (9000, 9001)..."
  kubectl port-forward svc/minio 9000:9000 9001:9001 -n storage &>/dev/null &
  echo $! > "$PID_FILE"
  sleep 2
  if ! kill -0 "$(cat "$PID_FILE")" 2>/dev/null; then
    log "Port-forward failed to start"
    rm -f "$PID_FILE"
    return 1
  fi

  export AWS_ACCESS_KEY_ID=minio
  export AWS_SECRET_ACCESS_KEY=minio123
  export AWS_DEFAULT_REGION=us-east-1

  if aws --endpoint-url http://localhost:9000 s3 ls s3://bmstore &>/dev/null; then
    log "Bucket s3://bmstore already exists"
  else
    log "Creating bucket s3://bmstore..."
    aws --endpoint-url http://localhost:9000 s3 mb s3://bmstore
  fi
  log "S3 bucket list:"
  aws --endpoint-url http://localhost:9000 s3 ls
}

# --- Test job (in-cluster) ---
ensure_test_job() {
  if kubectl get job s3-test &>/dev/null; then
    log "Job s3-test already exists; showing logs..."
  else
    log "Applying test job..."
    kubectl apply -f "$SCRIPT_DIR/test-minio-job.yaml"
    log "Waiting for job s3-test to complete..."
    kubectl wait --for=condition=complete job/s3-test --timeout=120s
  fi
  log "Job s3-test logs:"
  kubectl logs job/s3-test
}

# --- Main ---
main() {
  log "Starting idempotent setup (cluster=$CLUSTER_NAME)..."
  # Free port 3306 if MySQL was running via docker compose
  if [[ -f "$SCRIPT_DIR/../docker-compose.yml" ]]; then
    (cd "$SCRIPT_DIR/../" && docker compose down 2>/dev/null) || true
  fi
  ensure_brew
  ensure_wails
  ensure_kind_cluster
  kubectl get nodes
  ensure_ingress
  ensure_whoami_ingress_test
  ensure_storage
  ensure_mysql
  ensure_bucket_via_portforward
  ensure_test_job
  log "Setup done. MinIO: http://localhost:9000 (API), http://localhost:9001 (console). MySQL: localhost:3306 (user buildmax, DB buildmax)."
  log "Port-forwards: $PID_FILE (MinIO), $PID_FILE_MYSQL (MySQL). Use './make unsetup' to tear down cluster and stop port-forwards."
  log "For Ingress: add '127.0.0.1 whoami.kind.local' and/or '127.0.0.1 buildmax.kind.local' to /etc/hosts."
}

main "$@"
