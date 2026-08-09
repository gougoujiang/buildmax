---
id: 013-worker-pool
title: Implement a fixed-size concurrent worker pool
timeout: 180
---

The file `pool.go` contains a `Pool` struct and four stub methods.
Implement the worker pool so all tests in `pool_test.go` pass under the race detector.

Requirements:

- `New(workers int) *Pool` — start `workers` goroutines that read from a shared
  jobs channel. The channel capacity should be `workers * 4`.
- `Submit(job func())` — send the job to the channel. Track each submitted job
  with a `sync.WaitGroup` so `Wait` knows when they are all done.
- `Wait()` — block until every job submitted so far has finished executing.
- `Close()` — close the jobs channel so workers exit cleanly after draining it,
  then wait for all worker goroutines to stop.

Use only `sync`, `sync/atomic`, and channels from the standard library.
All methods must be safe for concurrent use.

Run `go test -race .` to check correctness and absence of data races.

---grader---

go test -race .
