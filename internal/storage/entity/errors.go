package entity

import "errors"

// ErrEmailExists is returned by CreateUser when the email is already registered.
var ErrEmailExists = errors.New("email already exists")
