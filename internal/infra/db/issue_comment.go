package db

import (
	"context"
	"errors"
	"time"

	"github.com/gougoujiang/buildmax/internal/core/model"
	"github.com/gougoujiang/buildmax/internal/util"
	"gorm.io/gorm"
)

// issueCommentRow is one statement about an issue.
//
// It carries no team_id: a comment's team is its issue's team, and every
// handler already loads the issue to authorize. Denormalizing the
// authorization key would give it a second place to be wrong. This follows
// conversation_message, which resolves its team through its conversation.
type issueCommentRow struct {
	ID              uint    `gorm:"primaryKey;autoIncrement"`
	IssueCommentID  string  `gorm:"column:issue_comment_id;type:varchar(64);uniqueIndex;not null"`
	IssueID         string  `gorm:"column:issue_id;type:varchar(64);not null;index"`
	AuthorKind      string  `gorm:"type:varchar(16);not null"`
	AuthorID        string  `gorm:"type:varchar(64);not null"`
	Body            string  `gorm:"type:text;not null"`
	SourceTaskID    *string `gorm:"column:source_task_id;type:varchar(64)"`
	SourceTaskRunID *string `gorm:"column:source_task_run_id;type:varchar(64)"`
	CreatedAt       int64   `gorm:"autoCreateTime"`
	EditedAt        *int64  `gorm:"column:edited_at"`
}

func (issueCommentRow) TableName() string { return "issue_comment" }

func toIssueComment(row *issueCommentRow) *model.IssueComment {
	if row == nil {
		return nil
	}
	return &model.IssueComment{
		ID:              row.ID,
		IssueCommentID:  row.IssueCommentID,
		IssueID:         row.IssueID,
		AuthorKind:      row.AuthorKind,
		AuthorID:        row.AuthorID,
		Body:            row.Body,
		SourceTaskID:    row.SourceTaskID,
		SourceTaskRunID: row.SourceTaskRunID,
		CreatedAt:       row.CreatedAt,
		EditedAt:        row.EditedAt,
	}
}

func toIssueComments(rows []issueCommentRow) []model.IssueComment {
	out := make([]model.IssueComment, len(rows))
	for i := range rows {
		out[i] = *toIssueComment(&rows[i])
	}
	return out
}

// CreateIssueComment appends a comment to an issue.
func (s *Store) CreateIssueComment(ctx context.Context, in model.CreateIssueCommentInput) (*model.IssueComment, error) {
	row := &issueCommentRow{
		IssueCommentID:  util.NewPrefixedID(util.PrefixIssueComment),
		IssueID:         in.IssueID,
		AuthorKind:      in.AuthorKind,
		AuthorID:        in.AuthorID,
		Body:            in.Body,
		SourceTaskID:    in.SourceTaskID,
		SourceTaskRunID: in.SourceTaskRunID,
		CreatedAt:       time.Now().Unix(),
	}
	if err := s.db.WithContext(ctx).Create(row).Error; err != nil {
		return nil, err
	}
	return toIssueComment(row), nil
}

// ListIssueComments returns an issue's comments oldest first.
//
// Ordering is created_at then id. Prefixed IDs are random rather than
// time-ordered, so issue_comment_id is never a sort key.
func (s *Store) ListIssueComments(ctx context.Context, issueID string, limit, offset int) ([]model.IssueComment, int, error) {
	var total int64
	if err := s.db.WithContext(ctx).Model(&issueCommentRow{}).Where("issue_id = ?", issueID).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var list []issueCommentRow
	q := s.db.WithContext(ctx).Where("issue_id = ?", issueID).Order("created_at ASC, id ASC")
	if limit > 0 {
		q = q.Limit(limit).Offset(offset)
	}
	if err := q.Find(&list).Error; err != nil {
		return nil, 0, err
	}
	return toIssueComments(list), int(total), nil
}

// GetIssueComment returns the comment by issue_comment_id, or (nil, nil) if not found.
func (s *Store) GetIssueComment(ctx context.Context, commentID string) (*model.IssueComment, error) {
	var row issueCommentRow
	err := s.db.WithContext(ctx).Where("issue_comment_id = ?", commentID).First(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return toIssueComment(&row), nil
}

// UpdateIssueComment replaces the body and stamps edited_at.
func (s *Store) UpdateIssueComment(ctx context.Context, commentID, body string) (*model.IssueComment, error) {
	updates := map[string]interface{}{
		"body":      body,
		"edited_at": time.Now().Unix(),
	}
	if err := s.db.WithContext(ctx).Model(&issueCommentRow{}).Where("issue_comment_id = ?", commentID).Updates(updates).Error; err != nil {
		return nil, err
	}
	return s.GetIssueComment(ctx, commentID)
}

// DeleteIssueComment removes the comment.
//
// The delete is hard: this schema has no soft-delete precedent, and adding
// deleted_at to one table would set a convention every other table has to
// answer to. See docs/design/issue-model.md.
func (s *Store) DeleteIssueComment(ctx context.Context, commentID string) error {
	return s.db.WithContext(ctx).Where("issue_comment_id = ?", commentID).Delete(&issueCommentRow{}).Error
}

// CountIssueComments returns comment totals for the given issues in one grouped
// query. Issues with no comments are absent from the map.
func (s *Store) CountIssueComments(ctx context.Context, issueIDs []string) (map[string]int, error) {
	out := map[string]int{}
	if len(issueIDs) == 0 {
		return out, nil
	}
	var rows []struct {
		IssueID string
		Total   int
	}
	if err := s.db.WithContext(ctx).Model(&issueCommentRow{}).
		Select("issue_id, COUNT(*) AS total").
		Where("issue_id IN ?", issueIDs).
		Group("issue_id").
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		out[row.IssueID] = row.Total
	}
	return out, nil
}
