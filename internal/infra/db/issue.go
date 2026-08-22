package db

import (
	"context"
	"errors"
	"github.com/gougoujiang/buildmax/internal/core/model"
	"time"

	"github.com/gougoujiang/buildmax/internal/util"
	"gorm.io/gorm"
)

type issueRow struct {
	ID            uint    `gorm:"primaryKey;autoIncrement"`
	IssueID       string  `gorm:"column:issue_id;type:varchar(64);uniqueIndex;not null"`
	UserID        string  `gorm:"type:varchar(64);not null;index"`
	TeamID        string  `gorm:"type:varchar(64);index"`
	ParentIssueID *string `gorm:"column:parent_issue_id;type:varchar(64);index"`
	Title         string  `gorm:"type:varchar(255);not null"`
	Description   string  `gorm:"type:text;not null"`
	Status        string  `gorm:"type:varchar(32);not null"`
	AssigneeKind  *string `gorm:"type:varchar(32)"`
	AssigneeID    *string `gorm:"type:varchar(64)"`
	CreatedBy     string  `gorm:"type:varchar(64);not null"`
	CreatedAt     int64   `gorm:"autoCreateTime"`
	UpdatedAt     int64   `gorm:"autoUpdateTime"`
}

func (issueRow) TableName() string { return "issue" }

func toIssue(row *issueRow) *model.Issue {
	if row == nil {
		return nil
	}
	return &model.Issue{
		ID:            row.IssueID,
		UserID:        row.UserID,
		TeamID:        row.TeamID,
		ParentIssueID: row.ParentIssueID,
		Title:         row.Title,
		Description:   row.Description,
		Status:        row.Status,
		AssigneeKind:  row.AssigneeKind,
		AssigneeID:    row.AssigneeID,
		CreatedBy:     row.CreatedBy,
		CreatedAt:     row.CreatedAt,
		UpdatedAt:     row.UpdatedAt,
	}
}

func toIssues(rows []issueRow) []model.Issue {
	out := make([]model.Issue, len(rows))
	for i := range rows {
		out[i] = *toIssue(&rows[i])
	}
	return out
}

func toIssueRow(m *model.Issue) *issueRow {
	if m == nil {
		return nil
	}
	return &issueRow{
		IssueID:       m.ID,
		UserID:        m.UserID,
		TeamID:        m.TeamID,
		ParentIssueID: m.ParentIssueID,
		Title:         m.Title,
		Description:   m.Description,
		Status:        m.Status,
		AssigneeKind:  m.AssigneeKind,
		AssigneeID:    m.AssigneeID,
		CreatedBy:     m.CreatedBy,
		CreatedAt:     m.CreatedAt,
		UpdatedAt:     m.UpdatedAt,
	}
}

// CreateIssue creates an issue with default status todo. During the transition to
// team ownership, issues created through user-scoped flows are attached to the
// user's default personal team.
func (s *Store) CreateIssue(ctx context.Context, userID string, in model.CreateIssueInput) (*model.Issue, error) {
	teamID, err := s.personalTeamIDForUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	return s.CreateIssueInTeam(ctx, teamID, userID, in)
}

// CreateIssueInTeam creates a team-scoped issue with default status todo.
func (s *Store) CreateIssueInTeam(ctx context.Context, teamID, createdBy string, in model.CreateIssueInput) (*model.Issue, error) {
	now := time.Now().Unix()
	issue := &model.Issue{
		ID:            util.NewPrefixedID(util.PrefixIssue),
		UserID:        createdBy,
		TeamID:        teamID,
		ParentIssueID: in.ParentIssueID,
		Title:         in.Title,
		Description:   in.Description,
		Status:        model.IssueStatusTodo,
		CreatedBy:     createdBy,
		CreatedAt:     now,
		UpdatedAt:     now,
		AssigneeKind:  nil,
		AssigneeID:    nil,
	}
	if err := s.db.WithContext(ctx).Create(toIssueRow(issue)).Error; err != nil {
		return nil, err
	}
	return issue, nil
}

// ListIssuesByUser returns issues for the user ordered by updated_at DESC.
func (s *Store) ListIssuesByUser(ctx context.Context, userID string, limit, offset int) ([]model.Issue, int, error) {
	limit, offset = capPage(limit, offset)
	var total int64
	if err := s.db.WithContext(ctx).Model(&issueRow{}).Where("user_id = ?", userID).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var list []issueRow
	q := s.db.WithContext(ctx).Where("user_id = ?", userID).Order("updated_at DESC")
	if limit > 0 {
		q = q.Limit(limit).Offset(offset)
	}
	if err := q.Find(&list).Error; err != nil {
		return nil, 0, err
	}
	return toIssues(list), int(total), nil
}

