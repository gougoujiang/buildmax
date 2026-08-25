package issue

import (
	"context"
	"time"
)

const (
	StatusTodo       = "todo"
	StatusInProgress = "in_progress"
	StatusDone       = "done"
	AssigneePerson   = "person"
	AssigneeAgent    = "agent"
	AssigneeWorkflow = "workflow"
)

// Issue comment author kinds. A comment is written by a person, reported by an
// agent run, or stated by the server itself.
const (
	CommentAuthorUser   = "user"
	CommentAuthorAgent  = "agent"
	CommentAuthorSystem = "system"
)

// Issue is the user-facing work-management object. It is intentionally separate
// from low-level task/task_run execution records.
//
// ParentIssueID makes the issue a sub-issue of another issue in the same team.
// The hierarchy is capped at two levels — a parent must itself have no parent —
// which is enforced in internal/service/issue, not by the schema. See
// docs/design/issue-model.md.
type Issue struct {
	ID            string    `json:"id"`
	UserID        string    `json:"user_id"`
	TeamID        string    `json:"team_id,omitempty"`
	ParentIssueID *string   `json:"parent_issue_id,omitempty"`
	Title         string    `json:"title"`
	Description   string    `json:"description"`
	Status        string    `json:"status"`
	AssigneeKind  *string   `json:"assignee_kind,omitempty"`
	AssigneeID    *string   `json:"assignee_id,omitempty"`
	CreatedBy     string    `json:"created_by"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type CreateInput struct {
	Title         string
	Description   string
	ParentIssueID *string
}

type UpdateInput struct {
	Title         *string
	Description   *string
	Status        *string
	AssigneeKind  *string
	AssigneeID    *string
	ParentIssueID *string
}

// ListFilter narrows a team issue listing. The zero value lists every
// issue in the team, which is what callers predating sub-issues expect.
type ListFilter struct {
	// TopLevelOnly restricts the listing to issues with no parent.
	TopLevelOnly bool
	// ParentIssueID restricts the listing to one parent's children. It is
	// ignored when TopLevelOnly is set.
	ParentIssueID string
}

// ChildStats is a parent's sub-issue progress. It is always computed, never
// stored: a persisted counter is a second source of truth that drifts the first
// time a status write fails between the two rows.
type ChildStats struct {
	Total int `json:"total"`
	Done  int `json:"done"`
}

// Store provides issue persistence. Issues are user-scoped.
type Store interface {
	CreateIssue(ctx context.Context, userID string, in CreateInput) (*Issue, error)
	CreateIssueInTeam(ctx context.Context, teamID, createdBy string, in CreateInput) (*Issue, error)
	ListIssuesByUser(ctx context.Context, userID string, limit, offset int) ([]Issue, int, error)
	ListIssuesByTeam(ctx context.Context, teamID string, filter ListFilter, limit, offset int) ([]Issue, int, error)
	ListIssueChildren(ctx context.Context, parentIssueID string) ([]Issue, error)
	// ChildStatsForIssues returns sub-issue progress keyed by parent issue ID.
	// Parents with no children are absent from the map rather than present with
	// a zero value, so callers must treat a miss as "no sub-issues".
	ChildStatsForIssues(ctx context.Context, issueIDs []string) (map[string]ChildStats, error)
	GetIssue(ctx context.Context, issueID string) (*Issue, error)
	UpdateIssue(ctx context.Context, issueID, userID string, in UpdateInput) (*Issue, error)
	UpdateIssueInTeam(ctx context.Context, issueID, teamID string, in UpdateInput) (*Issue, error)
}

// Comment is one statement about an issue, addressed to people.
//
// It is deliberately not a conversation_message: that table is LLM turn history
// carrying roles and tool traffic, replayed into a model context. A comment
// outlives any particular conversation and is never replayed. See
// docs/design/issue-model.md.
type Comment struct {
	ID              string    `json:"id"`
	IssueID         string    `json:"issue_id"`
	AuthorKind      string    `json:"author_kind"`
	AuthorID        string    `json:"author_id"`
	Body            string    `json:"body"`
	SourceTaskID    *string   `json:"source_task_id,omitempty"`
	SourceTaskRunID *string   `json:"source_task_run_id,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
	// EditedAt is nil until the body is changed. Its absence is meaningful:
	// it means the text is as first written.
	EditedAt *time.Time `json:"edited_at,omitempty"`
}

type CreateCommentInput struct {
	IssueID         string
	AuthorKind      string
	AuthorID        string
	Body            string
	SourceTaskID    *string
	SourceTaskRunID *string
}

// CommentStore provides issue comment persistence. A comment's team is its
// issue's team; the row carries no team_id of its own, so every caller
// authorizes through the issue.
type CommentStore interface {
	CreateIssueComment(ctx context.Context, in CreateCommentInput) (*Comment, error)
	// ListIssueComments returns comments oldest first — a thread reads in the
	// order it was written.
	ListIssueComments(ctx context.Context, issueID string, limit, offset int) ([]Comment, int, error)
	GetIssueComment(ctx context.Context, commentID string) (*Comment, error)
	UpdateIssueComment(ctx context.Context, commentID, body string) (*Comment, error)
	DeleteIssueComment(ctx context.Context, commentID string) error
	// CountIssueComments returns comment totals keyed by issue ID. Issues with
	// no comments are absent from the map.
	CountIssueComments(ctx context.Context, issueIDs []string) (map[string]int, error)
}
