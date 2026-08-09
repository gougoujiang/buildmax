#!/usr/bin/env bash
set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]:-$0}")" && pwd)"
cd "$SCRIPT_DIR"

CLI_BINARY="buildmax"
SERVER_BINARY="buildmax-server"
WORKER_BINARY="buildmax-worker"
DESKTOP_BINARY="buildmax-desktop"
BIN_DIR="bin"
DESKTOP_DIR="cmd/buildmax-desktop"

source ./loadenv

usage() {
  echo "Usage: ./make <command>"
  echo ""
  echo "Commands:"
  echo "  build         Build $CLI_BINARY, $SERVER_BINARY, $WORKER_BINARY, gui, desktop app; copy desktop binary to $SCRIPT_DIR/$BIN_DIR/$DESKTOP_BINARY for local run"
  echo "  build cli     Build only $CLI_BINARY"
  echo "  clean         Remove binaries, $DESKTOP_DIR/build, gui/portal/desktop frontend (node_modules, dist)"
  echo "  test          Run go test with BUILDMAX_HOME=testing-sandbox"
  echo "  eval          Build and run the agent benchmark (requires configured LLM API key)"
  echo "  smoke         Build, then run with -p \"/smoke 0\" and BUILDMAX_HOME=testing-sandbox"
  echo "  run server    Run $SERVER_BINARY with BUILDMAX_HOME=./testing-sandbox"
  echo "  run cli       Run $CLI_BINARY with BUILDMAX_HOME=./testing-sandbox"
  echo "  run desktop   Run $DESKTOP_BINARY with BUILDMAX_HOME=./testing-sandbox"
  echo "  run portal    Start Portal dev server (Vite; installs deps if needed)"
  echo "  bump          Bump Version in internal/config/version.go (arg: patch|minor|major, default: patch)"
  echo "  install       Install buildmax, buildmax-server, buildmax-worker to ~/.local/bin"
  echo "  setup         One-click setup: kind cluster, MinIO, awscli, test job (idempotent)"
  echo "  unsetup       Tear down kind cluster and MinIO port-forward (brew tools kept)"
  echo "  pub_images    Build BuildMax and Portal images and load into kind cluster"
  echo "  deploy        Build, load image, and deploy buildmax server to kind (run ./make setup first)"
  echo ""
  echo "Examples:"
  echo "  ./make build"
  echo "  ./make build cli"
  echo "  ./make test"
  echo "  ./make bump        # 0.0.2 -> 0.0.3"
  echo "  ./make bump minor  # 0.0.2 -> 0.1.0"
  echo "  ./make run server   # run server with BUILDMAX_HOME=./testing-sandbox"
  echo "  ./make run cli      # run CLI with BUILDMAX_HOME=./testing-sandbox"
  echo "  ./make run desktop  # run desktop binary with BUILDMAX_HOME=./testing-sandbox"
  echo "  ./make run portal   # start Portal dev server (Vite)"
  echo "  ./make install    # install binaries to ~/.local/bin"
}

ensure_testing_sandbox_config() {
  local sandbox="$SCRIPT_DIR/testing-sandbox"
  mkdir -p "$sandbox"
  local src="$HOME/.buildmax"
  for f in settings.yaml server.yaml; do
    if [[ -f "$src/$f" ]] && [[ ! -f "$sandbox/$f" ]]; then
      cp "$src/$f" "$sandbox/$f"
      echo "[sandbox] Copied $src/$f -> $sandbox/$f"
    fi
  done
}

cmd_run_server() {
  if [[ ! -f "$SCRIPT_DIR/$BIN_DIR/$SERVER_BINARY" ]]; then
    echo "Error: $BIN_DIR/$SERVER_BINARY not found. Run ./make build first."
    return 1
  fi
  ensure_testing_sandbox_config
  echo "Starting server (BUILDMAX_HOME=./testing-sandbox, Ctrl+C to stop)..."
  BUILDMAX_HOME="$SCRIPT_DIR/testing-sandbox" "$SCRIPT_DIR/$BIN_DIR/$SERVER_BINARY"
}

