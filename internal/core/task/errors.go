package task

import "errors"

// ErrRunInProgress is returned by CreateTaskRun when the task already has a run in PENDING, SCHEDULED, or RUNNING.
var ErrRunInProgress = errors.New("task has a run already in progress")

// ErrInvalidRunTransition is returned when a caller asks a task run to move
// between statuses that are not adjacent in its lifecycle.
var ErrInvalidRunTransition = errors.New("invalid task run status transition")

// ErrRunCanceled is the reason a canceled run's context carries, and what
// RunTask returns for a run that stopped because someone asked it to. It marks
// an outcome, not a fault: a worker that returns it did what it was told.
var ErrRunCanceled = errors.New("task run canceled")

// ErrRunInterrupted is the reason a run's context carries when the process
// executing it was asked to stop — SIGTERM from a node drain, an eviction, or
// an operator restarting the deployment.
//
// It is deliberately distinct from ErrRunCanceled: nobody asked this run to
// stop, and it is equally not the run failing at its work. What it buys is a
// run that says what happened while it still can, instead of staying RUNNING
// until the stale-run reaper closes it hours later. See
// docs/design/graceful-shutdown.md §6.2.
var ErrRunInterrupted = errors.New("task run interrupted: the worker was shut down")
