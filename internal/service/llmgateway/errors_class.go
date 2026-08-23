package llmgateway

import (
	"context"
	"errors"
)

// Stable error classifications.
//
// These strings are the contract: they reach clients as error codes and land in
// the call ledger. They describe what BuildMax decided, never what an upstream
// provider said, so a provider's error body cannot leak account identifiers,
// endpoints, or request fragments through them.
const (
	ErrorClassTargetNotFound = "target_not_found"
	ErrorClassTargetDisabled = "target_disabled"
	ErrorClassCapability     = "capability_unsupported"
	ErrorClassQuotaExceeded  = "quota_exceeded"
	ErrorClassDuplicateCall  = "duplicate_call"
	ErrorClassInvalidRequest = "invalid_request"
	ErrorClassNotConfigured  = "not_configured"
	ErrorClassCanceled       = "canceled"
	ErrorClassUpstream       = "upstream_error"
	ErrorClassInternal       = "internal_error"
)

// ErrorClassFor maps an error to its stable classification. Anything
// unrecognized is internal rather than upstream, so a new failure mode is
// reported as our problem until someone classifies it.
func ErrorClassFor(err error) string {
	switch {
	case err == nil:
		return ""
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return ErrorClassCanceled
	case errors.Is(err, ErrTargetNotFound), errors.Is(err, ErrCatalogEmpty):
		return ErrorClassTargetNotFound
	case errors.Is(err, ErrTargetDisabled):
		return ErrorClassTargetDisabled
	case errors.Is(err, ErrCapabilityUnsupported):
		return ErrorClassCapability
	case errors.Is(err, ErrQuotaExceeded):
		return ErrorClassQuotaExceeded
	case errors.Is(err, ErrDuplicateCall):
		return ErrorClassDuplicateCall
	case errors.Is(err, ErrMessagesRequired), errors.Is(err, ErrTeamRequired):
		return ErrorClassInvalidRequest
	case errors.Is(err, ErrCatalogNotConfigured),
		errors.Is(err, ErrFactoryNotConfigured),
		errors.Is(err, ErrLedgerNotConfigured):
		return ErrorClassNotConfigured
	case errors.Is(err, ErrUpstream):
		return ErrorClassUpstream
	default:
		return ErrorClassInternal
	}
}

// RetryableClass reports whether trying the same call again could plausibly
// succeed. It describes the failure; it never authorizes a replay after the
// caller has already seen output.
func RetryableClass(class string) bool {
	return class == ErrorClassUpstream
}
