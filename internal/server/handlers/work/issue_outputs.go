package work

import (
	"context"
	"errors"
	"os"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/gougoujiang/buildmax/internal/core/model"
	blob "github.com/gougoujiang/buildmax/internal/infra/objectstore"
	"github.com/gougoujiang/buildmax/internal/util"
)

const (
	issueOutputResultFilename = "result.md"
	issueOutputPreviewBudget  = 4096
)

type outputSourceResponse struct {
	SourceType        string  `json:"source_type"`
	TaskID            string  `json:"task_id,omitempty"`
	TaskRunID         string  `json:"task_run_id,omitempty"`
	ConversationID    string  `json:"conversation_id,omitempty"`
	WorkflowRunID     *string `json:"workflow_run_id,omitempty"`
	WorkflowStepRunID *string `json:"workflow_step_run_id,omitempty"`
	WorkflowStepID    *string `json:"workflow_step_id,omitempty"`
}

type issueOutputResponse struct {
	ID               string               `json:"id"`
	Title            string               `json:"title"`
	Kind             string               `json:"kind"`
	RelativePath     string               `json:"relative_path,omitempty"`
	Preview          string               `json:"preview,omitempty"`
	PreviewTruncated bool                 `json:"preview_truncated"`
	Source           outputSourceResponse `json:"source"`
	CreatedAt        int64                `json:"created_at"`
}

func cleanOutputID(taskRunID, relPath string) string {
	cleaned := strings.ReplaceAll(relPath, "/", "_")
	cleaned = strings.ReplaceAll(cleaned, ".", "_")
	return "out_" + taskRunID + "_" + cleaned
}

func truncatePreview(data []byte, budget int) (string, bool) {
	if len(data) <= budget {
		return string(data), false
	}
	trimmed := data[:budget]
	// Back off to a valid UTF-8 boundary.
	for len(trimmed) > 0 && !utf8.Valid(trimmed) {
		trimmed = trimmed[:len(trimmed)-1]
	}
	return string(trimmed), true
}

// aggregateIssueOutputs collects produced outputs for an issue from its
// agent tasks. Workflow step provenance is attached via stepsByTaskID
// (built from all workflow step runs across the issue's workflow runs).
// Returns outputs sorted by created_at DESC, plus the latest_result pointer.
// Missing/unreadable artifacts are silently skipped — they must not fail
// the overall issue flow response.
func (h *Handler) aggregateIssueOutputs(
	ctx context.Context,
	agentTasks []model.Task,
	stepsByTaskID map[string]model.WorkflowStepRun,
) ([]issueOutputResponse, *issueOutputResponse) {
	if h.cfg.RunOutputs == nil {
		return []issueOutputResponse{}, nil
	}
	outputs := make([]issueOutputResponse, 0)
	for _, t := range agentTasks {
		if t.LastRunID == nil || *t.LastRunID == "" {
			continue
		}
		taskRunID := *t.LastRunID
		items, err := h.cfg.RunOutputs.GetTaskRunOutputFiles(ctx, taskRunID)
		if err != nil {
			// Tolerate listing failures: try fallback below if any.
			items = nil
		}
		source := outputSourceResponse{
			SourceType:     "task_run",
			TaskID:         t.TaskID,
			TaskRunID:      taskRunID,
			ConversationID: t.ConversationID,
		}
		if step, ok := stepsByTaskID[t.TaskID]; ok {
			source.WorkflowRunID = util.Ptr(step.WorkflowRunID)
			source.WorkflowStepRunID = util.Ptr(step.StepRunID)
			source.WorkflowStepID = util.Ptr(step.StepID)
		}

		// Prefer result.md when present.
		resultItem := findResultItem(items)
		switch {
		case resultItem != nil:
			preview, truncated := h.readArtifactPreview(ctx, t, taskRunID, issueOutputResultFilename)
			outputs = append(outputs, issueOutputResponse{
				ID:               cleanOutputID(taskRunID, issueOutputResultFilename),
				Title:            "Latest result",
				Kind:             "markdown",
				RelativePath:     issueOutputResultFilename,
				Preview:          preview,
				PreviewTruncated: truncated,
				Source:           source,
				CreatedAt:        t.CreatedAt,
			})
		case t.Output != nil && strings.TrimSpace(*t.Output) != "":
			// Fallback: task produced text output but no artifact file.
			text := *t.Output
			preview, truncated := truncatePreview([]byte(text), issueOutputPreviewBudget)
			outputs = append(outputs, issueOutputResponse{
				ID:               "out_" + taskRunID + "_task_output",
				Title:            "Task output",
				Kind:             "text",
				Preview:          preview,
				PreviewTruncated: truncated,
				Source:           source,
				CreatedAt:        t.CreatedAt,
			})
		}
	}
	sort.SliceStable(outputs, func(i, j int) bool {
		return outputs[i].CreatedAt > outputs[j].CreatedAt
	})
	var latest *issueOutputResponse
	if len(outputs) > 0 {
		l := outputs[0]
		latest = &l
	}
	return outputs, latest
}

func findResultItem(items []model.TaskRunArtifact) *model.TaskRunArtifact {
	for i := range items {
		if items[i].RelativePath == issueOutputResultFilename {
			return &items[i]
		}
	}
	return nil
}

// readArtifactPreview reads a bounded preview of result.md for a task run.
// Missing or unreadable content returns ("", false) — the caller still emits
// a card without preview.
func (h *Handler) readArtifactPreview(ctx context.Context, t model.Task, taskRunID, relPath string) (string, bool) {
	if h.cfg.ArtifactStorage == nil {
		return "", false
	}
	if relPath != issueOutputResultFilename {
		// MVP only previews result.md via GetResult; other paths would
		// need GetArtifactFile and a kind whitelist.
		return "", false
	}
	data, err := h.cfg.ArtifactStorage.GetResult(ctx, blob.RunRef{
		CreatedBy:      t.CreatedBy,
		ConversationID: t.ConversationID,
		TaskID:         t.TaskID,
		TaskRunID:      taskRunID,
	})
	if err != nil {
		if errors.Is(err, os.ErrNotExist) || errors.Is(err, blob.ErrNotFound) {
			return "", false
		}
		// Other errors: log via http handler caller is not available here;
		// silently degrade — the spec requires not to fail the whole flow.
		return "", false
	}
	return truncatePreview(data, issueOutputPreviewBudget)
}
