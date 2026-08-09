#!/usr/bin/env bash
# Shim for the task runner. Every task lives in cmd/mk (Go) so that macOS,
# Linux, and Windows contributors run the same code; make.bat is the same shim
# for cmd.exe. Run `./make help` for the command list.
set -e
cd "$(dirname "${BASH_SOURCE[0]:-$0}")"
exec go run ./cmd/mk "$@"
