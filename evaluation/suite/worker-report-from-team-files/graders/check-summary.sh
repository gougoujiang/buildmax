#!/bin/sh
# Checks the summary a worker run produced.
#
# The working directory is the run's workspace. The team's files are under
# home/, because that is where a worker materializes a team's persistent
# workspace — the same task written for the CLI would find them at the root.
#
# Expected from state/backlog.md: 4 open items, and the export restore is the
# urgent one.

fail() {
	echo "$1"
	exit 1
}

[ -f summary.md ] || fail "summary.md does not exist"

# The team's file must still be there: summarising it is not a licence to
# consume it.
[ -f home/backlog.md ] || fail "home/backlog.md is gone; the run consumed the team's file"

grep -q '4' summary.md || fail "summary.md does not state that 4 items are open"

if ! grep -Eqi 'export|nightly' summary.md; then
	fail "summary.md does not name the urgent item (the nightly export)"
fi

echo "summary.md reports the open count and the urgent item"