// ListIssuesByTeam returns issues for the team ordered by updated_at DESC.
//
// A zero filter lists every issue in the team, sub-issues included, which is
// what callers predating the hierarchy expect.
func (s *Store) ListIssuesByTeam(ctx context.Context, teamID string, filter model.ListIssuesFilter, limit, offset int) ([]model.Issue, int, error) {
	limit, offset = capPage(limit, offset)
	scope := func(q *gorm.DB) *gorm.DB {
		q = q.Where("team_id = ?", teamID)
		switch {
		case filter.TopLevelOnly:
			q = q.Where("parent_issue_id IS NULL")
		case filter.ParentIssueID != "":
			q = q.Where("parent_issue_id = ?", filter.ParentIssueID)
		}
		return q
	}
	var total int64
	if err := scope(s.db.WithContext(ctx).Model(&issueRow{})).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var list []issueRow
	q := scope(s.db.WithContext(ctx)).Order("updated_at DESC")
	if limit > 0 {
		q = q.Limit(limit).Offset(offset)
	}
	if err := q.Find(&list).Error; err != nil {
		return nil, 0, err
	}
	return toIssues(list), int(total), nil
}

// ListIssueChildren returns every sub-issue of parentIssueID, oldest first.
//
// There is no pagination because the hierarchy is two levels deep and a
// parent's children are shown as one group; a parent with enough children to
// need paging is a sign the breakdown wants a different shape, not a bigger
// page.
func (s *Store) ListIssueChildren(ctx context.Context, parentIssueID string) ([]model.Issue, error) {
	if parentIssueID == "" {
		return []model.Issue{}, nil
	}
	var list []issueRow
	if err := s.db.WithContext(ctx).
		Where("parent_issue_id = ?", parentIssueID).
		Order("created_at ASC, id ASC").
		Find(&list).Error; err != nil {
		return nil, err
	}
	return toIssues(list), nil
}

// ChildStatsForIssues returns sub-issue progress for the given parents in one
// grouped query. Parents with no children are absent from the map.
func (s *Store) ChildStatsForIssues(ctx context.Context, issueIDs []string) (map[string]model.IssueChildStats, error) {
	out := map[string]model.IssueChildStats{}
	if len(issueIDs) == 0 {
		return out, nil
	}
	var rows []struct {
		ParentIssueID string
		Total         int
		Done          int
	}
	if err := s.db.WithContext(ctx).Model(&issueRow{}).
		Select("parent_issue_id, COUNT(*) AS total, SUM(CASE WHEN status = ? THEN 1 ELSE 0 END) AS done", model.IssueStatusDone).
		Where("parent_issue_id IN ?", issueIDs).
		Group("parent_issue_id").
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		out[row.ParentIssueID] = model.IssueChildStats{Total: row.Total, Done: row.Done}
	}
	return out, nil
}

// GetIssue returns the issue by issue_id, or (nil, nil) if not found.
func (s *Store) GetIssue(ctx context.Context, issueID string) (*model.Issue, error) {
	var issue issueRow
	err := s.db.WithContext(ctx).Where("issue_id = ?", issueID).First(&issue).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return toIssue(&issue), nil
}

// UpdateIssue updates only provided fields. Returns (nil, nil) if not found or not owned by user.
func (s *Store) UpdateIssue(ctx context.Context, issueID, userID string, in model.UpdateIssueInput) (*model.Issue, error) {
	issue, err := s.GetIssue(ctx, issueID)
	if err != nil || issue == nil {
		return nil, err
	}
	if issue.UserID != userID {
		return nil, nil
	}
	return s.updateIssue(ctx, issueID, in)
}

// UpdateIssueInTeam updates only provided fields. Returns (nil, nil) if not found
// or not owned by the given team.
func (s *Store) UpdateIssueInTeam(ctx context.Context, issueID, teamID string, in model.UpdateIssueInput) (*model.Issue, error) {
	issue, err := s.GetIssue(ctx, issueID)
	if err != nil || issue == nil {
		return nil, err
	}
	if issue.TeamID != teamID {
		return nil, nil
	}
	return s.updateIssue(ctx, issueID, in)
}

func (s *Store) updateIssue(ctx context.Context, issueID string, in model.UpdateIssueInput) (*model.Issue, error) {
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
	if in.ParentIssueID != nil {
		if *in.ParentIssueID == "" {
			updates["parent_issue_id"] = nil
		} else {
			updates["parent_issue_id"] = *in.ParentIssueID
		}
	}
	if err := s.db.WithContext(ctx).Model(&issueRow{}).Where("issue_id = ?", issueID).Updates(updates).Error; err != nil {
		return nil, err
	}
	return s.GetIssue(ctx, issueID)
}
