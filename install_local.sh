#!/usr/bin/env bash
# Install buildmax (CLI), buildmax-server, and buildmax-worker locally to ~/.local/bin
# This allows testing with other projects

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]:-$0}")" && pwd)"
LOCAL_BIN="${HOME}/.local/bin"

SOURCE_CLI="${SCRIPT_DIR}/buildmax"
SOURCE_SERVER="${SCRIPT_DIR}/buildmax-server"
SOURCE_WORKER="${SCRIPT_DIR}/buildmax-worker"

echo "Installing BuildMax binaries to ${LOCAL_BIN}..."

if [ ! -f "$SOURCE_CLI" ]; then
  echo "Error: buildmax not found in the current directory: $SCRIPT_DIR" >&2
  echo "Run './make build' or 'go build -o buildmax ./cmd/buildmax' first." >&2
  exit 1
fi

mkdir -p "$LOCAL_BIN"

echo "Copying buildmax to ${LOCAL_BIN}/buildmax"
cp -f "$SOURCE_CLI" "${LOCAL_BIN}/buildmax"
chmod +x "${LOCAL_BIN}/buildmax"

if [ -f "$SOURCE_SERVER" ]; then
  echo "Copying buildmax-server to ${LOCAL_BIN}/buildmax-server"
  cp -f "$SOURCE_SERVER" "${LOCAL_BIN}/buildmax-server"
  chmod +x "${LOCAL_BIN}/buildmax-server"
else
  echo "Note: buildmax-server not found, skip. Run './make build' to build it."
fi

if [ -f "$SOURCE_WORKER" ]; then
  echo "Copying buildmax-worker to ${LOCAL_BIN}/buildmax-worker"
  cp -f "$SOURCE_WORKER" "${LOCAL_BIN}/buildmax-worker"
  chmod +x "${LOCAL_BIN}/buildmax-worker"
else
  echo "Note: buildmax-worker not found, skip. Run './make build' to build it."
fi

# Check if .local/bin is already in PATH
case ":$PATH:" in
  *":${LOCAL_BIN}:"*) LOCAL_BIN_IN_PATH=1 ;;
  *) LOCAL_BIN_IN_PATH=0 ;;
esac

if [ "$LOCAL_BIN_IN_PATH" -eq 0 ]; then
  echo ""
  echo "$LOCAL_BIN is not in your PATH."
  echo "To use buildmax from any directory, add it to your PATH:"
  echo "  export PATH=\"\$HOME/.local/bin:\$PATH\""
  echo ""
  echo "To make it permanent, add the line above to your shell config:"
  if [ -n "$ZSH_VERSION" ] || [ -f "$HOME/.zshrc" ]; then
    echo "  echo 'export PATH=\"\$HOME/.local/bin:\$PATH\"' >> ~/.zshrc"
  fi
  if [ -n "$BASH_VERSION" ] || [ -f "$HOME/.bashrc" ]; then
    echo "  echo 'export PATH=\"\$HOME/.local/bin:\$PATH\"' >> ~/.bashrc"
  fi
  echo ""
  echo "After adding to PATH, start a new terminal session or run: source ~/.zshrc (or ~/.bashrc)"
else
  echo ""
  echo "$LOCAL_BIN is already in your PATH."
  echo "buildmax, buildmax-server, and buildmax-worker are now available from any directory."
fi

echo ""
echo "Installation complete!"
echo "You can run 'buildmax' (CLI), 'buildmax-server', and 'buildmax-worker' from any directory (after updating PATH if needed)."
