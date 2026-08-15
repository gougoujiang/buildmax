package llmgateway

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	cllm "github.com/gougoujiang/buildmax/internal/core/llm"
	"github.com/gougoujiang/buildmax/internal/core/model"
)

// Service-level failures, in addition to the resolution errors in resolve.go.
var (
	ErrLedgerNotConfigured = errors.New("call ledger is not configured")
	ErrMessagesRequired    = errors.New("messages are required")
	ErrQuotaExceeded       = errors.New("team quota exceeded")
	// ErrUpstream wraps a provider failure. The wrapped error stays inside the
	// server: callers receive the classification, not the provider's body.
	ErrUpstream = errors.New("upstream model call failed")
)

// QuotaError reports why a team was refused. It satisfies
// errors.Is(err, ErrQuotaExceeded).
type QuotaError struct {
	Reason string
}

func (e *QuotaError) Error() string { return e.Reason }

// Is reports whether the error matches the ErrQuotaExceeded sentinel.
func (e *QuotaError) Is(target error) bool { return target == ErrQuotaExceeded }

// QuotaChecker is the narrow quota surface the gateway needs.
type QuotaChecker interface {
	Check(ctx context.Context, teamID string, addRuns, addTokens int) (allowed bool, reason string)
}

// Service runs one managed call: resolve, authorize, meter, dispatch, record.
type Service struct {
	// Router resolves models and supplies clients.
	Router *Router
	// Ledger records every managed call.
	Ledger model.LLMCallStore
	// Quota is optional. Without it, calls are recorded but never refused.
	Quota QuotaChecker
	// Now is the clock, injectable so tests can pin ledger timestamps.
	Now func() time.Time
}

func (s *Service) now() time.Time {
	if s == nil || s.Now == nil {
		return time.Now()
	}
	return s.Now()
}

// CompleteRequest is one managed inference request.
//
// Identity fields are derived from authentication by the caller of this
// service; nothing here may be taken from a client request body.
type CompleteRequest struct {
	TeamID string
	// UserID is set for user-authenticated calls, TaskRunID for worker calls.
	UserID    *string
	TaskRunID *string
	TaskID    *string

	// ClientCallID is the caller's idempotency key. Optional.
	ClientCallID *string
	// Surface and SessionID are correlation context only.
	Surface   string
	SessionID *string

	// Alias is the team-facing model alias. Empty selects the team default.
	Alias    string
	Messages []cllm.Message
	Tools    []cllm.ToolDef
}

// CompleteResult is a finished managed call.
type CompleteResult struct {
	LLMCallID string
	Alias     string
	Content   string
	ToolCalls []cllm.ToolCall
	Usage     cllm.Usage
	// UsageReported is false when the provider returned no token counts. The
	// zero Usage then means "unknown", not "free".
	UsageReported bool
}

// Models lists the aliases a team may use.
func (s *Service) Models(ctx context.Context, teamID string) ([]AvailableModel, error) {
	if s == nil || s.Router == nil {
		return nil, ErrCatalogNotConfigured
	}
	return s.Router.Available(ctx, teamID)
}

