---
id: 010-implement-pagination
title: Implement offset/limit pagination on an in-memory store
timeout: 150
---

The file `store.go` contains a `Store` with a `Page(offset, limit int) []Item`
method that currently returns an empty slice regardless of arguments.

Implement `Page` so it:
- Returns at most `limit` items starting at zero-based `offset`.
- Returns an empty slice (not nil) when `offset >= Count()` or `limit <= 0`.
- Returns only the remaining items when `offset + limit > Count()`.

Read the existing `List()` and `Count()` methods for context.
Do not modify `store_test.go`.

---grader---

go test .
