package entity

import (
	"context"
	"errors"
	"time"

	"buildmax/internal/util"
	"gorm.io/gorm"
)

const (
	IssueStatusTodo       = "todo"
	IssueStatusInProgress = "in_progress"
	IssueStatusDone       = "done"
	IssueAssigneePerson   = "person"
	IssueAssigneeAgent    = "agent"
)

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

// CreateIssue creates a user-scoped issue with default status todo.
func (s *Store) CreateIssue(ctx context.Context, userID string, in CreateIssueInput) (*Issue, error) {
	now := time.Now().Unix()
	issue := &Issue{
		IssueID:      util.NewPrefixedID(util.PrefixIssue),
		UserID:       userID,
		Title:        in.Title,
		Description:  in.Description,
		Status:       IssueStatusTodo,
		CreatedBy:    userID,
		CreatedAt:    now,
		UpdatedAt:    now,
		AssigneeKind: nil,
		AssigneeID:   nil,
	}
	if err := s.db.WithContext(ctx).Create(issue).Error; err != nil {
		return nil, err
	}
	return issue, nil
}

// ListIssuesByUser returns issues for the user ordered by updated_at DESC.
func (s *Store) ListIssuesByUser(ctx context.Context, userID string, limit, offset int) ([]Issue, int, error) {
	var total int64
	if err := s.db.WithContext(ctx).Model(&Issue{}).Where("user_id = ?", userID).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var list []Issue
	q := s.db.WithContext(ctx).Where("user_id = ?", userID).Order("updated_at DESC")
	if limit > 0 {
		q = q.Limit(limit).Offset(offset)
	}
	if err := q.Find(&list).Error; err != nil {
		return nil, 0, err
	}
	return list, int(total), nil
}

// GetIssue returns the issue by issue_id, or (nil, nil) if not found.
func (s *Store) GetIssue(ctx context.Context, issueID string) (*Issue, error) {
	var issue Issue
	err := s.db.WithContext(ctx).Where("issue_id = ?", issueID).First(&issue).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &issue, nil
}

// UpdateIssue updates only provided fields. Returns (nil, nil) if not found or not owned by user.
func (s *Store) UpdateIssue(ctx context.Context, issueID, userID string, in UpdateIssueInput) (*Issue, error) {
	issue, err := s.GetIssue(ctx, issueID)
	if err != nil || issue == nil {
		return nil, err
	}
	if issue.UserID != userID {
		return nil, nil
	}
	updates := map[string]interface{}{
		"updated_at": time.Now().Unix(),
	}
	if in.Title != nil {
		updates["title"] = *in.Title
	}
	if in.Description != nil {
		updates["description"] = *in.Description
	}
	if in.Status != nil {
		updates["status"] = *in.Status
	}
	if in.AssigneeKind != nil {
		if *in.AssigneeKind == "" {
			updates["assignee_kind"] = nil
		} else {
			updates["assignee_kind"] = *in.AssigneeKind
		}
	}
	if in.AssigneeID != nil {
		if *in.AssigneeID == "" {
			updates["assignee_id"] = nil
		} else {
			updates["assignee_id"] = *in.AssigneeID
		}
	}
	if err := s.db.WithContext(ctx).Model(&Issue{}).Where("issue_id = ?", issueID).Updates(updates).Error; err != nil {
		return nil, err
	}
	return s.GetIssue(ctx, issueID)
}
