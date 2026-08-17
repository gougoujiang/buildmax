package model

import "context"

const (
	IssueStatusTodo       = "todo"
	IssueStatusInProgress = "in_progress"
	IssueStatusDone       = "done"
	IssueAssigneePerson   = "person"
	IssueAssigneeAgent    = "agent"
	IssueAssigneeWorkflow = "workflow"
)

// Issue comment author kinds. A comment is written by a person, reported by an
// agent run, or stated by the server itself.
const (
	IssueCommentAuthorUser   = "user"
	IssueCommentAuthorAgent  = "agent"
	IssueCommentAuthorSystem = "system"
)

// Issue is the user-facing work-management object. It is intentionally separate
// from low-level task/task_run execution records.
//
// ParentIssueID makes the issue a sub-issue of another issue in the same team.
// The hierarchy is capped at two levels — a parent must itself have no parent —
// which is enforced in internal/service/issue, not by the schema. See
// docs/design/issue-model.md.
type Issue struct {
	ID            uint    `json:"-"`
	IssueID       string  `json:"issue_id"`
	UserID        string  `json:"user_id"`
	TeamID        string  `json:"team_id,omitempty"`
	ParentIssueID *string `json:"parent_issue_id,omitempty"`
	Title         string  `json:"title"`
	Description   string  `json:"description"`
	Status        string  `json:"status"`
	AssigneeKind  *string `json:"assignee_kind,omitempty"`
	AssigneeID    *string `json:"assignee_id,omitempty"`
	CreatedBy     string  `json:"created_by"`
	CreatedAt     int64   `json:"created_at"`
	UpdatedAt     int64   `json:"updated_at"`
}

type CreateIssueInput struct {
	Title         string
	Description   string
	ParentIssueID *string
}

type UpdateIssueInput struct {
	Title         *string
	Description   *string
	Status        *string
	AssigneeKind  *string
	AssigneeID    *string
	ParentIssueID *string
}

// ListIssuesFilter narrows a team issue listing. The zero value lists every
// issue in the team, which is what callers predating sub-issues expect.
type ListIssuesFilter struct {
	// TopLevelOnly restricts the listing to issues with no parent.
	TopLevelOnly bool
	// ParentIssueID restricts the listing to one parent's children. It is
	// ignored when TopLevelOnly is set.
	ParentIssueID string
}

// IssueChildStats is a parent's sub-issue progress. It is always computed, never
// stored: a persisted counter is a second source of truth that drifts the first
// time a status write fails between the two rows.
type IssueChildStats struct {
	Total int `json:"total"`
	Done  int `json:"done"`
}

// IssueStore provides issue persistence. Issues are user-scoped.
type IssueStore interface {
	CreateIssue(ctx context.Context, userID string, in CreateIssueInput) (*Issue, error)
	CreateIssueInTeam(ctx context.Context, teamID, createdBy string, in CreateIssueInput) (*Issue, error)
	ListIssuesByUser(ctx context.Context, userID string, limit, offset int) ([]Issue, int, error)
	ListIssuesByTeam(ctx context.Context, teamID string, filter ListIssuesFilter, limit, offset int) ([]Issue, int, error)
	ListIssueChildren(ctx context.Context, parentIssueID string) ([]Issue, error)
	// ChildStatsForIssues returns sub-issue progress keyed by parent issue ID.
	// Parents with no children are absent from the map rather than present with
	// a zero value, so callers must treat a miss as "no sub-issues".
	ChildStatsForIssues(ctx context.Context, issueIDs []string) (map[string]IssueChildStats, error)
	GetIssue(ctx context.Context, issueID string) (*Issue, error)
	UpdateIssue(ctx context.Context, issueID, userID string, in UpdateIssueInput) (*Issue, error)
	UpdateIssueInTeam(ctx context.Context, issueID, teamID string, in UpdateIssueInput) (*Issue, error)
}

// IssueComment is one statement about an issue, addressed to people.
//
// It is deliberately not a conversation_message: that table is LLM turn history
// carrying roles and tool traffic, replayed into a model context. A comment
// outlives any particular conversation and is never replayed. See
// docs/design/issue-model.md.
type IssueComment struct {
	ID              uint    `json:"-"`
	IssueCommentID  string  `json:"issue_comment_id"`
	IssueID         string  `json:"issue_id"`
	AuthorKind      string  `json:"author_kind"`
	AuthorID        string  `json:"author_id"`
	Body            string  `json:"body"`
	SourceTaskID    *string `json:"source_task_id,omitempty"`
	SourceTaskRunID *string `json:"source_task_run_id,omitempty"`
	CreatedAt       int64   `json:"created_at"`
	// EditedAt is nil until the body is changed. Its absence is meaningful:
	// it means the text is as first written.
	EditedAt *int64 `json:"edited_at,omitempty"`
}

type CreateIssueCommentInput struct {
	IssueID         string
	AuthorKind      string
	AuthorID        string
	Body            string
	SourceTaskID    *string
	SourceTaskRunID *string
}

// IssueCommentStore provides issue comment persistence. A comment's team is its
// issue's team; the row carries no team_id of its own, so every caller
// authorizes through the issue.
type IssueCommentStore interface {
	CreateIssueComment(ctx context.Context, in CreateIssueCommentInput) (*IssueComment, error)
	// ListIssueComments returns comments oldest first — a thread reads in the
	// order it was written.
	ListIssueComments(ctx context.Context, issueID string, limit, offset int) ([]IssueComment, int, error)
	GetIssueComment(ctx context.Context, commentID string) (*IssueComment, error)
	UpdateIssueComment(ctx context.Context, commentID, body string) (*IssueComment, error)
	DeleteIssueComment(ctx context.Context, commentID string) error
	// CountIssueComments returns comment totals keyed by issue ID. Issues with
	// no comments are absent from the map.
	CountIssueComments(ctx context.Context, issueIDs []string) (map[string]int, error)
}
