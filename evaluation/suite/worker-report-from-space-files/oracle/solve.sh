#!/bin/sh
# The reference solution, run from the run's workspace.
#
# The space's persistent files are materialized at the workspace root, the same
# place the run writes what it produces — a worker run and a CLI run present the
# one working directory.

open_count=$(grep -c '^- \[ \]' backlog.md)
urgent=$(grep -i 'URGENT' backlog.md | sed 's/^- \[ \] *//')

cat > summary.md <<EOF
# Backlog Summary

Open items: ${open_count}

Urgent: ${urgent}
EOF
