package server

import "time"

// DefaultShutdownGrace is the whole budget for an orderly stop when the
// deployment configures none.
//
// It sits under the Kubernetes default terminationGracePeriodSeconds of 30 with
// room for a preStop hook, so a deployment that changed nothing still finishes
// its ladder before the kubelet kills it.
const DefaultShutdownGrace = 25 * time.Second

// minShutdownGrace floors a misconfigured value. Below this the phases round
// down to nothing and the ladder becomes an immediate exit wearing a costume.
const minShutdownGrace = 2 * time.Second

// ShutdownBudget divides the grace period among the rungs of the shutdown
// ladder. Derived rather than configured: an operator sets one number, and the
// phases keep their proportions to each other automatically.
//
// See docs/design/graceful-shutdown.md §3.
type ShutdownBudget struct {
	// Workers is how long in-flight runs have to stop and report. It is the
	// largest share because it is the only phase that waits on another process.
	Workers time.Duration
	// Streams is how long watcher streams have to notice the drain and return.
	// They are selecting on a closed channel, so this is generous already.
	Streams time.Duration
	// Requests is how long ordinary in-flight requests have to finish.
	Requests time.Duration
	// Background is how long the background loops have to end their current
	// pass.
	Background time.Duration
}

// NewShutdownBudget splits grace into the ladder's phases. A zero or negative
// grace uses the default; anything under minShutdownGrace is raised to it,
// because a budget too small to divide is a configuration mistake rather than a
// request to skip the ladder.
func NewShutdownBudget(grace time.Duration) ShutdownBudget {
	if grace <= 0 {
		grace = DefaultShutdownGrace
	}
	if grace < minShutdownGrace {
		grace = minShutdownGrace
	}
	return ShutdownBudget{
		Workers:    grace * 60 / 100,
		Streams:    grace * 5 / 100,
		Requests:   grace * 25 / 100,
		Background: grace * 10 / 100,
	}
}

// Total is the sum of the phases, which is the grace period rounded down by the
// integer division above.
func (b ShutdownBudget) Total() time.Duration {
	return b.Workers + b.Streams + b.Requests + b.Background
}
