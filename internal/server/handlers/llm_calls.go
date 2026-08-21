package handlers

import (
	"net/http"

	"github.com/gougoujiang/buildmax/internal/core/model"
	"github.com/gougoujiang/buildmax/internal/server/httputil"
)

// LLMCallSummary is one managed call as a reader sees it.
//
// It is the ledger row minus nothing, because the ledger already holds no
// prompts, tool arguments, or generated content — it was built as an accounting
// record. What it does hold and this deliberately drops is `target_id`, the
// catalog entry the alias resolved to: a team is granted aliases, and the
// operator's routing behind one is not the team's business.
type LLMCallSummary struct {
	LLMCallID string `json:"llm_call_id"`
	// UserID is the run's owner. A task run is somebody's work even though no
	// person was at the keyboard when it called a model.
	UserID    *string `json:"user_id,omitempty"`
	TaskID    *string `json:"task_id,omitempty"`
	Surface   string  `json:"surface,omitempty"`
	SessionID *string `json:"session_id,omitempty"`

	Alias     string `json:"alias,omitempty"`
	Streaming bool   `json:"streaming"`

	AcceptedAt   int64  `json:"accepted_at"`
	FirstDeltaAt *int64 `json:"first_delta_at,omitempty"`
	CompletedAt  *int64 `json:"completed_at,omitempty"`

	Status     string  `json:"status"`
	ErrorClass *string `json:"error_class,omitempty"`
	Attempts   int     `json:"attempts,omitempty"`

	PromptTokens     *int `json:"prompt_tokens,omitempty"`
	CompletionTokens *int `json:"completion_tokens,omitempty"`
	TotalTokens      *int `json:"total_tokens,omitempty"`
	// UsageSource separates a provider that reported nothing from one that
	// reported zero. Without it an absent count reads as a free call.
	UsageSource string `json:"usage_source,omitempty"`
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
	_, teamID, ok := h.guard().UserAndPathTeam(w, r, h.cfg.TeamStore, "teams not configured")
	if !ok {
		return
	}
	if !httputil.RequireStore(w, h.cfg.LLMCallStore, "managed model calls not configured") {
		return
	}
	taskRunID, ok := httputil.PathValue(w, r, "task_run_id")
	if !ok {
		return
	}
	// The run has to belong to this team's conversations before its ledger is
	// read, so a member of one team cannot enumerate another's spending by
	// guessing run ids.
	if _, _, ok = h.getArtifactRunAndTaskForTeam(w, r, teamID, taskRunID); !ok {
		return
	}

	calls, err := h.cfg.LLMCallStore.ListLLMCallsByTaskRun(r.Context(), teamID, taskRunID)
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

func toLLMCallSummary(call model.LLMCall) LLMCallSummary {
	return LLMCallSummary{
		LLMCallID:        call.LLMCallID,
		UserID:           call.UserID,
		TaskID:           call.TaskID,
		Surface:          call.Surface,
		SessionID:        call.SessionID,
		Alias:            call.Alias,
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
		UsageSource:      call.UsageSource,
	}
}
