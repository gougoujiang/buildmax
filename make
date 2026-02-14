#!/usr/bin/env bash
set -e

# Load env from .env if present (copy .env.example to .env and fill in keys)
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]:-$0}")" && pwd)"
cd "$SCRIPT_DIR"
# shellcheck source=./loadenv.sh
source ./loadenv.sh 2>/dev/null || true

BINARY="buildmax"

usage() {
  echo "Usage: ./make.sh <command>"
  echo "  build   Build $BINARY"
  echo "  test    Run go test with testing-sandbox as data dir"
  echo "  smoke   Smoke test: build and run with /smoke 0, BUILDMAX_HOME=testing-sandbox"
}

cmd="${1:-}"
case "${cmd}" in
  build)
    go build -o "$BINARY" ./cmd/buildmax
    ;;
  test)
    mkdir -p testing-sandbox
    export BUILDMAX_HOME="$SCRIPT_DIR/testing-sandbox"
    go test ./...
    ;;
  smoke)
    mkdir -p testing-sandbox
    export BUILDMAX_HOME="$SCRIPT_DIR/testing-sandbox"
    go build -o "$BINARY" ./cmd/buildmax
    export BUILDMAX_LOG_LEVEL=debug
    ./"$BINARY" -p "/smoke 0"
    ;;
  *)
    usage
    exit 0
    ;;
esac
