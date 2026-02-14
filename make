#!/usr/bin/env bash
set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]:-$0}")" && pwd)"
cd "$SCRIPT_DIR"

BINARY="buildmax"

usage() {
  echo "Usage: ./make <command>"
  echo ""
  echo "Commands:"
  echo "  build   Build $BINARY (output: $SCRIPT_DIR/$BINARY)"
  echo "  test    Run go test with BUILDMAX_HOME=testing-sandbox"
  echo "  smoke   Build, then run with -p \"/smoke 0\" and BUILDMAX_HOME=testing-sandbox"
  echo ""
  echo "Examples:"
  echo "  ./make build"
  echo "  ./make test"
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
  if [[ -f .env ]]; then
    set -a
    # shellcheck source=./.env
    source .env
    set +a
  fi
  mkdir -p testing-sandbox
  export BUILDMAX_HOME="$SCRIPT_DIR/testing-sandbox"
  go build -o "$BINARY" ./cmd/buildmax
  export BUILDMAX_LOG_LEVEL=debug
  echo "Running smoke test..."
  ./"$BINARY" -p "/smoke 0"
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
