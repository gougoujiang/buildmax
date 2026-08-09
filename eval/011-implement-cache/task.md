---
id: 011-implement-cache
title: Implement a thread-safe in-memory cache with TTL
timeout: 180
---

The file `cache.go` defines a `Cache` struct with stub implementations.
Implement the cache so all tests in `cache_test.go` pass, including
the concurrent-access test which is run with the race detector.

Requirements:
- `New(cleanupInterval)` — initialise the cache; if `cleanupInterval > 0`,
  start a background goroutine that periodically removes expired entries.
- `Set(key, value, ttl)` — store the value; `ttl == 0` means no expiry.
- `Get(key)` — return the value and true, or `"", false` for absent/expired keys.
- `Delete(key)` — remove the key (no-op if absent).
- `Len()` — number of stored entries (may include not-yet-cleaned expired ones).
- `Close()` — stop the background goroutine.

All methods must be safe for concurrent use from multiple goroutines.
Use `sync.RWMutex` or `sync.Mutex` from the standard library.

---grader---

go test -race .
