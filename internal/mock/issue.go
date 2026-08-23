package mock

import (
	"context"
	"fmt"
	"time"

	"github.com/gougoujiang/buildmax/internal/core/model"
)

// MockIssueStore is an in-memory IssueStore for tests.
type MockIssueStore struct {
	Issues []model.Issue
}

func (m *MockIssueStore) CreateIssue(_ context.Context, userID string, in model.CreateIssueInput) (*model.Issue, error) {
	return m.CreateIssueInTeam(context.Background(), "tm_personal", userID, in)
}

func (m *MockIssueStore) CreateIssueInTeam(_ context.Context, teamID, createdBy string, in model.CreateIssueInput) (*model.Issue, error) {
	issue := model.Issue{
		ID:            fmt.Sprintf("i_mock_%d", len(m.Issues)+1),
		UserID:        createdBy,
		TeamID:        teamID,
		ParentIssueID: in.ParentIssueID,
		Title:         in.Title,
		Description:   in.Description,
		Status:        model.IssueStatusTodo,
		CreatedBy:     createdBy,
		CreatedAt:     time.Now().UTC(),
		UpdatedAt:     time.Now().UTC(),
		AssigneeKind:  nil,
		AssigneeID:    nil,
	}
	m.Issues = append(m.Issues, issue)
	return &m.Issues[len(m.Issues)-1], nil
}

func (m *MockIssueStore) ListIssuesByUser(_ context.Context, userID string, limit, offset int) ([]model.Issue, int, error) {
	var filtered []model.Issue
	for _, issue := range m.Issues {
		if issue.UserID == userID {
			filtered = append(filtered, issue)
		}
	}
	total := len(filtered)
	if offset > len(filtered) {
		return []model.Issue{}, total, nil
	}
	end := offset + limit
	if limit <= 0 || end > len(filtered) {
		end = len(filtered)
	}
	return filtered[offset:end], total, nil
}

func (m *MockIssueStore) ListIssuesByTeam(_ context.Context, teamID string, filter model.ListIssuesFilter, limit, offset int) ([]model.Issue, int, error) {
	var filtered []model.Issue
	for _, issue := range m.Issues {
		if issue.TeamID != teamID {
			continue
		}
		switch {
		case filter.TopLevelOnly && issue.ParentIssueID != nil:
			continue
		case filter.ParentIssueID != "" && (issue.ParentIssueID == nil || *issue.ParentIssueID != filter.ParentIssueID):
			continue
		}
		filtered = append(filtered, issue)
	}
	total := len(filtered)
	if offset > len(filtered) {
		return []model.Issue{}, total, nil
	}
	end := offset + limit
	if limit <= 0 || end > len(filtered) {
		end = len(filtered)
	}
	return filtered[offset:end], total, nil
}

func (m *MockIssueStore) ListIssueChildren(_ context.Context, parentIssueID string) ([]model.Issue, error) {
	out := []model.Issue{}
	if parentIssueID == "" {
		return out, nil
	}
	for _, issue := range m.Issues {
		if issue.ParentIssueID != nil && *issue.ParentIssueID == parentIssueID {
			out = append(out, issue)
		}
	}
	return out, nil
}

func (m *MockIssueStore) ChildStatsForIssues(_ context.Context, issueIDs []string) (map[string]model.IssueChildStats, error) {
	wanted := map[string]bool{}
	for _, id := range issueIDs {
		wanted[id] = true
	}
	out := map[string]model.IssueChildStats{}
	for _, issue := range m.Issues {
		if issue.ParentIssueID == nil || !wanted[*issue.ParentIssueID] {
			continue
		}
		stats := out[*issue.ParentIssueID]
		stats.Total++
		if issue.Status == model.IssueStatusDone {
			stats.Done++
		}
		out[*issue.ParentIssueID] = stats
	}
	return out, nil
}

func (m *MockIssueStore) GetIssue(_ context.Context, issueID string) (*model.Issue, error) {
	for i := range m.Issues {
		if m.Issues[i].ID == issueID {
			return &m.Issues[i], nil
		}
	}
	return nil, nil
}

func (m *MockIssueStore) UpdateIssue(_ context.Context, issueID, userID string, in model.UpdateIssueInput) (*model.Issue, error) {
	for i := range m.Issues {
		if m.Issues[i].ID != issueID || m.Issues[i].UserID != userID {
			continue
		}
		return m.applyIssueUpdate(i, in), nil
	}
	return nil, nil
}

func (m *MockIssueStore) UpdateIssueInTeam(_ context.Context, issueID, teamID string, in model.UpdateIssueInput) (*model.Issue, error) {
	for i := range m.Issues {
		if m.Issues[i].ID != issueID || m.Issues[i].TeamID != teamID {
			continue
		}
		return m.applyIssueUpdate(i, in), nil
	}
	return nil, nil
}

func (m *MockIssueStore) applyIssueUpdate(i int, in model.UpdateIssueInput) *model.Issue {
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
	if in.ParentIssueID != nil {
		if *in.ParentIssueID == "" {
			m.Issues[i].ParentIssueID = nil
		} else {
			m.Issues[i].ParentIssueID = in.ParentIssueID
		}
	}
	m.Issues[i].UpdatedAt = time.Now().UTC()
	return &m.Issues[i]
}
