---
id: 004-fix-bug
title: Find and fix a bug to make a failing test pass
timeout: 120
---

The file `parse.go` has a bug in `ParsePositive`.
The function is supposed to return an error when the input is zero or negative,
but it currently accepts zero as valid.

Run `go test .` to see the failing test, then fix the bug in `parse.go`.

---grader---

go test -run TestParsePositive .