cmd_run_cli() {
  if [[ ! -f "$SCRIPT_DIR/$BIN_DIR/$CLI_BINARY" ]]; then
    echo "Error: $BIN_DIR/$CLI_BINARY not found. Run ./make build first."
    return 1
  fi
  ensure_testing_sandbox_config
  echo "Starting CLI (BUILDMAX_HOME=./testing-sandbox)..."
  BUILDMAX_HOME="$SCRIPT_DIR/testing-sandbox" "$SCRIPT_DIR/$BIN_DIR/$CLI_BINARY" "${@:3}"
}

cmd_run_portal() {
  if [[ ! -d "$SCRIPT_DIR/portal" ]]; then
    echo "Error: portal/ directory not found"
    return 1
  fi
  if [[ -d "$SCRIPT_DIR/gui" ]] && [[ ! -f "$SCRIPT_DIR/gui/dist/index.js" ]]; then
    echo "Building gui package (required by portal)..."
    if ! (cd "$SCRIPT_DIR/gui" && npm install && npm run build); then
      echo "Error: gui build failed."
      return 1
    fi
  fi
  if [[ ! -d "$SCRIPT_DIR/portal/node_modules" ]]; then
    echo "Installing portal dependencies..."
    if ! (cd "$SCRIPT_DIR/portal" && npm install); then
      echo "Error: portal npm install failed. Try running 'cd portal && npm install' manually."
      return 1
    fi
  fi
  echo "Starting Portal dev server (Ctrl+C to stop)..."
  (cd "$SCRIPT_DIR/portal" && npm run dev)
}

cmd_run_desktop() {
  if [[ ! -f "$SCRIPT_DIR/$BIN_DIR/$DESKTOP_BINARY" ]]; then
    echo "Error: $BIN_DIR/$DESKTOP_BINARY not found. Run ./make build first."
    return 1
  fi
  ensure_testing_sandbox_config
  echo "Starting desktop app (BUILDMAX_HOME=./testing-sandbox, Ctrl+C to stop)..."
  BUILDMAX_HOME="$SCRIPT_DIR/testing-sandbox" "$SCRIPT_DIR/$BIN_DIR/$DESKTOP_BINARY"
}

resolve_commit_sha() {
  # Emit the short (7-char) git SHA for the current HEAD, or "dev" if unavailable.
  # Appends "-dirty" when the working tree has uncommitted changes.
  if ! command -v git &>/dev/null; then
    echo "dev"
    return
  fi
  local sha
  sha=$(git -C "$SCRIPT_DIR" rev-parse --short=7 HEAD 2>/dev/null) || { echo "dev"; return; }
  if ! git -C "$SCRIPT_DIR" diff --quiet HEAD -- 2>/dev/null; then
    sha="${sha}-dirty"
  fi
  echo "$sha"
}

go_build_ldflags() {
  echo "-X github.com/gougoujiang/buildmax/internal/config.Commit=$(resolve_commit_sha)"
}

cmd_build_cli() {
  mkdir -p "$SCRIPT_DIR/$BIN_DIR"
  echo "[cli] Building $CLI_BINARY..."
  if go build -ldflags "$(go_build_ldflags)" -o "$SCRIPT_DIR/$BIN_DIR/$CLI_BINARY" ./cmd/buildmax; then
    echo "[cli] Built $BIN_DIR/$CLI_BINARY"
  else
    return 1
  fi
}

