---
name: review
description: "Review a code change against a structured checklist: correctness, readability, tests, and security. Use before opening or approving a pull request."
---

# Code Review

Review the change under discussion against the checklist below. Work through each
section in order and record a finding for anything that fails, with a
`file:line` reference so the author can jump straight to it.

## Scope First

Identify what actually changed before judging it. Read the diff, then read
enough of the surrounding code to understand the intent. A review that only reads
added lines misses the caller that no longer holds.

## Checklist

### Correctness

- Does the change do what its description says, and only that?
- Are edge cases handled: empty input, nil/None, zero, overflow, concurrency?
- Are errors propagated rather than swallowed?

### Readability

- Do names say what the thing is or does?
- Is there dead code, a stray debug print, or a commented-out block?
- Do comments explain *why*, not restate the code?

### Tests

- Is the new behavior covered by a test that would fail without the change?
- Are failure paths tested, not only the happy path?

### Security

- Is untrusted input validated before use?
- Are secrets kept out of logs, errors, and source?

## Report

Group findings by severity — blocking, should-fix, nit — and lead with a
one-line summary of whether the change is ready to merge.
