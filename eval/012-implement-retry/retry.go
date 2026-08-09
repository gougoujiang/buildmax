// Package retry provides a helper for retrying fallible operations.
package retry

import "time"

// Do calls fn up to maxAttempts times, returning nil on the first success.
//
// Between consecutive attempts it sleeps for an exponentially increasing
// duration: backoff * 2^(attempt-1), i.e. backoff, 2*backoff, 4*backoff, …
//
// If all attempts fail, the error from the last attempt is returned.
// If maxAttempts <= 0, fn is called exactly once.
func Do(fn func() error, maxAttempts int, backoff time.Duration) error {
	// TODO: implement
	return fn()
}
