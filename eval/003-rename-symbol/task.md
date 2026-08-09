---
id: 003-rename-symbol
title: Rename a function across multiple files
timeout: 120
---

The workspace contains a Go package with a function called `ComputeSum`.

Rename `ComputeSum` to `Add` across all Go files in the workspace.
Make sure all call sites and the function declaration are updated consistently.

The test file has a `TestAdd` test that calls `Add` — make it pass.

---grader---

go test -run TestAdd . && ! grep -rq "ComputeSum" --include="*.go" .
