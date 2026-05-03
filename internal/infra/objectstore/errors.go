package objectstore

import "errors"

// ErrNotFound is returned when a requested object does not exist in the store.
var ErrNotFound = errors.New("object not found")
