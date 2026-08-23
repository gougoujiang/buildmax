package model

import (
	"context"
	"errors"
	"time"
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
	ID string `json:"id"`
	// ClientCallID is the caller's idempotency key, unique within one user's
	// calls. It is absent for calls the server makes on its own behalf.
	ClientCallID *string `json:"client_call_id,omitempty"`

	// Identity — derived from authentication, never from the request body.
	//
	// A call is attributed to a person. There is no team column: a foreground
	// call belongs to no team, and a run's team is reached through TaskRunID.
	// See docs/design/client-modes.md section 9.
	UserID    *string `json:"user_id,omitempty"`
	TaskRunID *string `json:"task_run_id,omitempty"`

	// Correlation — context for investigation, not authorization input.
	Surface   string  `json:"surface,omitempty"`
	SessionID *string `json:"session_id,omitempty"`
	TaskID    *string `json:"task_id,omitempty"`

	// Model — what the caller asked for and what it resolved to.
	Model         string `json:"model,omitempty"`
	TargetID      string `json:"target_id"`
	ProviderType  string `json:"provider_type"`
	UpstreamModel string `json:"upstream_model"`
	Streaming     bool   `json:"streaming"`

	// Timing, in unix seconds like every other table.
	AcceptedAt        time.Time  `json:"accepted_at"`
	UpstreamStartedAt *time.Time `json:"upstream_started_at,omitempty"`
	FirstDeltaAt      *time.Time `json:"first_delta_at,omitempty"`
	CompletedAt       *time.Time `json:"completed_at,omitempty"`

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
	CacheReadTokens  *int   `json:"cache_read_tokens,omitempty"`
	CacheWriteTokens *int   `json:"cache_write_tokens,omitempty"`
	UsageSource      string `json:"usage_source,omitempty"`

	// Pricing is the rate snapshot taken when the call was accepted, in
	// nano-currency-units per million tokens. It is a snapshot rather than a
	// reference: a model's price changes, and recomputing an old call from the
	// new rates would rewrite what a team already spent. An empty Currency
	// means the model was unpriced then, which is not the same fact as a call
	// that cost nothing.
	Currency              string `json:"currency,omitempty"`
	RateInputPerMTok      *int64 `json:"rate_input_per_mtok,omitempty"`
	RateCacheReadPerMTok  *int64 `json:"rate_cache_read_per_mtok,omitempty"`
	RateCacheWritePerMTok *int64 `json:"rate_cache_write_per_mtok,omitempty"`
	RateOutputPerMTok     *int64 `json:"rate_output_per_mtok,omitempty"`
}

// LLMCallUsage is the token usage reported for one call.
type LLMCallUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
	// CacheReadTokens and CacheWriteTokens are the cached parts of the prompt.
	// They break PromptTokens down rather than adding to it, so a spend report
	// must not sum them alongside it.
	CacheReadTokens  int    `json:"cache_read_tokens"`
	CacheWriteTokens int    `json:"cache_write_tokens"`
	Source           string `json:"source"`
}

// LLMCallOutcome is the terminal state written when a call finishes.
type LLMCallOutcome struct {
	Status            string
	ErrorClass        *string
	Attempts          int
	UpstreamStartedAt *time.Time
	FirstDeltaAt      *time.Time
	CompletedAt       time.Time
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
	// GetLLMCallByClientID returns one user's call by their idempotency key, or
	// (nil, nil) when not found. The key is scoped to the user who sent it, so
	// one caller's key can never resolve another's call.
	GetLLMCallByClientID(ctx context.Context, userID, clientCallID string) (*LLMCall, error)
	// ListLLMCallsByTaskRun returns a run's calls, oldest first, so a reader
	// follows the run in the order it happened.
	//
	// A run belongs to exactly one team, so authorizing the run authorizes its
	// ledger; the caller must have established that before asking.
	ListLLMCallsByTaskRun(ctx context.Context, taskRunID string) ([]LLMCall, error)
}
