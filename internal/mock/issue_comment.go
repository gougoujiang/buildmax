package mock

import (
	"context"
	"fmt"
	"time"

	"github.com/gougoujiang/buildmax/internal/core/model"
)

// MockIssueCommentStore is an in-memory IssueCommentStore for tests.
type MockIssueCommentStore struct {
	Comments []model.IssueComment
	// CreateErr, when set, is returned by CreateIssueComment. It exists so a
	// test can prove a failed comment write does not fail the run that
	// triggered it.
	CreateErr error
}

func (m *MockIssueCommentStore) CreateIssueComment(_ context.Context, in model.CreateIssueCommentInput) (*model.IssueComment, error) {
	if m.CreateErr != nil {
		return nil, m.CreateErr
	}
	comment := model.IssueComment{
		ID:              fmt.Sprintf("ic_mock_%d", len(m.Comments)+1),
		IssueID:         in.IssueID,
		AuthorKind:      in.AuthorKind,
		AuthorID:        in.AuthorID,
		Body:            in.Body,
		SourceTaskID:    in.SourceTaskID,
		SourceTaskRunID: in.SourceTaskRunID,
		CreatedAt:       time.Now().Unix(),
	}
	m.Comments = append(m.Comments, comment)
	return &m.Comments[len(m.Comments)-1], nil
}

func (m *MockIssueCommentStore) ListIssueComments(_ context.Context, issueID string, limit, offset int) ([]model.IssueComment, int, error) {
	var filtered []model.IssueComment
	for _, comment := range m.Comments {
		if comment.IssueID == issueID {
			filtered = append(filtered, comment)
		}
	}
	total := len(filtered)
	if offset > len(filtered) {
		return []model.IssueComment{}, total, nil
	}
	end := offset + limit
	if limit <= 0 || end > len(filtered) {
		end = len(filtered)
	}
	return filtered[offset:end], total, nil
}

func (m *MockIssueCommentStore) GetIssueComment(_ context.Context, commentID string) (*model.IssueComment, error) {
	for i := range m.Comments {
		if m.Comments[i].ID == commentID {
			return &m.Comments[i], nil
		}
	}
	return nil, nil
}

func (m *MockIssueCommentStore) UpdateIssueComment(_ context.Context, commentID, body string) (*model.IssueComment, error) {
	for i := range m.Comments {
		if m.Comments[i].ID != commentID {
			continue
		}
		now := time.Now().Unix()
		m.Comments[i].Body = body
		m.Comments[i].EditedAt = &now
		return &m.Comments[i], nil
	}
	return nil, nil
}

func (m *MockIssueCommentStore) DeleteIssueComment(_ context.Context, commentID string) error {
	for i := range m.Comments {
		if m.Comments[i].ID == commentID {
			m.Comments = append(m.Comments[:i], m.Comments[i+1:]...)
			return nil
		}
	}
	return nil
}

func (m *MockIssueCommentStore) CountIssueComments(_ context.Context, issueIDs []string) (map[string]int, error) {
	wanted := map[string]bool{}
	for _, id := range issueIDs {
		wanted[id] = true
	}
	out := map[string]int{}
	for _, comment := range m.Comments {
		if wanted[comment.IssueID] {
			out[comment.IssueID]++
		}
	}
	return out, nil
}