// Complete runs one blocking managed call.
//
// The ledger record opens before the upstream request and closes after it, so a
// call that never returns still leaves an ACCEPTED row behind as evidence that
// tokens may have been spent.
func (s *Service) Complete(ctx context.Context, req CompleteRequest) (CompleteResult, error) {
	if s == nil || s.Router == nil {
		return CompleteResult{}, ErrCatalogNotConfigured
	}
	if s.Ledger == nil {
		return CompleteResult{}, ErrLedgerNotConfigured
	}
	if req.TeamID == "" {
		return CompleteResult{}, ErrTeamRequired
	}
	if len(req.Messages) == 0 {
		return CompleteResult{}, ErrMessagesRequired
	}

	routed, err := s.Router.ClientFor(ctx, ResolveRequest{
		TeamID:   req.TeamID,
		Alias:    req.Alias,
		Requires: requiredCapabilities(req),
	})
	if err != nil {
		return CompleteResult{}, err
	}

	// Soft enforcement: a team already over its limit is refused. Concurrent
	// calls can still overshoot, because the size of a completion is unknown
	// before it exists. See docs/design/llm-gateway.md section 10.
	if s.Quota != nil {
		allowed, reason := s.Quota.Check(ctx, req.TeamID, 0, 0)
		if !allowed {
			return CompleteResult{}, &QuotaError{Reason: reason}
		}
	}

	acceptedAt := s.now().Unix()
	call, err := s.Ledger.OpenLLMCall(ctx, &model.LLMCall{
		ClientCallID:  req.ClientCallID,
		TeamID:        req.TeamID,
		UserID:        req.UserID,
		TaskRunID:     req.TaskRunID,
		TaskID:        req.TaskID,
		Surface:       req.Surface,
		SessionID:     req.SessionID,
		Alias:         routed.Resolution.Alias,
		TargetID:      routed.Resolution.Target.ID,
		ProviderType:  routed.Resolution.Target.ProviderType,
		UpstreamModel: routed.Resolution.Target.UpstreamModel,
		Streaming:     false,
		AcceptedAt:    acceptedAt,
		Status:        model.LLMCallStatusAccepted,
	})
	if err != nil {
		return CompleteResult{}, fmt.Errorf("open call ledger: %w", err)
	}

	upstreamStartedAt := s.now().Unix()
	content, toolCalls, usage, callErr := routed.Client.ChatCompletionBlocking(ctx, req.Messages, req.Tools)

	outcome := model.LLMCallOutcome{
		Attempts:          1,
		UpstreamStartedAt: &upstreamStartedAt,
		CompletedAt:       s.now().Unix(),
	}
	if callErr != nil {
		class := ErrorClassFor(callErr)
		if class == ErrorClassInternal {
			class = ErrorClassUpstream
		}
		outcome.Status = model.LLMCallStatusFailed
		if errors.Is(callErr, context.Canceled) || errors.Is(callErr, context.DeadlineExceeded) {
			outcome.Status = model.LLMCallStatusCanceled
		}
		outcome.ErrorClass = &class
		s.closeLedger(ctx, call.LLMCallID, outcome)
		return CompleteResult{LLMCallID: call.LLMCallID}, fmt.Errorf("%w: %w", ErrUpstream, callErr)
	}

	reported := usage.TotalTokens > 0 || usage.PromptTokens > 0 || usage.CompletionTokens > 0
	outcome.Status = model.LLMCallStatusSucceeded
	if reported {
		outcome.Usage = &model.LLMCallUsage{
			PromptTokens:     usage.PromptTokens,
			CompletionTokens: usage.CompletionTokens,
			TotalTokens:      usage.TotalTokens,
			Source:           model.LLMUsageSourceReported,
		}
	}
	s.closeLedger(ctx, call.LLMCallID, outcome)

	return CompleteResult{
		LLMCallID:     call.LLMCallID,
		Alias:         routed.Resolution.Alias,
		Content:       content,
		ToolCalls:     toolCalls,
		Usage:         usage,
		UsageReported: reported,
	}, nil
}

// closeLedger writes the terminal record. A write failure does not discard work
// the provider already charged for: the row stays ACCEPTED, which is the signal
// that reconciliation is needed.
func (s *Service) closeLedger(ctx context.Context, llmCallID string, outcome model.LLMCallOutcome) {
	// The call may have ended because the caller went away; use a context that
	// is still live so the terminal record is written anyway.
	writeCtx := context.WithoutCancel(ctx)
	if err := s.Ledger.CompleteLLMCall(writeCtx, llmCallID, outcome); err != nil {
		slog.Error("llm gateway: call ledger not closed; row stays ACCEPTED",
			"err", err, "llm_call_id", llmCallID, "status", outcome.Status)
	}
}

// requiredCapabilities derives what this request needs from its shape, so a
// target that cannot serve it fails before an upstream call.
func requiredCapabilities(req CompleteRequest) []Capability {
	capabilities := []Capability{CapabilityTextChat}
	if len(req.Tools) > 0 {
		capabilities = append(capabilities, CapabilityToolCalls)
	}
	return capabilities
}