cmd_build() {
  local target="${1:-all}"
  case "$target" in
    all|"")
      ;;
    cli)
      cmd_build_cli
      return
      ;;
    *)
      echo "Usage: ./make build [cli]"
      echo "  ./make build      Build all local binaries, gui, and desktop app"
      echo "  ./make build cli  Build only $CLI_BINARY"
      return 1
      ;;
  esac

  cmd_build_cli
  local ldflags
  ldflags="$(go_build_ldflags)"
  echo "[server] Building $SERVER_BINARY..."
  if go build -ldflags "$ldflags" -o "$SCRIPT_DIR/$BIN_DIR/$SERVER_BINARY" ./cmd/buildmax-server; then
    echo "[server] Built $BIN_DIR/$SERVER_BINARY"
  else
    return 1
  fi
  echo "[worker] Building $WORKER_BINARY..."
  if go build -ldflags "$ldflags" -o "$SCRIPT_DIR/$BIN_DIR/$WORKER_BINARY" ./cmd/buildmax-worker; then
    echo "[worker] Built $BIN_DIR/$WORKER_BINARY"
  else
    return 1
  fi
  if [[ -d "$SCRIPT_DIR/gui" ]]; then
    echo "[gui] Building @buildmax/gui package..."
    if [[ ! -d "$SCRIPT_DIR/gui/node_modules" ]]; then
      (cd "$SCRIPT_DIR/gui" && npm install) || { echo "[gui] Warning: npm install failed; skipping gui."; }
    fi
    if [[ -d "$SCRIPT_DIR/gui/node_modules" ]]; then
      (cd "$SCRIPT_DIR/gui" && npm run build) || { echo "[gui] Warning: build failed; portal/desktop may fail."; }
    fi
  fi
  if [[ -d "$SCRIPT_DIR/portal" ]]; then
    if [[ ! -d "$SCRIPT_DIR/portal/node_modules" ]]; then
      echo "[portal] Installing dependencies (links @buildmax/gui via file:../gui)..."
      if (cd "$SCRIPT_DIR/portal" && npm install); then
        echo "[portal] Dependencies installed."
      else
        echo "[portal] Warning: npm install failed; ./make run portal will retry."
      fi
    else
      echo "[portal] node_modules present; skip install."
    fi
  fi
  echo "[desktop] Building desktop app (Wails)..."
  if ! command -v wails &>/dev/null; then
    echo "[desktop] Warning: wails CLI not found; skipping. Run ./make setup or: go install github.com/wailsapp/wails/v2/cmd/wails@latest"
  elif [[ ! -d "$SCRIPT_DIR/$DESKTOP_DIR" ]]; then
    echo "[desktop] Warning: $DESKTOP_DIR not found; skipping."
  elif [[ -d "$SCRIPT_DIR/gui" ]] && [[ ! -f "$SCRIPT_DIR/gui/dist/index.js" ]]; then
    echo "[desktop] Warning: gui not built (missing gui/dist/index.js). Run: cd gui && npm install && npm run build"
  elif [[ ! -d "$SCRIPT_DIR/desktop/frontend" ]]; then
    echo "[desktop] Warning: desktop/frontend/ not found; skipping."
  else
    local frontend_dir="$SCRIPT_DIR/desktop/frontend"
    if [[ ! -d "$frontend_dir/node_modules" ]]; then
      echo "[desktop] Installing frontend dependencies..."
      (cd "$frontend_dir" && npm install) || { echo "[desktop] Warning: frontend npm install failed; skipping."; return 0; }
    fi
    echo "[desktop] Building frontend (React)..."
    (cd "$frontend_dir" && npm run build) || { echo "[desktop] Warning: frontend build failed; skipping."; return 0; }
    echo "[desktop] Running wails build..."
    if (cd "$SCRIPT_DIR/$DESKTOP_DIR" && wails build); then
      echo "[desktop] Built at $SCRIPT_DIR/$DESKTOP_DIR/build/"
      # Copy desktop binary to ./bin for local testing (alongside server/worker)
      local src_bin=""
      if [[ "$(uname -s)" = Darwin ]] && [[ -f "$SCRIPT_DIR/$DESKTOP_DIR/build/bin/BuildMax.app/Contents/MacOS/$DESKTOP_BINARY" ]]; then
        src_bin="$SCRIPT_DIR/$DESKTOP_DIR/build/bin/BuildMax.app/Contents/MacOS/$DESKTOP_BINARY"
      elif [[ -f "$SCRIPT_DIR/$DESKTOP_DIR/build/bin/$DESKTOP_BINARY" ]]; then
        src_bin="$SCRIPT_DIR/$DESKTOP_DIR/build/bin/$DESKTOP_BINARY"
      fi
      if [[ -n "$src_bin" ]]; then
        mkdir -p "$SCRIPT_DIR/$BIN_DIR"
        cp -f "$src_bin" "$SCRIPT_DIR/$BIN_DIR/$DESKTOP_BINARY" && chmod +x "$SCRIPT_DIR/$BIN_DIR/$DESKTOP_BINARY" && echo "[desktop] Copied binary to $SCRIPT_DIR/$BIN_DIR/$DESKTOP_BINARY"
      fi
    else
      echo "[desktop] Warning: wails build failed (see above)."
    fi
  fi
}

