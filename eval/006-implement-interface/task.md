---
id: 006-implement-interface
title: Implement all methods of a filesystem-backed key-value store
timeout: 150
---

The file `store.go` defines a `Store` interface and a `FileStore` struct.
All four methods (`Set`, `Get`, `Delete`, `Exists`) are stubbed with
`errors.New("not implemented")` or `return false`.

Implement each method so the tests in `store_test.go` pass:

- `Set(key, value string) error` — write value to a file named `key` inside `f.Dir`
- `Get(key string) (string, error)` — read the file; return `ErrNotFound` if it doesn't exist
- `Delete(key string) error` — remove the file; return `ErrNotFound` if it doesn't exist
- `Exists(key string) bool` — report whether the file exists

Use `os` and `path/filepath` from the standard library. Do not add new dependencies.

---grader---

go test .
