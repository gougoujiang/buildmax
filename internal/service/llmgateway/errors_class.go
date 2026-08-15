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
	ErrorClassTeamRequired      = "team_required"
	ErrorClassTeamNotAuthorized = "team_not_authorized"
	ErrorClassUnknownAlias      = "unknown_alias"
	ErrorClassTargetNotFound    = "target_not_found"
	ErrorClassTargetDisabled    = "target_disabled"
	ErrorClassCapability        = "capability_unsupported"
	ErrorClassQuotaExceeded     = "quota_exceeded"
	ErrorClassInvalidRequest    = "invalid_request"
	ErrorClassNotConfigured     = "not_configured"
	ErrorClassCanceled          = "canceled"
	ErrorClassUpstream          = "upstream_error"
	ErrorClassInternal          = "internal_error"
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
	case errors.Is(err, ErrTeamRequired):
		return ErrorClassTeamRequired
	case errors.Is(err, ErrTeamNotAuthorized):
		return ErrorClassTeamNotAuthorized
	case errors.Is(err, ErrUnknownAlias), errors.Is(err, ErrNoDefaultAlias):
		return ErrorClassUnknownAlias
	case errors.Is(err, ErrTargetNotFound):
		return ErrorClassTargetNotFound
	case errors.Is(err, ErrTargetDisabled):
		return ErrorClassTargetDisabled
	case errors.Is(err, ErrCapabilityUnsupported):
		return ErrorClassCapability
	case errors.Is(err, ErrQuotaExceeded):
		return ErrorClassQuotaExceeded
	case errors.Is(err, ErrMessagesRequired):
		return ErrorClassInvalidRequest
	case errors.Is(err, ErrCatalogNotConfigured),
		errors.Is(err, ErrPolicyNotConfigured),
		errors.Is(err, ErrFactoryNotConfigured),
		errors.Is(err, ErrLedgerNotConfigured):
		return ErrorClassNotConfigured
	case errors.Is(err, ErrUpstream):
		return ErrorClassUpstream
	default:
		return ErrorClassInternal
	}
}
