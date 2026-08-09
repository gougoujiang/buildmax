---
id: 012-implement-retry
title: Implement exponential backoff retry
timeout: 150
---

The file `retry.go` contains a stub `Do` function that calls `fn` once and
returns — it does not retry or apply any backoff.

Implement `Do` with the following behaviour:

- Call `fn` up to `maxAttempts` times (if `maxAttempts <= 0`, call exactly once).
- Return `nil` immediately on the first successful call.
- Between consecutive attempts sleep for `backoff * 2^(attempt-1)` so the
  wait doubles with each retry: backoff, 2×backoff, 4×backoff, …
- After all attempts are exhausted, return the error from the **last** attempt.

Use only `time.Sleep` and standard arithmetic — no external packages.

Run `go test .` to see the current failures, then fix `retry.go`.

---grader---

go test .
