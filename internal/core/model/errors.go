package model

import "errors"

// ErrEmailExists is returned by CreateUser when the email is already registered.
var ErrEmailExists = errors.New("email already exists")

// ErrUserNotFound is returned when an operation names an account that is not there.
var ErrUserNotFound = errors.New("user not found")

// ErrRunInProgress is returned by CreateTaskRun when the task already has a run in PENDING, SCHEDULED, or RUNNING.
var ErrRunInProgress = errors.New("task has a run already in progress")
