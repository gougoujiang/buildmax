package apierr

import "errors"

// ErrNotFound is what a store says when the row or object a caller named does
// not exist.
//
// It lives here rather than with any one domain because every store speaks it
// and no capability owns it: "the thing you asked for is not there" is a fact
// about the persistence boundary, not about teams, tasks, or plugins. A store
// method whose only return is an error uses it; one that can return a nil value
// keeps doing that instead.
//
// It deliberately carries no Kind. A Kind would make WriteServiceError answer
// 404 for any store miss that reached a transport unhandled, which is a change
// to what those routes reply and, on a route that refuses with 404 to avoid
// confirming an ID exists, a change to what they disclose. Whether it should
// carry one is worth deciding on its own.
var ErrNotFound = errors.New("not found")
