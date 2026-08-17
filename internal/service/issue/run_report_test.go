package issue

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/gougoujiang/buildmax/internal/core/model"
	"github.com/gougoujiang/buildmax/internal/mock"
	"github.com/gougoujiang/buildmax/internal/util"
)

func reporterFor(task model.Task, comments *mock.MockIssueCommentStore) *RunReporter {
	return &RunReporter{
		Tasks:    &mock.MockTaskStore{List: []model.Task{task}},
		Comments: comments,
	}
}

func TestReportRunTerminal_WritesOneAgentComment(t *testing.T) {
	comments := &mock.MockIssueCommentStore{}
	reporter := reporterFor(model.Task{
		TaskID:  "t_1",
		IssueID: util.Ptr("i_1"),
		AgentID: util.Ptr("a_1"),
	}, comments)
	err := reporter.ReportRunTerminal(context.Background(), model.TaskRunTerminalInfo{
		TaskRunID: "r_1",
		TaskID:    "t_1",
		Status:    string(model.RunStatusSucceeded),
		Output:    util.Ptr("Shipped the migration."),
	})
	if err != nil {
		t.Fatalf("ReportRunTerminal: %v", err)
	}
	if len(comments.Comments) != 1 {
		t.Fatalf("wrote %d comments, want exactly 1 per terminal run", len(comments.Comments))
	}
	got := comments.Comments[0]
	if got.AuthorKind != model.IssueCommentAuthorAgent || got.AuthorID != "a_1" {
		t.Fatalf("author = %s/%s, want agent/a_1", got.AuthorKind, got.AuthorID)
	}
	if got.SourceTaskRunID == nil || *got.SourceTaskRunID != "r_1" {
		t.Fatalf("source_task_run_id = %v, want r_1", got.SourceTaskRunID)
	}
	if got.Body != "Shipped the migration." {
		t.Fatalf("body = %q", got.Body)
	}
}

// A task with no issue has nowhere to report, and a run that said nothing is
// not worth a line in the thread.
func TestReportRunTerminal_SilentCases(t *testing.T) {
	cases := []struct {
		name string
		task model.Task
		info model.TaskRunTerminalInfo
	}{
		{
			name: "task not on an issue",
			task: model.Task{TaskID: "t_1"},
			info: model.TaskRunTerminalInfo{TaskID: "t_1", Status: string(model.RunStatusSucceeded), Output: util.Ptr("done")},
		},
		{
			name: "success with no output",
			task: model.Task{TaskID: "t_1", IssueID: util.Ptr("i_1")},
			info: model.TaskRunTerminalInfo{TaskID: "t_1", Status: string(model.RunStatusSucceeded)},
		},
		{
			name: "success with blank output",
			task: model.Task{TaskID: "t_1", IssueID: util.Ptr("i_1")},
			info: model.TaskRunTerminalInfo{TaskID: "t_1", Status: string(model.RunStatusSucceeded), Output: util.Ptr("  \n ")},
		},
		{
			name: "failure with no message",
			task: model.Task{TaskID: "t_1", IssueID: util.Ptr("i_1")},
			info: model.TaskRunTerminalInfo{TaskID: "t_1", Status: string(model.RunStatusFailed)},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			comments := &mock.MockIssueCommentStore{}
			if err := reporterFor(tc.task, comments).ReportRunTerminal(context.Background(), tc.info); err != nil {
				t.Fatalf("ReportRunTerminal: %v", err)
			}
			if len(comments.Comments) != 0 {
				t.Fatalf("wrote %d comments, want none", len(comments.Comments))
			}
		})
	}
}

func TestReportRunTerminal_FailureReportsTheError(t *testing.T) {
	comments := &mock.MockIssueCommentStore{}
	reporter := reporterFor(model.Task{TaskID: "t_1", IssueID: util.Ptr("i_1"), AgentID: util.Ptr("a_1")}, comments)
	if err := reporter.ReportRunTerminal(context.Background(), model.TaskRunTerminalInfo{
		TaskRunID:    "r_1",
		TaskID:       "t_1",
		Status:       string(model.RunStatusFailed),
		ErrorMessage: util.Ptr("model refused the tool call"),
	}); err != nil {
		t.Fatalf("ReportRunTerminal: %v", err)
	}
	if len(comments.Comments) != 1 {
		t.Fatalf("wrote %d comments, want 1", len(comments.Comments))
	}
	if !strings.Contains(comments.Comments[0].Body, "model refused the tool call") {
		t.Fatalf("body = %q, want the error message", comments.Comments[0].Body)
	}
}

func TestReportRunTerminal_TruncatesLongOutput(t *testing.T) {
	comments := &mock.MockIssueCommentStore{}
	reporter := reporterFor(model.Task{TaskID: "t_1", IssueID: util.Ptr("i_1")}, comments)
	if err := reporter.ReportRunTerminal(context.Background(), model.TaskRunTerminalInfo{
		TaskRunID: "r_1",
		TaskID:    "t_1",
		Status:    string(model.RunStatusSucceeded),
		Output:    util.Ptr(strings.Repeat("é", runSummaryLimit)),
	}); err != nil {
		t.Fatalf("ReportRunTerminal: %v", err)
	}
	body := comments.Comments[0].Body
	if !strings.Contains(body, "truncated") {
		t.Fatalf("long output was not marked truncated: %q", body[max(0, len(body)-80):])
	}
	// The cut lands on a rune boundary, never inside a multi-byte character.
	if !utf8ValidPrefix(body) {
		t.Fatalf("truncation split a multi-byte character")
	}
}

// A failed comment write must not surface as a failed run. The reporter returns
// the error so the caller can log it; the caller is what decides it is not
// fatal, and internal/server/server.go logs and continues.
func TestReportRunTerminal_StoreFailureIsReturnedNotSwallowed(t *testing.T) {
	sentinel := errors.New("comment store down")
	comments := &mock.MockIssueCommentStore{CreateErr: sentinel}
	reporter := reporterFor(model.Task{TaskID: "t_1", IssueID: util.Ptr("i_1")}, comments)
	err := reporter.ReportRunTerminal(context.Background(), model.TaskRunTerminalInfo{
		TaskRunID: "r_1",
		TaskID:    "t_1",
		Status:    string(model.RunStatusSucceeded),
		Output:    util.Ptr("done"),
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("err = %v, want %v", err, sentinel)
	}
}

func TestReportRunTerminal_NoStoreIsANoop(t *testing.T) {
	reporter := &RunReporter{}
	if err := reporter.ReportRunTerminal(context.Background(), model.TaskRunTerminalInfo{TaskID: "t_1"}); err != nil {
		t.Fatalf("ReportRunTerminal with no stores: %v", err)
	}
}

func utf8ValidPrefix(s string) bool {
	for _, r := range s {
		if r == '�' {
			return false
		}
	}
	return true
}
