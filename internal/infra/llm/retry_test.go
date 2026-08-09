package llm

import (
	"context"
	"errors"
	"net/http"
	"testing"

	openai "github.com/sashabaranov/go-openai"
)

func apiErr(status int) *openai.APIError {
	return &openai.APIError{HTTPStatusCode: status}
}

func TestIsRetryableError_NilIsNotRetryable(t *testing.T) {
	if isRetryableError(nil) {
		t.Error("nil error should not be retryable")
	}
}

func TestIsRetryableError_ContextCancelledNotRetryable(t *testing.T) {
	if isRetryableError(context.Canceled) {
		t.Error("context.Canceled should not be retryable")
	}
	if isRetryableError(context.DeadlineExceeded) {
		t.Error("context.DeadlineExceeded should not be retryable")
	}
}

func TestIsRetryableError_AuthNotRetryable(t *testing.T) {
	for _, code := range []int{http.StatusUnauthorized, http.StatusForbidden} {
		if isRetryableError(apiErr(code)) {
			t.Errorf("HTTP %d should not be retryable (auth error)", code)
		}
	}
}

func TestIsRetryableError_BadRequestNotRetryable(t *testing.T) {
	if isRetryableError(apiErr(http.StatusBadRequest)) {
		t.Error("HTTP 400 should not be retryable")
	}
}

func TestIsRetryableError_RateLimitAndServerErrorsAreRetryable(t *testing.T) {
	for _, code := range []int{429, 500, 502, 503, 504} {
		if !isRetryableError(apiErr(code)) {
			t.Errorf("HTTP %d should be retryable", code)
		}
	}
}

func TestIsRetryableError_NetworkErrorIsRetryable(t *testing.T) {
	netErr := errors.New("connection refused")
	if !isRetryableError(netErr) {
		t.Error("generic network error should be retryable")
	}
}
