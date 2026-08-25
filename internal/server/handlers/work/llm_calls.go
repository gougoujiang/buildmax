package work

import (
	"net/http"
	"time"

	cllm "github.com/gougoujiang/buildmax/internal/core/llm"
	coregw "github.com/gougoujiang/buildmax/internal/core/llmgateway"
	"github.com/gougoujiang/buildmax/internal/server/httputil"
)

// LLMCallSummary is one managed call as a reader sees it.
//
// It is the ledger row minus nothing, because the ledger already holds no
// prompts, tool arguments, or generated content — it was built as an accounting
// record. What it does hold and this deliberately drops is `target_id`, the
// catalog entry the name resolved to: the operator's routing behind a model
// name is not the caller's business.
type LLMCallSummary struct {
	ID string `json:"id"`
	// UserID is the run's owner. A task run is somebody's work even though no
	// person was at the keyboard when it called a model.
	UserID    *string `json:"user_id,omitempty"`
	TaskID    *string `json:"task_id,omitempty"`
	Surface   string  `json:"surface,omitempty"`
	SessionID *string `json:"session_id,omitempty"`

	Model     string `json:"model,omitempty"`
	Streaming bool   `json:"streaming"`

	AcceptedAt   time.Time  `json:"accepted_at"`
	FirstDeltaAt *time.Time `json:"first_delta_at,omitempty"`
	CompletedAt  *time.Time `json:"completed_at,omitempty"`

	Status     string  `json:"status"`
	ErrorClass *string `json:"error_class,omitempty"`
	Attempts   int     `json:"attempts,omitempty"`

	PromptTokens     *int `json:"prompt_tokens,omitempty"`
	CompletionTokens *int `json:"completion_tokens,omitempty"`
	TotalTokens      *int `json:"total_tokens,omitempty"`
	// Cache counts are the cached parts of PromptTokens, not tokens on top of
	// it. The ledger already recorded them; leaving them out of this view was
	// what made a cache-heavy run indistinguishable from an uncached one.
	CacheReadTokens  *int `json:"cache_read_tokens,omitempty"`
	CacheWriteTokens *int `json:"cache_write_tokens,omitempty"`
	// UsageSource separates a provider that reported nothing from one that
	// reported zero. Without it an absent count reads as a free call.
	UsageSource string `json:"usage_source,omitempty"`

	// Cost is what this call is estimated to have cost, priced at the rates
	// recorded when it ran rather than at whatever the catalog says now. Absent
	// when the model was unpriced or the provider reported no usage: an
	// unpriced call is an unknown, and a zero would read as a free one.
	//
	// Amounts are nano-units of Currency — one currency unit is 1e9 of them —
	// so a client sums them exactly instead of accumulating float error across
	// a run.
	Cost *LLMCallCost `json:"cost,omitempty"`
}

// LLMCallCost is one call's estimated spend, broken down the way caching
// charges it.
//
// The breakdown is kept rather than summed away because the parts answer
// different questions. Total is what the call cost; Baseline is what the same
// tokens would have cost with no caching at all, which is the only honest way
// to say whether caching helped — comparing against zero would report a saving
// on a call that only ever wrote.
type LLMCallCost struct {
	Currency   string `json:"currency"`
	Uncached   int64  `json:"uncached"`
	CacheRead  int64  `json:"cache_read"`
	CacheWrite int64  `json:"cache_write"`
	Output     int64  `json:"output"`
	Total      int64  `json:"total"`
	Baseline   int64  `json:"baseline"`
}

