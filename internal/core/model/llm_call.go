package model

import (
	"context"
	"errors"
)

// ErrDuplicateLLMCall is returned when a team reuses a client call ID. The
// unique index is what actually decides it, so two concurrent requests with one
// key cannot both open a record.
var ErrDuplicateLLMCall = errors.New("llm call already exists for this client call id")

// Managed LLM call lifecycle statuses.
const (
	LLMCallStatusAccepted  = "ACCEPTED"
	LLMCallStatusSucceeded = "SUCCEEDED"
	LLMCallStatusFailed    = "FAILED"
	LLMCallStatusCanceled  = "CANCELED"
)

// Where a call's token counts came from. Recording this keeps accounting honest
// when a provider reports no usage: an absent number and a zero are different
// facts, and only one of them may be billed.
const (
	LLMUsageSourceReported    = "reported"
	LLMUsageSourceEstimated   = "estimated"
	LLMUsageSourceUnavailable = "unavailable"
)

// Surfaces a managed call can originate from.
const (
	LLMCallSurfaceServer  = "server"
	LLMCallSurfaceCLI     = "cli"
	LLMCallSurfaceDesktop = "desktop"
	LLMCallSurfaceWorker  = "worker"
)

// LLMCall is one logical managed inference call.
//
// It is an accounting and diagnostic record, not a transcript: prompts, tool
// arguments, tool results, and generated content are deliberately absent. Run
// detail belongs to durable local traces. See docs/design/llm-gateway.md.
type LLMCall struct {
	ID        uint   `json:"-"`
	LLMCallID string `json:"llm_call_id"`
	// ClientCallID is the caller's idempotency key, unique within a team. It is
	// absent for calls the server makes on its own behalf.
	ClientCallID *string `json:"client_call_id,omitempty"`

	// Identity — derived from authentication, never from the request body.
	TeamID    string  `json:"team_id"`
	UserID    *string `json:"user_id,omitempty"`
	TaskRunID *string `json:"task_run_id,omitempty"`

	// Correlation — context for investigation, not authorization input.
	Surface   string  `json:"surface,omitempty"`
	SessionID *string `json:"session_id,omitempty"`
	TaskID    *string `json:"task_id,omitempty"`

	// Model — what the caller asked for and what it resolved to.
	Alias         string `json:"alias,omitempty"`
	TargetID      string `json:"target_id"`
	ProviderType  string `json:"provider_type"`
	UpstreamModel string `json:"upstream_model"`
	Streaming     bool   `json:"streaming"`

	// Timing, in unix seconds like every other table.
	AcceptedAt        int64  `json:"accepted_at"`
	UpstreamStartedAt *int64 `json:"upstream_started_at,omitempty"`
	FirstDeltaAt      *int64 `json:"first_delta_at,omitempty"`
	CompletedAt       *int64 `json:"completed_at,omitempty"`

	// Outcome.
	Status string `json:"status"`
	// ErrorClass is the stable BuildMax error classification, never an upstream
	// error body.
	ErrorClass *string `json:"error_class,omitempty"`
	// Attempts counts upstream attempts, so a retry does not read as two calls.
	Attempts int `json:"attempts,omitempty"`

	// Usage.
	PromptTokens     *int   `json:"prompt_tokens,omitempty"`
	CompletionTokens *int   `json:"completion_tokens,omitempty"`
	TotalTokens      *int   `json:"total_tokens,omitempty"`
	UsageSource      string `json:"usage_source,omitempty"`
}

// LLMCallUsage is the token usage reported for one call.
type LLMCallUsage struct {
	PromptTokens     int    `json:"prompt_tokens"`
	CompletionTokens int    `json:"completion_tokens"`
	TotalTokens      int    `json:"total_tokens"`
	Source           string `json:"source"`
}

// LLMCallOutcome is the terminal state written when a call finishes.
type LLMCallOutcome struct {
	Status            string
	ErrorClass        *string
	Attempts          int
	UpstreamStartedAt *int64
	FirstDeltaAt      *int64
	CompletedAt       int64
	// Usage is nil when the provider reported none; the record then keeps
	// LLMUsageSourceUnavailable rather than zero counts.
	Usage *LLMCallUsage
}

// LLMCallStore persists the managed call ledger.
type LLMCallStore interface {
	// OpenLLMCall records an accepted call before the upstream request starts.
	// It assigns the call ID and returns the stored record.
	OpenLLMCall(ctx context.Context, call *LLMCall) (*LLMCall, error)
	// CompleteLLMCall writes the terminal outcome of an open call.
	CompleteLLMCall(ctx context.Context, llmCallID string, outcome LLMCallOutcome) error
	// GetLLMCall returns one call by ID, or (nil, nil) when not found.
	GetLLMCall(ctx context.Context, llmCallID string) (*LLMCall, error)
	// GetLLMCallByClientID returns a team's call by the caller's idempotency
	// key, or (nil, nil) when not found.
	GetLLMCallByClientID(ctx context.Context, teamID, clientCallID string) (*LLMCall, error)
}