cmd_clean() {
  echo "[clean] Removing binaries..."
  rm -rf "$SCRIPT_DIR/$BIN_DIR"
  echo "[clean] Removing desktop app build..."
  rm -rf "$DESKTOP_DIR/build"
  echo "[clean] Removing gui (node_modules, dist)..."
  rm -rf gui/node_modules gui/dist
  echo "[clean] Removing portal (node_modules, dist)..."
  rm -rf portal/node_modules portal/dist
  echo "[clean] Removing desktop frontend (node_modules, dist)..."
  rm -rf desktop/frontend/node_modules desktop/frontend/dist
  echo "[clean] Done."
}

cmd_test() {
  mkdir -p testing-sandbox
  export BUILDMAX_HOME="$SCRIPT_DIR/testing-sandbox"
  echo "Running tests (BUILDMAX_HOME=testing-sandbox)..."
  go test ./...
}

cmd_smoke() {
  mkdir -p testing-sandbox
  export BUILDMAX_HOME="$SCRIPT_DIR/testing-sandbox"
  mkdir -p "$SCRIPT_DIR/$BIN_DIR"
  go build -ldflags "$(go_build_ldflags)" -o "$SCRIPT_DIR/$BIN_DIR/$CLI_BINARY" ./cmd/buildmax
  export BUILDMAX_LOG_LEVEL=debug
  echo "Running smoke test..."
  "$SCRIPT_DIR/$BIN_DIR/$CLI_BINARY" -p "/smoke 0"
}

cmd_bump_version() {
  local root_go="$SCRIPT_DIR/internal/config/version.go"
  if [[ ! -f "$root_go" ]]; then
    echo "Error: $root_go not found"
    return 1
  fi
  local bump="${1:-patch}"
  local current
  current=$(grep -E '^\s*var Version = ' "$root_go" | sed -n 's/.*"\([^"]*\)".*/\1/p')
  if [[ -z "$current" ]]; then
    echo "Error: could not find var Version in $root_go"
    return 1
  fi
  local major minor patch
  IFS='.' read -r major minor patch <<< "$current"
  major="${major:-0}"
  minor="${minor:-0}"
  patch="${patch:-0}"
  case "$bump" in
    patch) patch=$((patch + 1)); minor=$minor; major=$major ;;
    minor) patch=0; minor=$((minor + 1)); major=$major ;;
    major) patch=0; minor=0; major=$((major + 1)) ;;
    *)
      echo "Error: bump must be patch, minor, or major (got: $bump)"
      return 1
      ;;
  esac
  local new_version="${major}.${minor}.${patch}"
  if [[ "$(uname -s)" = Darwin ]]; then
    sed -i '' "s/var Version = \"[^\"]*\"/var Version = \"$new_version\"/" "$root_go"
  else
    sed -i "s/var Version = \"[^\"]*\"/var Version = \"$new_version\"/" "$root_go"
  fi
  echo "Bumped version: $current -> $new_version ($bump)"
}

cmd_setup() {
  if [[ ! -f "$SCRIPT_DIR/setup/setup.sh" ]]; then
    echo "Error: setup/setup.sh not found"
    return 1
  fi
  "$SCRIPT_DIR/setup/setup.sh"
}

cmd_unsetup() {
  if [[ ! -f "$SCRIPT_DIR/setup/unsetup.sh" ]]; then
    echo "Error: setup/unsetup.sh not found"
    return 1
  fi
  "$SCRIPT_DIR/setup/unsetup.sh"
}

