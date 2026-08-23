package work

import (
	"net/http"

	"github.com/gougoujiang/buildmax/internal/core/model"
	"github.com/gougoujiang/buildmax/internal/server/httputil"
	"github.com/gougoujiang/buildmax/internal/util"
)

// sourceMessageMaxLen bounds the quoted message. It is a comparison, not a
// transcript: the conversation route serves the full text.
const sourceMessageMaxLen = 2000

// RunProvenanceResponse answers where one run came from.
//
// It is deliberately not the run record. What a reader needs here is the chain
// of responsibility — who or what asked, through which path, repeating which
// earlier attempt, and in which message — next to the instruction that reached
// the worker. Output, tokens, and worker placement answer different questions
// and have their own routes.
type RunProvenanceResponse struct {
	TaskRunID string `json:"task_run_id"`
	TaskID    string `json:"task_id"`
	Status    string `json:"status"`
	// Input is what the worker was given. Compare it with SourceMessage.
	Input            string                 `json:"input"`
	CreatedBy        string                 `json:"created_by,omitempty"`
	CreatedByType    string                 `json:"created_by_type,omitempty"`
	TriggerSource    string                 `json:"trigger_source,omitempty"`
	RetryOfTaskRunID *string                `json:"retry_of_task_run_id,omitempty"`
	CreatedAt        int64                  `json:"created_at"`
	SourceMessage    *SourceMessageResponse `json:"source_message,omitempty"`
}

// SourceMessageResponse is what the person actually said.
type SourceMessageResponse struct {
	ID        string `json:"id"`
	Content   string `json:"content"`
	Truncated bool   `json:"truncated"`
	CreatedAt int64  `json:"created_at"`
}

// getTaskRunProvenanceHandler serves GET
// /api/teams/{team_id}/task-runs/{task_run_id}.
func (h *Handler) getTaskRunProvenanceHandler(w http.ResponseWriter, r *http.Request) {
	_, teamID, ok := h.guard().UserAndPathTeam(w, r, h.cfg.TaskRuns, "task runs not configured")
	if !ok {
		return
	}
	taskRunID, ok := httputil.PathValue(w, r, "task_run_id")
	if !ok {
		return
	}
	run, task, ok := h.getArtifactRunAndTaskForTeam(w, r, teamID, taskRunID)
	if !ok {
		return
	}
	out := RunProvenanceResponse{
		TaskRunID:        run.ID,
		TaskID:           task.ID,
		Status:           run.Status,
		Input:            run.Input,
		CreatedBy:        run.CreatedBy,
		CreatedByType:    run.CreatedByType,
		TriggerSource:    run.TriggerSource,
		RetryOfTaskRunID: run.RetryOfTaskRunID,
		CreatedAt:        run.CreatedAt,
		SourceMessage:    h.resolveSourceMessage(r, task, run.SourceMessageID),
	}
	httputil.WriteJSON(w, http.StatusOK, out)
}

// resolveSourceMessage reads the message a run was asked for in.
//
// A message that cannot be read leaves the field absent rather than failing the
// request: the rest of the provenance is still true, and a run with no message
// behind it is the normal case anyway. The message is confirmed to belong to
// the run's own conversation before it is returned, so a stale handle cannot
// quote text from somewhere else.
func (h *Handler) resolveSourceMessage(r *http.Request, task *model.Task, messageID *string) *SourceMessageResponse {
	if messageID == nil || *messageID == "" || h.cfg.Messages == nil {
		return nil
	}
	msg, err := h.cfg.Messages.GetMessage(r.Context(), *messageID)
	if err != nil || msg == nil || msg.ConversationID != task.ConversationID {
		return nil
	}
	content := util.TruncateRunes(msg.Content, sourceMessageMaxLen)
	return &SourceMessageResponse{
		ID:        msg.ID,
		Content:   content,
		Truncated: content != msg.Content,
		CreatedAt: msg.CreatedAt,
	}
}
