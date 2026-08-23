#!/bin/sh
# The reference solution, run from the trial workspace.
#
# Preflight requires this to pass every required grader. A task whose own oracle
# fails is measuring its graders rather than the subject, and one whose oracle
# passes against the untouched initial state is asking for something already
# true.

total=$(awk -F, 'NR > 1 { sum += $3 } END { print sum }' sales.csv)
top=$(awk -F, 'NR > 1 { by[$1] += $3 } END {
	best = ""; best_value = -1
	for (region in by) if (by[region] > best_value) { best = region; best_value = by[region] }
	print best
}' sales.csv)

cat > report.md <<EOF
# Sales Report

Total revenue across every region: ${total}

Highest-revenue region: ${top}
EOF
