package identity

import "errors"

// These are the two account facts a store reports as an error rather than a
// value: a caller asking for an account that is not there gets nil, and only
// an operation whose sole return is an error needs a sentinel.

// ErrEmailExists is returned by CreateUser when the email is already registered.
var ErrEmailExists = errors.New("email already exists")

// ErrUserNotFound is returned when an operation names an account that is not there.
var ErrUserNotFound = errors.New("user not found")