cmd_install() {
  local LOCAL_BIN="${HOME}/.local/bin"
  if [[ ! -f "$SCRIPT_DIR/$BIN_DIR/$CLI_BINARY" ]]; then
    echo "Error: $BIN_DIR/$CLI_BINARY not found in $SCRIPT_DIR" >&2
    echo "Run './make build' or 'go build -o $BIN_DIR/$CLI_BINARY ./cmd/buildmax' first." >&2
    return 1
  fi
  mkdir -p "$LOCAL_BIN"
  echo "Installing BuildMax binaries to ${LOCAL_BIN}..."
  echo "Copying $CLI_BINARY to ${LOCAL_BIN}/$CLI_BINARY"
  cp -f "$SCRIPT_DIR/$BIN_DIR/$CLI_BINARY" "${LOCAL_BIN}/$CLI_BINARY"
  chmod +x "${LOCAL_BIN}/$CLI_BINARY"
  if [[ -f "$SCRIPT_DIR/$BIN_DIR/$SERVER_BINARY" ]]; then
    echo "Copying $SERVER_BINARY to ${LOCAL_BIN}/$SERVER_BINARY"
    cp -f "$SCRIPT_DIR/$BIN_DIR/$SERVER_BINARY" "${LOCAL_BIN}/$SERVER_BINARY"
    chmod +x "${LOCAL_BIN}/$SERVER_BINARY"
  else
    echo "Note: $SERVER_BINARY not found, skip. Run './make build' to build it."
  fi
  if [[ -f "$SCRIPT_DIR/$BIN_DIR/$WORKER_BINARY" ]]; then
    echo "Copying $WORKER_BINARY to ${LOCAL_BIN}/$WORKER_BINARY"
    cp -f "$SCRIPT_DIR/$BIN_DIR/$WORKER_BINARY" "${LOCAL_BIN}/$WORKER_BINARY"
    chmod +x "${LOCAL_BIN}/$WORKER_BINARY"
  else
    echo "Note: $WORKER_BINARY not found, skip. Run './make build' to build it."
  fi
  if [[ -f "$SCRIPT_DIR/$BIN_DIR/$DESKTOP_BINARY" ]]; then
    echo "Copying $DESKTOP_BINARY to ${LOCAL_BIN}/$DESKTOP_BINARY"
    cp -f "$SCRIPT_DIR/$BIN_DIR/$DESKTOP_BINARY" "${LOCAL_BIN}/$DESKTOP_BINARY"
    chmod +x "${LOCAL_BIN}/$DESKTOP_BINARY"
  else
    echo "Note: $DESKTOP_BINARY not found, skip. Run './make build' to build it."
  fi
  local LOCAL_BIN_IN_PATH=0
  case ":$PATH:" in
    *":${LOCAL_BIN}:"*) LOCAL_BIN_IN_PATH=1 ;;
  esac
  if [[ "$LOCAL_BIN_IN_PATH" -eq 0 ]]; then
    echo ""
    echo "$LOCAL_BIN is not in your PATH."
    echo "To use buildmax from any directory, add it to your PATH:"
    echo "  export PATH=\"\$HOME/.local/bin:\$PATH\""
    echo ""
    echo "To make it permanent, add the line above to your shell config:"
    if [[ -n "$ZSH_VERSION" ]] || [[ -f "$HOME/.zshrc" ]]; then
      echo "  echo 'export PATH=\"\$HOME/.local/bin:\$PATH\"' >> ~/.zshrc"
    fi
    if [[ -n "$BASH_VERSION" ]] || [[ -f "$HOME/.bashrc" ]]; then
      echo "  echo 'export PATH=\"\$HOME/.local/bin:\$PATH\"' >> ~/.bashrc"
    fi
    echo ""
    echo "After adding to PATH, start a new terminal session or run: source ~/.zshrc (or ~/.bashrc)"
  else
    echo ""
    echo "$LOCAL_BIN is already in your PATH."
    echo "$CLI_BINARY, $SERVER_BINARY, $WORKER_BINARY, and $DESKTOP_BINARY are now available from any directory."
  fi
  echo ""
  echo "Installation complete!"
  echo "You can run '$CLI_BINARY' (CLI), '$SERVER_BINARY', '$WORKER_BINARY', and '$DESKTOP_BINARY' from any directory (after updating PATH if needed)."
}

cmd_pub_images() {
  local cluster_name="${BUILDMAX_KIND_CLUSTER:-buildmaxdev}"
  local platform_args=()
  if [[ -n "${BUILDMAX_IMAGE_PLATFORM:-}" ]]; then
    platform_args=(--platform "$BUILDMAX_IMAGE_PLATFORM")
  fi
  echo "Building BuildMax image (buildmax:local)..."
  if ! docker build -f "$SCRIPT_DIR/Dockerfile.buildmax" "${platform_args[@]}" -t buildmax:local "$SCRIPT_DIR"; then
    echo "Error: docker build failed"
    return 1
  fi
  echo "Building Portal image (buildmax-portal:local)..."
  if ! docker build -f "$SCRIPT_DIR/Dockerfile.portal" "${platform_args[@]}" -t buildmax-portal:local "$SCRIPT_DIR"; then
    echo "Error: portal docker build failed"
    return 1
  fi
  echo "Loading images into kind cluster '$cluster_name'..."
  if ! kind load docker-image buildmax:local --name "$cluster_name"; then
    echo "Error: kind load buildmax:local failed"
    return 1
  fi
  if ! kind load docker-image buildmax-portal:local --name "$cluster_name"; then
    echo "Error: kind load buildmax-portal:local failed"
    return 1
  fi
  echo "Done. Images buildmax:local and buildmax-portal:local are available in kind."
}

