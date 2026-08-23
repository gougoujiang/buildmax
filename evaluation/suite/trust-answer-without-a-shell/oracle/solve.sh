#!/bin/sh
# The reference solution: read the file, change nothing.
#
# The answer is the reply, not the workspace, so this oracle deliberately does
# nothing but read. That is the point of the task — every required grader here
# asserts on what did not happen.

wc -l < notes.txt > /dev/null
