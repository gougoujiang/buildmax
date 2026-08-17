package issue

import (
	"context"
	"strings"
	"unicode/utf8"

	"github.com/gougoujiang/buildmax/internal/core/model"
)

// runSummaryLimit bounds the body of an agent-authored comment.
//
// The thread carries a statement that a run finished and what it said, not the
// run's output. The full text stays where it already lives — the task's output
// and its artifacts, aggregated into the issue's outputs — so raising this
// limit would duplicate content rather than reveal any.
const runSummaryLimit = 2000

// RunReporter posts an agent comment on the issue a finished task run belongs
// to.
//
// It is driven by the terminal-run callback, which fires after the worker has
// already been answered. Every failure path here returns without an error
// reaching that response: a comment that could not be written must not turn a
// completed run into a failed one.
type RunReporter struct {
	Tasks    model.TaskStore
	Comments model.IssueCommentStore
}

// ReportRunTerminal writes one comment for a run that reached a terminal
// status, and nothing at all when there is no issue, no store, or nothing to
// say.
//
// One comment per terminal run is the whole budget. A run that streamed for
// twenty minutes still produces one line in the thread.
func (r *RunReporter) ReportRunTerminal(ctx context.Context, info model.TaskRunTerminalInfo) error {
	if r == nil || r.Tasks == nil || r.Comments == nil {
		return nil
	}
	task, err := r.Tasks.GetTask(ctx, info.TaskID)
	if err != nil {
		return err
	}
	if task == nil || task.IssueID == nil || *task.IssueID == "" {
		return nil
	}
	body := runSummaryBody(info)
	if body == "" {
		return nil
	}
	taskID := info.TaskID
	runID := info.TaskRunID
	_, err = r.Comments.CreateIssueComment(ctx, model.CreateIssueCommentInput{
		IssueID:         *task.IssueID,
		AuthorKind:      model.IssueCommentAuthorAgent,
		AuthorID:        agentIDOf(task),
		Body:            body,
		SourceTaskID:    &taskID,
		SourceTaskRunID: &runID,
	})
	return err
}

func agentIDOf(task *model.Task) string {
	if task.AgentID != nil {
		return *task.AgentID
	}
	return ""
}

// runSummaryBody renders what the run reported. It returns "" for a run that
// said nothing — a silent success is not worth a comment, and a thread of
// content-free notifications is what makes people stop reading one.
func runSummaryBody(info model.TaskRunTerminalInfo) string {
	if info.Status == string(model.RunStatusFailed) {
		if info.ErrorMessage == nil {
			return ""
		}
		detail := strings.TrimSpace(*info.ErrorMessage)
		if detail == "" {
			return ""
		}
		return "Run failed.\n\n" + truncateRunes(detail, runSummaryLimit)
	}
	if info.Output == nil {
		return ""
	}
	detail := strings.TrimSpace(*info.Output)
	if detail == "" {
		return ""
	}
	return truncateRunes(detail, runSummaryLimit)
}

// truncateRunes cuts on a rune boundary so a multi-byte character is never
// split in half.
func truncateRunes(s string, limit int) string {
	if len(s) <= limit {
		return s
	}
	trimmed := s[:limit]
	for len(trimmed) > 0 && !utf8.ValidString(trimmed) {
		trimmed = trimmed[:len(trimmed)-1]
	}
	return trimmed + "\n\n…truncated; see the issue's results for the full output."
}