// listTaskRunLLMCallsHandler serves
// GET /api/teams/{team_id}/task-runs/{task_run_id}/llm-calls.
//
// It answers what a run spent and on which approved model, which until now was
// recorded and unreachable: the ledger had no route, so the only way to read it
// was a database query. Diagnosing a run should not require the database
// password.
func (h *Handler) listTaskRunLLMCallsHandler(w http.ResponseWriter, r *http.Request) {
	// Team membership is checked before the ledger, so an unauthenticated caller
	// learns nothing about whether this deployment records managed calls. Every
	// other team-scoped route authenticates first, and an authorization matrix is
	// only meaningful if they all agree.
	_, teamID, ok := h.guard().UserAndPathTeam(w, r, h.cfg.Teams, "teams not configured")
	if !ok {
		return
	}
	if !httputil.RequireStore(w, h.cfg.LLMCalls, "managed model calls not configured") {
		return
	}
	taskRunID, ok := httputil.PathValue(w, r, "task_run_id")
	if !ok {
		return
	}
	// The run has to belong to this team's conversations before its ledger is
	// read, so a member of one team cannot enumerate another's spending by
	// guessing run ids. This check is the whole authorization: ledger rows carry
	// no team of their own, and a run belongs to exactly one.
	if _, _, ok = h.getArtifactRunAndTaskForTeam(w, r, teamID, taskRunID); !ok {
		return
	}

	calls, err := h.cfg.LLMCalls.ListLLMCallsByTaskRun(r.Context(), taskRunID)
	if err != nil {
		httputil.WriteInternalError(w, err, "handler error", "handler", "task_run_llm_calls", "task_run_id", taskRunID)
		return
	}
	out := make([]LLMCallSummary, 0, len(calls))
	for _, call := range calls {
		out = append(out, toLLMCallSummary(call))
	}
	httputil.WriteJSON(w, http.StatusOK, out)
}

func toLLMCallSummary(call coregw.Call) LLMCallSummary {
	out := LLMCallSummary{
		ID:               call.ID,
		UserID:           call.UserID,
		TaskID:           call.TaskID,
		Surface:          call.Surface,
		SessionID:        call.SessionID,
		Model:            call.Model,
		Streaming:        call.Streaming,
		AcceptedAt:       call.AcceptedAt,
		FirstDeltaAt:     call.FirstDeltaAt,
		CompletedAt:      call.CompletedAt,
		Status:           call.Status,
		ErrorClass:       call.ErrorClass,
		Attempts:         call.Attempts,
		PromptTokens:     call.PromptTokens,
		CompletionTokens: call.CompletionTokens,
		TotalTokens:      call.TotalTokens,
		CacheReadTokens:  call.CacheReadTokens,
		CacheWriteTokens: call.CacheWriteTokens,
		UsageSource:      call.UsageSource,
	}
	if cost, ok := llmCallCost(call); ok {
		out.Cost = &cost
	}
	return out
}

// llmCallCost prices a ledger row from its own rate snapshot.
//
// The rates come from the row rather than the catalog on purpose: a model's
// price changes, and recomputing an old call from the new rates would restate
// what a team already spent. A row written before the snapshot existed has no
// rates and reports no cost, which is the truthful answer — nobody recorded
// what it was charged.
func llmCallCost(call coregw.Call) (LLMCallCost, bool) {
	if call.Currency == "" || call.RateInputPerMTok == nil {
		return LLMCallCost{}, false
	}
	usage := cllm.Usage{
		PromptTokens:     derefInt(call.PromptTokens),
		CompletionTokens: derefInt(call.CompletionTokens),
		TotalTokens:      derefInt(call.TotalTokens),
		CacheReadTokens:  derefInt(call.CacheReadTokens),
		CacheWriteTokens: derefInt(call.CacheWriteTokens),
	}
	cost, ok := cllm.EstimateCost(usage, cllm.Pricing{
		Currency:          call.Currency,
		InputPerMTok:      derefInt64(call.RateInputPerMTok),
		CacheReadPerMTok:  derefInt64(call.RateCacheReadPerMTok),
		CacheWritePerMTok: derefInt64(call.RateCacheWritePerMTok),
		OutputPerMTok:     derefInt64(call.RateOutputPerMTok),
	})
	if !ok {
		return LLMCallCost{}, false
	}
	return LLMCallCost{
		Currency:   cost.Currency,
		Uncached:   cost.Uncached,
		CacheRead:  cost.CacheRead,
		CacheWrite: cost.CacheWrite,
		Output:     cost.Output,
		Total:      cost.Total,
		Baseline:   cost.Baseline,
	}, true
}

func derefInt(v *int) int {
	if v == nil {
		return 0
	}
	return *v
}

func derefInt64(v *int64) int64 {
	if v == nil {
		return 0
	}
	return *v
}