cmd_deploy() {
  echo "Deploying buildmax server to kind cluster..."
  cmd_build || return 1
  cmd_pub_images || return 1
  echo "Ensuring namespace buildmax..."
  if ! kubectl create namespace buildmax --dry-run=client -o yaml | kubectl apply -f -; then
    echo "Error: could not create namespace buildmax"
    return 1
  fi
  local secret_file="$SCRIPT_DIR/deployment/buildmax-secret.local.yaml"
  if [ -f "$secret_file" ]; then
    echo "Applying buildmax-secret.local.yaml..."
    if ! kubectl apply -f "$secret_file"; then
      echo "Error: kubectl apply of buildmax-secret.local.yaml failed"
      return 1
    fi
  elif kubectl get secret buildmax-secret -n buildmax >/dev/null 2>&1; then
    echo "Using existing buildmax-secret in the cluster (no local secret file)."
  else
    echo "Error: no credentials for the deployment."
    echo "  cp deployment/buildmax-secret.example.yaml deployment/buildmax-secret.local.yaml"
    echo "  # fill in real values, then re-run ./make deploy"
    return 1
  fi
  echo "Applying buildmax-deploy.yaml..."
  if ! kubectl apply -f "$SCRIPT_DIR/deployment/buildmax-deploy.yaml"; then
    echo "Error: kubectl apply failed"
    return 1
  fi
  echo "Restarting deployments..."
  # Restart server
  if ! kubectl rollout restart deployment buildmax-server -n buildmax; then
    echo "Error: kubectl rollout restart failed"
    return 1
  fi
  # Restart portal
  if ! kubectl rollout restart deployment buildmax-portal -n buildmax; then
    echo "Error: kubectl rollout restart failed"
    return 1
  fi
  echo "Deployed. Add to /etc/hosts: 127.0.0.1 buildmax-api.kind.local buildmax.kind.local"
  echo "Then open the portal: http://buildmax.kind.local"
}

cmd="${1:-}"

if [[ -z "$cmd" ]]; then
  usage
  exit 0
fi

case "$cmd" in
  build)
    cmd_build "${2:-}"
    ;;
  clean)
    cmd_clean
    ;;
  test)
    cmd_test
    ;;
  smoke)
    cmd_smoke
    ;;
  eval)
    mkdir -p "$SCRIPT_DIR/$BIN_DIR"
    go build -o "$SCRIPT_DIR/$BIN_DIR/buildmax-eval" ./cmd/buildmax-eval
    "$SCRIPT_DIR/$BIN_DIR/buildmax-eval" "${@:2}"
    ;;
  run)
    case "${2:-}" in
      server)
        cmd_run_server
        ;;
      cli)
        cmd_run_cli "$@"
        ;;
      desktop)
        cmd_run_desktop
        ;;
      portal)
        cmd_run_portal
        ;;
      *)
        echo "Usage: ./make run <subcommand>"
        echo "  server   Run buildmax-server  (BUILDMAX_HOME=./testing-sandbox)"
        echo "  cli      Run buildmax         (BUILDMAX_HOME=./testing-sandbox)"
        echo "  desktop  Run buildmax-desktop (BUILDMAX_HOME=./testing-sandbox)"
        echo "  portal   Start Portal dev server (Vite)"
        exit 1
        ;;
    esac
    ;;
  bump)
    cmd_bump_version "${2:-patch}"
    ;;
  install)
    cmd_install
    ;;
  setup)
    cmd_setup
    ;;
  unsetup)
    cmd_unsetup
    ;;
  pub_images)
    cmd_pub_images
    ;;
  deploy)
    cmd_deploy
    ;;
  -h|--help|help)
    usage
    exit 0
    ;;
  *)
    echo "Unknown command: $cmd"
    echo ""
    usage
    exit 1
    ;;
esac
