---
id: 005-write-test
title: Write tests for existing functions
timeout: 120
---

The file `math.go` contains three functions: `Add`, `Subtract`, and `Abs`.
The file `math_test.go` only has a test for `Subtract`.

Add test functions `TestAdd` and `TestAbs` to `math_test.go`.
Each test must cover at least two different inputs including edge cases
(e.g., zero, negative numbers).

---grader---

go test -run "TestAdd|TestAbs" .
