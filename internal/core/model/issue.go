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

// Issue is the user-facing work-management object. It is intentionally separate
// from low-level task/task_run execution records.
type Issue struct {
	ID           uint    `json:"-"`
	IssueID      string  `json:"issue_id"`
	UserID       string  `json:"user_id"`
	TeamID       string  `json:"team_id,omitempty"`
	Title        string  `json:"title"`
	Description  string  `json:"description"`
	Status       string  `json:"status"`
	AssigneeKind *string `json:"assignee_kind,omitempty"`
	AssigneeID   *string `json:"assignee_id,omitempty"`
	CreatedBy    string  `json:"created_by"`
	CreatedAt    int64   `json:"created_at"`
	UpdatedAt    int64   `json:"updated_at"`
}

type CreateIssueInput struct {
	Title       string
	Description string
}

type UpdateIssueInput struct {
	Title        *string
	Description  *string
	Status       *string
	AssigneeKind *string
	AssigneeID   *string
}

// IssueStore provides issue persistence. Issues are user-scoped.
type IssueStore interface {
	CreateIssue(ctx context.Context, userID string, in CreateIssueInput) (*Issue, error)
	CreateIssueInTeam(ctx context.Context, teamID, createdBy string, in CreateIssueInput) (*Issue, error)
	ListIssuesByUser(ctx context.Context, userID string, limit, offset int) ([]Issue, int, error)
	ListIssuesByTeam(ctx context.Context, teamID string, limit, offset int) ([]Issue, int, error)
	GetIssue(ctx context.Context, issueID string) (*Issue, error)
	UpdateIssue(ctx context.Context, issueID, userID string, in UpdateIssueInput) (*Issue, error)
	UpdateIssueInTeam(ctx context.Context, issueID, teamID string, in UpdateIssueInput) (*Issue, error)
}
