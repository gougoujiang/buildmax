package model

import "errors"

// ErrNotFound is what a store returns when an operation names a row that is not
// there, for the cases where "not there" is the answer rather than an empty
// result. A store that can return a nil value instead should keep doing that;
// this is for the operations whose only return is an error.
//
// It exists so a caller can ask "did that exist?" without importing the
// persistence library to find out. internal/architecture forbids gorm.io
// imports outside internal/infra/db precisely so that translation happens once,
// at the store boundary.
var ErrNotFound = errors.New("not found")

// ErrEmailExists is returned by CreateUser when the email is already registered.
var ErrEmailExists = errors.New("email already exists")

// ErrUserNotFound is returned when an operation names an account that is not there.
var ErrUserNotFound = errors.New("user not found")

// ErrUserDisabled is returned when a credential belongs to an account an
// administrator has disabled. It is deliberately distinguishable from a wrong
// credential: someone who can prove the account is theirs should be told why
// they are being refused, while a wrong password still gets the generic answer.
var ErrUserDisabled = errors.New("account is disabled")

// ErrRunInProgress is returned by CreateTaskRun when the task already has a run in PENDING, SCHEDULED, or RUNNING.
var ErrRunInProgress = errors.New("task has a run already in progress")

// ErrRunCanceled is the reason a canceled run's context carries, and what
// RunTask returns for a run that stopped because someone asked it to. It marks
// an outcome, not a fault: a worker that returns it did what it was told.
var ErrRunCanceled = errors.New("task run canceled")
