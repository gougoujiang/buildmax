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
	ErrDuplicateCall       = errors.New("client call id has already been used")
	// ErrUpstream wraps a provider failure. The wrapped error stays inside the
	// server: callers receive the classification, not the provider's body.
	ErrUpstream = errors.New("upstream model call failed")
)

// DuplicateCallError reports a client call ID the team has already used.
//
// The first version does not attach to a running call or replay a completed
// one: it names the original call so the caller can stop guessing whether its
// request landed. It satisfies errors.Is(err, ErrDuplicateCall).
type DuplicateCallError struct {
	LLMCallID string
	Status    string
}

func (e *DuplicateCallError) Error() string {
	if e.LLMCallID == "" {
		return "this call id has already been used"
	}
	return fmt.Sprintf("call id already used by %s (%s)", e.LLMCallID, e.Status)
}

// Is reports whether the error matches the ErrDuplicateCall sentinel.
func (e *DuplicateCallError) Is(target error) bool { return target == ErrDuplicateCall }

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
	// UserID is who the call is for. A user-authenticated call takes it from the
	// login; a worker call takes it from the run token, which names the task's
	// owner — a run is somebody's work even though no person is at the keyboard.
	UserID *string
	// TaskRunID and TaskID are set on worker calls, from the run token.
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
	// ProviderState is reasoning state the upstream needs back on the next
	// request. The gateway carries it without reading it.
	ProviderState *cllm.ProviderState
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
	return s.run(ctx, req, nil)
}

// Stream runs one managed call, delivering content deltas to onDelta as they
// arrive and returning the assembled result.
//
// Retry policy belongs to the component that talks to the provider, which is
// the server-side client underneath this call. It stops retrying once a delta
// has been emitted, because the caller has already seen output. No layer above
// this one may add a retry of its own.
func (s *Service) Stream(ctx context.Context, req CompleteRequest, onDelta func(string)) (CompleteResult, error) {
	if onDelta == nil {
		onDelta = func(string) {}
	}
	return s.run(ctx, req, onDelta)
}

func (s *Service) run(ctx context.Context, req CompleteRequest, onDelta func(string)) (CompleteResult, error) {
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

	streaming := onDelta != nil

	// A repeated client call ID means the caller does not know whether we
	// accepted the first attempt. Say so instead of running the call twice.
	if err := s.rejectDuplicate(ctx, req); err != nil {
		return CompleteResult{}, err
	}

	routed, err := s.Router.ClientFor(ctx, ResolveRequest{
		TeamID:   req.TeamID,
		Alias:    req.Alias,
		Requires: requiredCapabilities(req, streaming),
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

	acceptedAt := s.now().UTC()
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
		Streaming:     streaming,
		AcceptedAt:    acceptedAt,
		Status:        model.LLMCallStatusAccepted,
	})
	if err != nil {
		if errors.Is(err, model.ErrDuplicateLLMCall) {
			return CompleteResult{}, &DuplicateCallError{}
		}
		return CompleteResult{}, fmt.Errorf("open call ledger: %w", err)
	}

	upstreamStartedAt := s.now().UTC()
	var firstDeltaAt *time.Time

	var completion cllm.Completion
	var callErr error
	upstreamCtx := cllm.WithCallOrigin(ctx, upstreamCallOrigin(req.Surface))
	if streaming {
		observed := func(delta string) {
			if firstDeltaAt == nil {
				at := s.now().UTC()
				firstDeltaAt = &at
			}
			onDelta(delta)
		}
		completion, callErr = routed.Client.ChatCompletionStreaming(upstreamCtx, req.Messages, req.Tools, observed)
	} else {
		completion, callErr = routed.Client.ChatCompletionBlocking(upstreamCtx, req.Messages, req.Tools)
	}

	outcome := model.LLMCallOutcome{
		Attempts:          1,
		UpstreamStartedAt: &upstreamStartedAt,
		FirstDeltaAt:      firstDeltaAt,
		CompletedAt:       s.now().UTC(),
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
		s.closeLedger(ctx, call.ID, outcome)
		return CompleteResult{LLMCallID: call.ID}, fmt.Errorf("%w: %w", ErrUpstream, callErr)
	}

	usage := completion.Usage
	reported := usage.TotalTokens > 0 || usage.PromptTokens > 0 || usage.CompletionTokens > 0
	outcome.Status = model.LLMCallStatusSucceeded
	if reported {
		outcome.Usage = &model.LLMCallUsage{
			PromptTokens:     usage.PromptTokens,
			CompletionTokens: usage.CompletionTokens,
			TotalTokens:      usage.TotalTokens,
			CacheReadTokens:  usage.CacheReadTokens,
			CacheWriteTokens: usage.CacheWriteTokens,
			Source:           model.LLMUsageSourceReported,
		}
	}
	s.closeLedger(ctx, call.ID, outcome)

	return CompleteResult{
		LLMCallID:     call.ID,
		Alias:         routed.Resolution.Alias,
		Content:       completion.Content,
		ToolCalls:     completion.ToolCalls,
		Usage:         usage,
		UsageReported: reported,
		ProviderState: completion.ProviderState,
	}, nil
}

