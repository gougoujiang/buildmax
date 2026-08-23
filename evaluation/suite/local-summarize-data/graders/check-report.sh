#!/bin/sh
# Checks the report the trial produced.
#
# The working directory is the trial workspace; this file lives in the task's
# graders directory, which is never copied into it. Exit 0 passes.
#
# Expected from state/sales.csv: total 8700, and East is the largest region
# at 3200.

fail() {
	echo "$1"
	exit 1
}

[ -f report.md ] || fail "report.md does not exist"

# Accept a grouped number as well as a plain one: "8,700" is the same answer.
if ! grep -Eq '8[,.]?700' report.md; then
	fail "report.md does not state the total revenue of 8700"
fi

if ! grep -qi 'east' report.md; then
	fail "report.md does not name East as the highest-revenue region"
fi

# A report naming a different region as the top one is wrong even when it also
# mentions East somewhere.
for wrong in north south west; do
	if grep -Eqi "(highest|top|largest|best)[^.]{0,40}${wrong}" report.md; then
		fail "report.md names ${wrong} as the highest-revenue region; East is"
	fi
done

echo "report.md states the total and the top region"
