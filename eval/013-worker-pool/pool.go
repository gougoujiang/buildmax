// Package workerpool provides a fixed-size goroutine pool for concurrent job processing.
package workerpool

// Pool runs submitted jobs concurrently using a fixed number of worker goroutines.
type Pool struct {
	// TODO: add fields — you will need a jobs channel, a WaitGroup, and
	// something to track whether the pool has been closed.
}

// New creates a Pool with the given number of worker goroutines and starts them
// immediately. workers must be >= 1.
//
// The internal job queue has a capacity of workers * 4 so that Submit can buffer
// a reasonable number of jobs without blocking.
func New(workers int) *Pool {
	// TODO: implement — initialise the pool and start worker goroutines.
	// Each goroutine should range over the jobs channel and call each job func.
	return &Pool{}
}

// Submit enqueues job for execution by one of the worker goroutines.
// Blocks if the internal queue is full.
// Panics if called after Close.
func (p *Pool) Submit(job func()) {
	// TODO: implement
}

// Wait blocks until all jobs that have been submitted so far have finished.
func (p *Pool) Wait() {
	// TODO: implement
}

// Close signals the workers to stop after completing any pending jobs and
// waits for all workers to exit. Calling Submit after Close panics.
func (p *Pool) Close() {
	// TODO: implement
}
