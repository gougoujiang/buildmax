package llm

import (
	"context"
	"errors"
	"log/slog"
	"time"

	openai "github.com/sashabaranov/go-openai"
)

const maxRetryAttempts = 3

// retryBackoff is the wait between successive attempts (attempt 1→2, 2→3).
var retryBackoff = []time.Duration{time.Second, 2 * time.Second, 4 * time.Second}

// isRetryableError returns true when the error is transient and the call should be retried.
//
// Never retried:
//   - context cancellation or deadline (caller gave up)
//   - auth errors (401, 403) — user action required
//   - bad request (400) — model/request bug, retrying won't help
//
// Retried:
//   - rate limit (429)
//   - server errors (500, 502, 503, 504)
//   - network-level errors (connection refused, DNS, etc.)
func isRetryableError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	var apiErr *openai.APIError
	if errors.As(err, &apiErr) {
		switch apiErr.HTTPStatusCode {
		case 429, 500, 502, 503, 504:
			return true
		}
		return false
	}
	// Network-level errors (DNS failure, connection reset, etc.) are transient.
	return true
}

// sleepWithContext waits for d or until ctx is cancelled, whichever comes first.
func sleepWithContext(ctx context.Context, d time.Duration) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(d):
		return nil
	}
}

// logRetry emits a warning before a retry attempt.
func logRetry(attempt int, err error) {
	delay := retryBackoff[attempt-1]
	slog.Warn("LLM call failed, retrying",
		"attempt", attempt+1,
		"of", maxRetryAttempts,
		"delay", delay,
		"err", err,
	)
}
