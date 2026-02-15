#!/usr/bin/env bash
set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]:-$0}")" && pwd)"
cd "$SCRIPT_DIR"

BINARY="buildmax"

# Load .env into environment for all commands (Docker Compose, smoke, etc.)
if [[ -f .env ]]; then
  set -a
  # shellcheck source=./.env
  source .env
  set +a
fi

usage() {
  echo "Usage: ./make <command>"
  echo ""
  echo "Commands:"
  echo "  build         Build $BINARY (output: $SCRIPT_DIR/$BINARY)"
  echo "  test          Run go test with BUILDMAX_HOME=testing-sandbox"
  echo "  smoke         Build, then run with -p \"/smoke 0\" and BUILDMAX_HOME=testing-sandbox"
  echo "  run server    Build (if needed) and start HTTP server for local testing (default port 5678)"
  echo "  run portal    Start Portal dev server (Vite; installs deps if needed)"
  echo "  bump          Bump Version in internal/cmd/root.go (arg: patch|minor|major, default: patch)"
  echo "  up            Start Docker Compose services (e.g. MySQL) for local dev"
  echo "  down          Stop Docker Compose services"
  echo "  docker-logs   Show Docker Compose service logs (follow)"
  echo ""
  echo "Examples:"
  echo "  ./make build"
  echo "  ./make test"
  echo "  ./make up            # start MySQL etc."
  echo "  ./make down          # stop containers"
  echo "  ./make bump        # 0.0.2 -> 0.0.3"
  echo "  ./make bump minor  # 0.0.2 -> 0.1.0"
  echo "  ./make run server  # start backend server (port 5678)"
  echo "  ./make run portal # start Portal dev server (Vite)"
}

cmd_run_server() {
  go build -o "$BINARY" ./cmd/buildmax
  echo "Starting server (Ctrl+C to stop)..."
  ./"$BINARY" server
}

cmd_run_portal() {
  if [[ ! -d portal ]]; then
    echo "Error: portal/ directory not found"
    return 1
  fi
  if [[ ! -d portal/node_modules ]]; then
    echo "Installing portal dependencies..."
    (cd portal && npm install)
  fi
  echo "Starting Portal dev server (Ctrl+C to stop)..."
  (cd portal && npm run dev)
}

cmd_build() {
  echo "Building $BINARY..."
  if go build -o "$BINARY" ./cmd/buildmax; then
    echo "Built $BINARY at $SCRIPT_DIR/$BINARY"
  else
    return 1
  fi
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
  go build -o "$BINARY" ./cmd/buildmax
  export BUILDMAX_LOG_LEVEL=debug
  echo "Running smoke test..."
  ./"$BINARY" -p "/smoke 0"
}

cmd_bump_version() {
  local root_go="$SCRIPT_DIR/internal/cmd/root.go"
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

cmd_docker_up() {
  echo "Starting Docker Compose services..."
  docker compose up -d
}

cmd_docker_down() {
  echo "Stopping Docker Compose services..."
  docker compose down
}

cmd_docker_logs() {
  docker compose logs -f
}

cmd="${1:-}"

if [[ -z "$cmd" ]]; then
  usage
  exit 0
fi

case "$cmd" in
  build)
    cmd_build
    ;;
  test)
    cmd_test
    ;;
  smoke)
    cmd_smoke
    ;;
  run)
    case "${2:-}" in
      server)
        cmd_run_server
        ;;
      portal)
        cmd_run_portal
        ;;
      *)
        echo "Usage: ./make run <subcommand>"
        echo "  server  Start HTTP server for local testing (default port 5678)"
        echo "  portal  Start Portal dev server (Vite)"
        exit 1
        ;;
    esac
    ;;
  bump)
    cmd_bump_version "${2:-patch}"
    ;;
  up)
    cmd_docker_up
    ;;
  down)
    cmd_docker_down
    ;;
  docker-logs)
    cmd_docker_logs
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
