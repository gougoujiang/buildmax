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
	ID       uint64 `gorm:"primaryKey;autoIncrement"`
	PublicID []byte `gorm:"column:public_id;type:binary(12);uniqueIndex:uq_issue_comment_public_id;not null"`
	IssueID  uint64 `gorm:"column:issue_id;not null;index:idx_issue_comment_issue_created,priority:1"`
	// AuthorID stays an opaque handle under author_kind, and the two source
	// columns record where a comment came from rather than a live relation.
	AuthorKind      string  `gorm:"type:varchar(16);not null"`
	AuthorID        string  `gorm:"type:varchar(64);not null"`
	Body            string  `gorm:"type:text;not null"`
	SourceTaskID    *uint64 `gorm:"column:source_task_id"`
	SourceTaskRunID *uint64 `gorm:"column:source_task_run_id"`
	CreatedAt       int64   `gorm:"autoCreateTime;index:idx_issue_comment_issue_created,priority:2"`
	EditedAt        *int64  `gorm:"column:edited_at"`
}

func (issueCommentRow) TableName() string { return "issue_comment" }

// issueCommentReadRow is the row plus the handles its references resolve to.
type issueCommentReadRow struct {
	Row                issueCommentRow `gorm:"embedded"`
	IssuePublicID      []byte          `gorm:"column:issue_public_id"`
	SourceTaskPublicID []byte          `gorm:"column:source_task_public_id"`
	SourceRunPublicID  []byte          `gorm:"column:source_run_public_id"`
}

func (s *Store) issueCommentSelect(ctx context.Context) *gorm.DB {
	return s.db.WithContext(ctx).Model(&issueCommentRow{}).
		Select("issue_comment.*, i.public_id AS issue_public_id, st.public_id AS source_task_public_id, " +
			"sr.public_id AS source_run_public_id").
		Joins("INNER JOIN issue i ON i.id = issue_comment.issue_id").
		Joins("LEFT JOIN task st ON st.id = issue_comment.source_task_id").
		Joins("LEFT JOIN task_run sr ON sr.id = issue_comment.source_task_run_id")
}

func toIssueComment(row *issueCommentReadRow) *model.IssueComment {
	if row == nil {
		return nil
	}
	out := &model.IssueComment{
		ID:         util.FormatPublicID(row.Row.PublicID),
		IssueID:    util.FormatPublicID(row.IssuePublicID),
		AuthorKind: row.Row.AuthorKind,
		AuthorID:   row.Row.AuthorID,
		Body:       row.Row.Body,
		CreatedAt:  row.Row.CreatedAt,
		EditedAt:   row.Row.EditedAt,
	}
	if row.Row.SourceTaskID != nil {
		task := util.FormatPublicID(row.SourceTaskPublicID)
		out.SourceTaskID = &task
	}
	if row.Row.SourceTaskRunID != nil {
		run := util.FormatPublicID(row.SourceRunPublicID)
		out.SourceTaskRunID = &run
	}
	return out
}

func toIssueComments(rows []issueCommentReadRow) []model.IssueComment {
	out := make([]model.IssueComment, len(rows))
	for i := range rows {
		out[i] = *toIssueComment(&rows[i])
	}
	return out
}

// CreateIssueComment appends a comment to an issue.
func (s *Store) CreateIssueComment(ctx context.Context, in model.CreateIssueCommentInput) (*model.IssueComment, error) {
	issueKey, err := lookupKey(ctx, s.db, "issue", in.IssueID)
	if err != nil {
		return nil, err
	}
	row := &issueCommentRow{
		IssueID:    issueKey,
		AuthorKind: in.AuthorKind,
		AuthorID:   in.AuthorID,
		Body:       in.Body,
		CreatedAt:  time.Now().Unix(),
	}
	read := &issueCommentReadRow{IssuePublicID: mustParsePublicID(in.IssueID)}
	if in.SourceTaskID != nil && *in.SourceTaskID != "" {
		key, err := lookupKey(ctx, s.db, "task", *in.SourceTaskID)
		if err != nil {
			return nil, err
		}
		row.SourceTaskID = &key
		read.SourceTaskPublicID = mustParsePublicID(*in.SourceTaskID)
	}
	if in.SourceTaskRunID != nil && *in.SourceTaskRunID != "" {
		key, err := lookupKey(ctx, s.db, "task_run", *in.SourceTaskRunID)
		if err != nil {
			return nil, err
		}
		row.SourceTaskRunID = &key
		read.SourceRunPublicID = mustParsePublicID(*in.SourceTaskRunID)
	}
	if err := createWithPublicID(ctx, s.db, "uq_issue_comment_public_id",
		func(b []byte) { row.PublicID = b }, row); err != nil {
		return nil, err
	}
	read.Row = *row
	return toIssueComment(read), nil
}

// ListIssueComments returns an issue's comments oldest first.
//
// Ordering is created_at then the row key. A public ID is random rather than
// time-ordered, so it is never a sort key.
func (s *Store) ListIssueComments(ctx context.Context, issueID string, limit, offset int) ([]model.IssueComment, int, error) {
	limit, offset = capPage(limit, offset)
	issueKey, err := lookupKey(ctx, s.db, "issue", issueID)
	if errors.Is(err, model.ErrNotFound) {
		return nil, 0, nil
	}
	if err != nil {
		return nil, 0, err
	}
	var total int64
	if err := s.db.WithContext(ctx).Model(&issueCommentRow{}).Where("issue_id = ?", issueKey).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var list []issueCommentReadRow
	q := s.issueCommentSelect(ctx).Where("issue_comment.issue_id = ?", issueKey).
		Order("issue_comment.created_at ASC, issue_comment.id ASC")
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
	raw, ok := util.ParsePublicID(commentID)
	if !ok {
		return nil, nil
	}
	var row issueCommentReadRow
	err := s.issueCommentSelect(ctx).Where("issue_comment.public_id = ?", raw).Take(&row).Error
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
	raw, ok := util.ParsePublicID(commentID)
	if !ok {
		return nil, nil
	}
	if err := s.db.WithContext(ctx).Model(&issueCommentRow{}).Where("public_id = ?", raw).Updates(updates).Error; err != nil {
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
	raw, ok := util.ParsePublicID(commentID)
	if !ok {
		return nil
	}
	return s.db.WithContext(ctx).Where("public_id = ?", raw).Delete(&issueCommentRow{}).Error
}

// CountIssueComments returns comment totals for the given issues in one grouped
// query. Issues with no comments are absent from the map.
func (s *Store) CountIssueComments(ctx context.Context, issueIDs []string) (map[string]int, error) {
	out := map[string]int{}
	if len(issueIDs) == 0 {
		return out, nil
	}
	raws := make([][]byte, 0, len(issueIDs))
	for _, id := range issueIDs {
		if raw, ok := util.ParsePublicID(id); ok {
			raws = append(raws, raw)
		}
	}
	if len(raws) == 0 {
		return out, nil
	}
	var rows []struct {
		IssuePublicID []byte
		Total         int
	}
	if err := s.db.WithContext(ctx).Model(&issueCommentRow{}).
		Select("i.public_id AS issue_public_id, COUNT(*) AS total").
		Joins("INNER JOIN issue i ON i.id = issue_comment.issue_id").
		Where("i.public_id IN ?", raws).
		Group("i.public_id").
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		out[util.FormatPublicID(row.IssuePublicID)] = row.Total
	}
	return out, nil
}
