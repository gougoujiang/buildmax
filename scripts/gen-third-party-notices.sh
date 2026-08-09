#!/usr/bin/env bash
# Produce NOTICE-THIRD-PARTY: the license text of every module linked into the
# released binaries.
#
# Apache-2.0 section 4(d) requires carrying forward the attribution notices of
# Apache-2.0 dependencies when redistributing them, and compiled Go binaries
# contain that dependency code. The release archives and container images ship
# the generated file for that reason. See docs/dependency-licenses.md.
#
# Usage: scripts/gen-third-party-notices.sh [output-file]
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]:-$0}")/.." && pwd)"
cd "$SCRIPT_DIR"

OUT="${1:-NOTICE-THIRD-PARTY}"

if ! command -v go-licenses &>/dev/null; then
  if [[ -x "$(go env GOPATH)/bin/go-licenses" ]]; then
    PATH="$PATH:$(go env GOPATH)/bin"
  else
    echo "error: go-licenses not found. Install it with:" >&2
    echo "  go install github.com/google/go-licenses@latest" >&2
    exit 1
  fi
fi

WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT

# go-licenses refuses to write into an existing directory, so name a child path.
SAVE="$WORK/licenses"
go-licenses save ./cmd/... --save_path="$SAVE" --force 2>/dev/null

{
  echo "BuildMax third-party notices"
  echo
  echo "BuildMax itself is licensed under Apache-2.0; see LICENSE."
  echo "The binaries in this distribution statically link the modules below."
  echo "Each module's own license text follows, unmodified."
  echo
  echo "A summary of the license mix, and how to regenerate this file, is in"
  echo "docs/dependency-licenses.md."
  echo
} > "$OUT"

# Walk deterministically so regenerating produces a byte-identical file.
find "$SAVE" -type f \( -name 'LICENSE*' -o -name 'COPYING*' -o -name 'NOTICE*' \) \
  | LC_ALL=C sort \
  | while read -r lic; do
      module="${lic#"$SAVE"/}"
      module="$(dirname "$module")"
      {
        echo "================================================================"
        echo "$module"
        echo "================================================================"
        echo
        cat "$lic"
        echo
      } >> "$OUT"
    done

count=$(grep -c '^================================================================$' "$OUT" || true)
echo "wrote $OUT ($((count / 2)) modules)"
