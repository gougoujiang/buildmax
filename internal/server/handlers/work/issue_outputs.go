package work

import (
	"context"
	"errors"
	"github.com/gougoujiang/buildmax/internal/core/apierr"
	"os"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	coretask "github.com/gougoujiang/buildmax/internal/core/task"
	coreworkflow "github.com/gougoujiang/buildmax/internal/core/workflow"
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
	ID           string `json:"id"`
	Title        string `json:"title"`
	Kind         string `json:"kind"`
	RelativePath string `json:"relative_path,omitempty"`
	// ArtifactID is set on an output that is a durable artifact. It is the
	// whole address: a client opens it at /api/artifacts/{id} without needing
	// the run that produced it.
	ArtifactID       string               `json:"artifact_id,omitempty"`
	Filename         string               `json:"filename,omitempty"`
	MediaType        string               `json:"media_type,omitempty"`
	SizeBytes        int64                `json:"size_bytes,omitempty"`
	Preview          string               `json:"preview,omitempty"`
	PreviewTruncated bool                 `json:"preview_truncated"`
	Source           outputSourceResponse `json:"source"`
	CreatedAt        time.Time            `json:"created_at"`
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
	agentTasks []coretask.Task,
	stepsByTaskID map[string]coreworkflow.StepRun,
) ([]issueOutputResponse, *issueOutputResponse) {
	if h.cfg.RunOutputs == nil {
		return []issueOutputResponse{}, nil
	}
	outputs := make([]issueOutputResponse, 0)
	outputs = append(outputs, h.artifactOutputs(ctx, agentTasks, stepsByTaskID)...)
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
			TaskID:         t.ID,
			TaskRunID:      taskRunID,
			ConversationID: t.ConversationID,
		}
		if step, ok := stepsByTaskID[t.ID]; ok {
			source.WorkflowRunID = util.Ptr(step.WorkflowRunID)
			source.WorkflowStepRunID = util.Ptr(step.ID)
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
		return outputs[i].CreatedAt.After(outputs[j].CreatedAt)
	})
	var latest *issueOutputResponse
	if len(outputs) > 0 {
		l := outputs[0]
		latest = &l
	}
	return outputs, latest
}

// artifactOutputs lists what the issue's runs published as artifacts.
//
// They are looked up by the run that produced them, not owned by it: an
// artifact outlives the run and keeps its own address, and this is only the
// issue asking what its work produced. A deployment with no artifact store
// returns none, which is the same shape as runs that published nothing.
func (h *Handler) artifactOutputs(
	ctx context.Context,
	agentTasks []coretask.Task,
	stepsByTaskID map[string]coreworkflow.StepRun,
) []issueOutputResponse {
	if h.cfg.Artifacts == nil || !h.cfg.Artifacts.Available() {
		return nil
	}
	// Every run of every task, not each task's last one. A retried task has
	// earlier runs, and an artifact one of them published is still a thing the
	// team keeps — it does not stop being this issue's output because the task
	// was run again.
	taskIDs := make([]string, 0, len(agentTasks))
	tasksByID := make(map[string]coretask.Task, len(agentTasks))
	for _, t := range agentTasks {
		taskIDs = append(taskIDs, t.ID)
		tasksByID[t.ID] = t
	}
	runsByTask, err := h.runIDsByTask(ctx, taskIDs)
	if err != nil {
		return nil
	}
	runIDs := make([]string, 0, len(taskIDs))
	runToTask := make(map[string]coretask.Task, len(taskIDs))
	for taskID, runs := range runsByTask {
		for _, runID := range runs {
			runIDs = append(runIDs, runID)
			runToTask[runID] = tasksByID[taskID]
		}
	}
	bySource, err := h.cfg.Artifacts.ListBySource(ctx, runIDs)
	if err != nil {
		// Tolerated for the same reason a missing run output is: an issue's
		// flow response must not fail because one part of it could not be read.
		return nil
	}
	var out []issueOutputResponse
	for runID, artifacts := range bySource {
		t := runToTask[runID]
		source := outputSourceResponse{
			SourceType:     "task_run",
			TaskID:         t.ID,
			TaskRunID:      runID,
			ConversationID: t.ConversationID,
		}
		if step, ok := stepsByTaskID[t.ID]; ok {
			source.WorkflowRunID = util.Ptr(step.WorkflowRunID)
			source.WorkflowStepRunID = util.Ptr(step.ID)
			source.WorkflowStepID = util.Ptr(step.StepID)
		}
		for i := range artifacts {
			a := artifacts[i]
			title := a.Title
			if title == "" {
				title = a.Filename
			}
			out = append(out, issueOutputResponse{
				ID:         a.ID,
				Title:      title,
				Kind:       "artifact",
				ArtifactID: a.ID,
				Filename:   a.Filename,
				MediaType:  a.MediaType,
				SizeBytes:  a.SizeBytes,
				Source:     source,
				CreatedAt:  a.CreatedAt,
			})
		}
	}
	return out
}

// runIDsByTask lists every run of the given tasks, falling back to each task's
// last run when the store cannot answer. The fallback is not equivalent — it
// loses a retried task's earlier runs — but a degraded output list beats an
// issue page that will not load.
func (h *Handler) runIDsByTask(ctx context.Context, taskIDs []string) (map[string][]string, error) {
	if h.cfg.TaskRuns != nil {
		return h.cfg.TaskRuns.ListTaskRunIDsByTasks(ctx, taskIDs)
	}
	return map[string][]string{}, nil
}

func findResultItem(items []coretask.RunOutputFile) *coretask.RunOutputFile {
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
func (h *Handler) readArtifactPreview(ctx context.Context, t coretask.Task, taskRunID, relPath string) (string, bool) {
	if h.cfg.RunOutputStorage == nil {
		return "", false
	}
	if relPath != issueOutputResultFilename {
		// MVP only previews result.md via GetResult; other paths would
		// need GetRunOutputFile and a kind whitelist.
		return "", false
	}
	data, err := h.cfg.RunOutputStorage.GetResult(ctx, blob.RunRef{
		CreatedBy:      t.CreatedBy,
		ConversationID: t.ConversationID,
		TaskID:         t.ID,
		TaskRunID:      taskRunID,
	})
	if err != nil {
		if errors.Is(err, os.ErrNotExist) || errors.Is(err, apierr.ErrNotFound) {
			return "", false
		}
		// Other errors: log via http handler caller is not available here;
		// silently degrade — the spec requires not to fail the whole flow.
		return "", false
	}
	return truncatePreview(data, issueOutputPreviewBudget)
}
