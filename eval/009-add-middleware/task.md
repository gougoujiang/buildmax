---
id: 009-add-middleware
title: Implement a request-ID middleware for an HTTP router
timeout: 150
---

The file `middleware.go` contains a stub `RequestID` middleware function that
currently does nothing (it just returns `next` unchanged).

Implement `RequestID` so that it:
1. Generates a unique string ID for each incoming request.
2. Sets that ID as the `X-Request-ID` response header before serving.
3. Calls `next.ServeHTTP` to forward the request to the next handler.

The ID must be non-empty and distinct across requests.
You may use `fmt`, `sync/atomic`, `math/rand`, or `crypto/rand` — no external packages.

Read `router.go` to understand the existing handler structure.
Do not modify `router.go` or `middleware_test.go`.

---grader---

go test .
