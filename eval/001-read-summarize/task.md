---
id: 001-read-summarize
title: Read and summarize code
timeout: 90
---

Read the file `server.go` in your workspace.

Write a one-sentence summary of what the program does to a file called `summary.txt`.
The summary must mention: what protocol it serves on, and what it serves.

---grader---

test -s summary.txt && grep -qi "http" summary.txt && grep -qi "file\|static\|director" summary.txt
