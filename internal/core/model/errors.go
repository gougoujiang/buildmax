package model

import "errors"

// ErrEmailExists is returned by CreateUser when the email is already registered.
var ErrEmailExists = errors.New("email already exists")

// ErrRunInProgress is returned by CreateTaskRun when the task already has a run in PENDING, SCHEDULED, or RUNNING.
var ErrRunInProgress = errors.New("task has a run already in progress")
