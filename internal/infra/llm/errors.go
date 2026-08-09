package llm

import (
	"context"
	"errors"
	"fmt"

	openai "github.com/sashabaranov/go-openai"
)

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
	var apiErr *openai.APIError
	if errors.As(err, &apiErr) {
		switch apiErr.HTTPStatusCode {
		case 401, 403:
			return fmt.Sprintf("authentication failed (HTTP %d): check api_key in settings.yaml", apiErr.HTTPStatusCode)
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
		default:
			if apiErr.Message != "" {
				return fmt.Sprintf("provider error (HTTP %d): %s", apiErr.HTTPStatusCode, apiErr.Message)
			}
			return fmt.Sprintf("provider error (HTTP %d)", apiErr.HTTPStatusCode)
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
