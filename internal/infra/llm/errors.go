package llm

import (
	"context"
	"errors"
	"fmt"
)

// apiError is a provider failure reduced to what every caller of this package
// reacts to. Each adapter converts its own library's error into one, so retry
// policy and human-readable classification are written once and cannot mean
// different things for different protocols.
//
// The original error is kept and unwrapped, so a caller that does know a
// specific library's error type can still reach it.
type apiError struct {
	// status is the HTTP status the provider returned, or 0 when the failure
	// happened below HTTP (DNS, connection reset, TLS).
	status int
	// message is the provider's own description, when it gave a safe one.
	message string
	err     error
}

func (e *apiError) Error() string {
	switch {
	case e.message != "" && e.status != 0:
		return fmt.Sprintf("provider error (HTTP %d): %s", e.status, e.message)
	case e.status != 0:
		return fmt.Sprintf("provider error (HTTP %d)", e.status)
	case e.err != nil:
		return e.err.Error()
	default:
		return "provider error"
	}
}

func (e *apiError) Unwrap() error { return e.err }

// requestError is a failure to build a request from the history the caller
// supplied. It never reached a provider, so it is deterministic: retrying it
// would repeat the same failure three times and report it three attempts late.
type requestError struct{ err error }

func (e *requestError) Error() string { return e.err.Error() }

func (e *requestError) Unwrap() error { return e.err }

// classifyLLMError returns a concise, human-readable description of an LLM call error.
// It is used to wrap raw provider errors before they propagate to callers.
func classifyLLMError(err error) string {
	if err == nil {
		return ""
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "LLM call timed out"
	}
	if errors.Is(err, context.Canceled) {
		return "LLM call cancelled"
	}
	var apiErr *apiError
	if errors.As(err, &apiErr) {
		switch apiErr.status {
		case 401, 403:
			return fmt.Sprintf("authentication failed (HTTP %d): check api_key in settings.yaml", apiErr.status)
		case 429:
			return "rate limited by provider: too many requests"
		case 500:
			return "provider internal server error (HTTP 500)"
		case 502:
			return "provider bad gateway (HTTP 502): upstream is unavailable"
		case 503:
			return "provider service unavailable (HTTP 503)"
		case 504:
			return "provider gateway timeout (HTTP 504)"
		case 0:
			// Below HTTP: report what actually failed rather than a status.
			return apiErr.Error()
		default:
			return apiErr.Error()
		}
	}
	return err.Error()
}

// wrapLLMError wraps err with a classified human-readable prefix.
// The original error is preserved via %w so errors.As / errors.Is still work.
func wrapLLMError(err error) error {
	if err == nil {
		return nil
	}
	msg := classifyLLMError(err)
	return fmt.Errorf("%s: %w", msg, err)
}
