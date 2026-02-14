#!/usr/bin/env bash
# Install buildmax locally to ~/.local/bin
# This allows testing with other projects

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]:-$0}")" && pwd)"
SOURCE_BIN="${SCRIPT_DIR}/buildmax"
LOCAL_BIN="${HOME}/.local/bin"
DEST_BIN="${LOCAL_BIN}/buildmax"

echo "Installing buildmax to ${LOCAL_BIN}..."

if [ ! -f "$SOURCE_BIN" ]; then
  echo "Error: buildmax not found in the current directory: $SCRIPT_DIR" >&2
  echo "Run './make.sh build' or 'go build -o buildmax ./cmd/buildmax' first." >&2
  exit 1
fi

mkdir -p "$LOCAL_BIN"
echo "Copying buildmax to $DEST_BIN"
cp -f "$SOURCE_BIN" "$DEST_BIN"
chmod +x "$DEST_BIN"

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
  echo "buildmax is now available from any directory."
fi

echo ""
echo "Installation complete!"
echo "You can now run 'buildmax' from any directory (after updating PATH if needed)."
