package mock

import (
	"context"
	"fmt"
	"time"

	"buildmax/internal/infra/db"
)

// MockIssueStore is an in-memory IssueStore for tests.
type MockIssueStore struct {
	Issues []db.Issue
}

func (m *MockIssueStore) CreateIssue(_ context.Context, userID string, in db.CreateIssueInput) (*db.Issue, error) {
	return m.CreateIssueInTeam(context.Background(), "tm_personal", userID, in)
}

func (m *MockIssueStore) CreateIssueInTeam(_ context.Context, teamID, createdBy string, in db.CreateIssueInput) (*db.Issue, error) {
	issue := db.Issue{
		IssueID:      fmt.Sprintf("i_mock_%d", len(m.Issues)+1),
		UserID:       createdBy,
		TeamID:       teamID,
		Title:        in.Title,
		Description:  in.Description,
		Status:       db.IssueStatusTodo,
		CreatedBy:    createdBy,
		CreatedAt:    time.Now().Unix(),
		UpdatedAt:    time.Now().Unix(),
		AssigneeKind: nil,
		AssigneeID:   nil,
	}
	m.Issues = append(m.Issues, issue)
	return &m.Issues[len(m.Issues)-1], nil
}

func (m *MockIssueStore) ListIssuesByUser(_ context.Context, userID string, limit, offset int) ([]db.Issue, int, error) {
	var filtered []db.Issue
	for _, issue := range m.Issues {
		if issue.UserID == userID {
			filtered = append(filtered, issue)
		}
	}
	total := len(filtered)
	if offset > len(filtered) {
		return []db.Issue{}, total, nil
	}
	end := offset + limit
	if limit <= 0 || end > len(filtered) {
		end = len(filtered)
	}
	return filtered[offset:end], total, nil
}

func (m *MockIssueStore) ListIssuesByTeam(_ context.Context, teamID string, limit, offset int) ([]db.Issue, int, error) {
	var filtered []db.Issue
	for _, issue := range m.Issues {
		if issue.TeamID == teamID {
			filtered = append(filtered, issue)
		}
	}
	total := len(filtered)
	if offset > len(filtered) {
		return []db.Issue{}, total, nil
	}
	end := offset + limit
	if limit <= 0 || end > len(filtered) {
		end = len(filtered)
	}
	return filtered[offset:end], total, nil
}

func (m *MockIssueStore) GetIssue(_ context.Context, issueID string) (*db.Issue, error) {
	for i := range m.Issues {
		if m.Issues[i].IssueID == issueID {
			return &m.Issues[i], nil
		}
	}
	return nil, nil
}

func (m *MockIssueStore) UpdateIssue(_ context.Context, issueID, userID string, in db.UpdateIssueInput) (*db.Issue, error) {
	for i := range m.Issues {
		if m.Issues[i].IssueID != issueID || m.Issues[i].UserID != userID {
			continue
		}
		return m.applyIssueUpdate(i, in), nil
	}
	return nil, nil
}

func (m *MockIssueStore) UpdateIssueInTeam(_ context.Context, issueID, teamID string, in db.UpdateIssueInput) (*db.Issue, error) {
	for i := range m.Issues {
		if m.Issues[i].IssueID != issueID || m.Issues[i].TeamID != teamID {
			continue
		}
		return m.applyIssueUpdate(i, in), nil
	}
	return nil, nil
}

func (m *MockIssueStore) applyIssueUpdate(i int, in db.UpdateIssueInput) *db.Issue {
	if in.Title != nil {
		m.Issues[i].Title = *in.Title
	}
	if in.Description != nil {
		m.Issues[i].Description = *in.Description
	}
	if in.Status != nil {
		m.Issues[i].Status = *in.Status
	}
	if in.AssigneeKind != nil {
		if *in.AssigneeKind == "" {
			m.Issues[i].AssigneeKind = nil
		} else {
			m.Issues[i].AssigneeKind = in.AssigneeKind
		}
	}
	if in.AssigneeID != nil {
		if *in.AssigneeID == "" {
			m.Issues[i].AssigneeID = nil
		} else {
			m.Issues[i].AssigneeID = in.AssigneeID
		}
	}
	m.Issues[i].UpdatedAt = time.Now().Unix()
	return &m.Issues[i]
}
