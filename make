#!/usr/bin/env bash
# BuildMax cross-platform task runner shim.
# Shim for the task runner. Every task lives in tools/mk (Go) so that macOS,
# Linux, and Windows contributors run the same code; make.bat is the same shim
# for cmd.exe. Run `./make help` for the command list.
set -e
cd "$(dirname "${BASH_SOURCE[0]:-$0}")"

# `./make doctor` cannot be the thing that reports a missing Go: the task runner
# is Go, so without this guard a new contributor's first command fails as
# `exec: go: not found` with nothing to act on. Go is the one prerequisite
# nothing in this repository can install for you.
if ! command -v go >/dev/null 2>&1; then
	want=$(sed -n 's/^go \([0-9].*\)$/\1/p' go.mod)
	echo "BuildMax needs Go ${want:-(the version in go.mod)}, and 'go' is not on your PATH." >&2
	# Not a package-manager command: distribution packages are routinely years
	# behind the go directive, so the download page is the answer that keeps
	# working. Homebrew tracks upstream, so it is safe to name on macOS.
	echo "Install it from https://go.dev/dl/ — distribution packages are often older." >&2
	if [ "$(uname -s 2>/dev/null)" = "Darwin" ]; then
		echo "On macOS: brew install go" >&2
	fi
	exit 1
fi

exec go run ./tools/mk "$@"
