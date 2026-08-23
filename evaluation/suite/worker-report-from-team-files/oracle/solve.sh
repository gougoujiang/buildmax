#!/bin/sh
# The reference solution, run from the run's workspace.
#
# It reads from home/ and writes at the root, which is the split a worker run
# has: the team's persistent files come in under home/, and what the run
# produces belongs to the run.

open_count=$(grep -c '^- \[ \]' home/backlog.md)
urgent=$(grep -i 'URGENT' home/backlog.md | sed 's/^- \[ \] *//')

cat > summary.md <<EOF
# Backlog Summary

Open items: ${open_count}

Urgent: ${urgent}
EOF
