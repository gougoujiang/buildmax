---
id: 008-fix-multiple-bugs
title: Find and fix three bugs to make all tests pass
timeout: 150
---

The file `math.go` contains three functions — `Divide`, `Pow`, and `Clamp` —
each with exactly one bug. The test file `math_test.go` exposes all three.

Run `go test .` to see the failures, then fix each bug in `math.go`.

Hints (read the code carefully before applying these):
- `Divide` should reject a zero divisor.
- `Pow` has an incorrect base case for `exp == 0`.
- `Clamp` returns the wrong bound when the value is out of range.

Do not modify `math_test.go`.

---grader---

go test .