// upstreamCallOrigin preserves only runtime-owned surfaces. CompleteRequest is
// assembled by handlers, but its metadata may have crossed the network and
// must not become an arbitrary HTTP header value at an upstream provider.
func upstreamCallOrigin(surface string) cllm.CallOrigin {
	switch surface {
	case model.LLMCallSurfaceCLI, model.LLMCallSurfaceDesktop, model.LLMCallSurfaceWorker:
		return cllm.CallOrigin{Surface: surface, ViaGateway: true}
	case model.LLMCallSurfaceServer:
		return cllm.CallOrigin{Surface: surface}
	default:
		return cllm.CallOrigin{Surface: model.LLMCallSurfaceServer}
	}
}

// closeLedger writes the terminal record. A write failure does not discard work
// the provider already charged for: the row stays ACCEPTED, which is the signal
// that reconciliation is needed.
func (s *Service) closeLedger(ctx context.Context, llmCallID string, outcome model.LLMCallOutcome) {
	// The call may have ended because the caller went away; use a context that
	// is still live so the terminal record is written anyway.
	writeCtx := context.WithoutCancel(ctx)
	if err := s.Ledger.CompleteLLMCall(writeCtx, llmCallID, outcome); err != nil {
		gatewayLog().Error("call ledger not closed; row stays ACCEPTED",
			"err", err, "llm_call_id", llmCallID, "status", outcome.Status)
	}
}

// requiredCapabilities derives what this request needs from its shape, so a
// target that cannot serve it fails before an upstream call.
func requiredCapabilities(req CompleteRequest, streaming bool) []Capability {
	capabilities := []Capability{CapabilityTextChat}
	if len(req.Tools) > 0 {
		capabilities = append(capabilities, CapabilityToolCalls)
	}
	if streaming {
		capabilities = append(capabilities, CapabilityStreamingText)
	}
	return capabilities
}

// rejectDuplicate reports a client call ID this team has already used.
//
// The unique index is what actually decides duplication; this lookup exists to
// answer with the original call ID instead of a bare constraint violation. A
// lookup failure is not fatal: the index still closes the race.
func (s *Service) rejectDuplicate(ctx context.Context, req CompleteRequest) error {
	if req.ClientCallID == nil || *req.ClientCallID == "" {
		return nil
	}
	existing, err := s.Ledger.GetLLMCallByClientID(ctx, req.TeamID, *req.ClientCallID)
	if err != nil || existing == nil {
		return nil
	}
	return &DuplicateCallError{LLMCallID: existing.ID, Status: existing.Status}
}

// Identity belongs in an attr, not in every message string.
func gatewayLog() *slog.Logger { return slog.With("component", "llm_gateway") }
