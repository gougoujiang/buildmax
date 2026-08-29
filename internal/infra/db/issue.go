package db

import (
	"context"
	"errors"
	"time"

	"github.com/gougoujiang/buildmax/internal/core/apierr"
	coreissue "github.com/gougoujiang/buildmax/internal/core/issue"

	"github.com/gougoujiang/buildmax/internal/util"
	"gorm.io/gorm"
)

type issueRow struct {
	ID            uint64  `gorm:"primaryKey;autoIncrement"`
	PublicID      string  `gorm:"column:public_id;type:char(20) CHARACTER SET ascii COLLATE ascii_bin;uniqueIndex:uq_issue_public_id;not null"`
	UserID        uint64  `gorm:"column:user_id;not null;index"`
	TeamID        uint64  `gorm:"column:team_id;index:idx_issue_team_updated,priority:1"`
	ParentIssueID *uint64 `gorm:"column:parent_issue_id;index"`
	Title         string  `gorm:"type:varchar(255);not null"`
	Description   string  `gorm:"type:text;not null"`
	Status        string  `gorm:"type:varchar(32);not null"`
	// AssigneeID stays an opaque handle: assignee_kind admits person, agent, or
	// workflow, and one numeric column cannot name rows in three tables.
	AssigneeKind *string `gorm:"type:varchar(32)"`
	AssigneeID   *string `gorm:"type:varchar(64)"`
	CreatedBy    uint64  `gorm:"column:created_by;not null"`
	// Version is the optimistic-concurrency token. Every accepted update carries
	// the version it was built from and bumps it, so two writers racing on one
	// issue produce one winner and one refusal instead of a silent overwrite.
	Version   uint64    `gorm:"column:version;not null;default:1"`
	CreatedAt time.Time `gorm:"autoCreateTime"`
	UpdatedAt time.Time `gorm:"autoUpdateTime;index:idx_issue_team_updated,priority:2"`
}

func (issueRow) TableName() string { return "issue" }

// issueReadRow is the row plus the handles its references resolve to. A
// pointer field is one a LEFT JOIN may leave NULL.
type issueReadRow struct {
	Row               issueRow `gorm:"embedded"`
	UserPublicID      string   `gorm:"column:user_public_id"`
	TeamPublicID      *string  `gorm:"column:team_public_id"`
	ParentPublicID    *string  `gorm:"column:parent_public_id"`
	CreatedByPublicID string   `gorm:"column:created_by_public_id"`
}

func (s *Store) issueSelect(ctx context.Context) *gorm.DB {
	return issueSelectTx(s.db.WithContext(ctx))
}

func issueSelectTx(tx *gorm.DB) *gorm.DB {
	return tx.Model(&issueRow{}).
		Select("issue.*, u.public_id AS user_public_id, t.public_id AS team_public_id, " +
			"p.public_id AS parent_public_id, cb.public_id AS created_by_public_id").
		Joins("INNER JOIN `user` u ON u.id = issue.user_id").
		Joins("LEFT JOIN team t ON t.id = issue.team_id").
		Joins("LEFT JOIN issue p ON p.id = issue.parent_issue_id").
		Joins("INNER JOIN `user` cb ON cb.id = issue.created_by")
}

func toIssue(row *issueReadRow) *coreissue.Issue {
	if row == nil {
		return nil
	}
	out := &coreissue.Issue{
		ID:           row.Row.PublicID,
		UserID:       row.UserPublicID,
		TeamID:       derefPublicID(row.TeamPublicID),
		Title:        row.Row.Title,
		Description:  row.Row.Description,
		Status:       row.Row.Status,
		AssigneeKind: row.Row.AssigneeKind,
		AssigneeID:   row.Row.AssigneeID,
		CreatedBy:    row.CreatedByPublicID,
		CreatedAt:    row.Row.CreatedAt,
		UpdatedAt:    row.Row.UpdatedAt,
		Version:      row.Row.Version,
	}
	if row.Row.ParentIssueID != nil {
		parent := derefPublicID(row.ParentPublicID)
		out.ParentIssueID = &parent
	}
	return out
}

func toIssues(rows []issueReadRow) []coreissue.Issue {
	out := make([]coreissue.Issue, len(rows))
	for i := range rows {
		out[i] = *toIssue(&rows[i])
	}
	return out
}

// CreateIssue creates an issue with default status todo. During the transition to
// team ownership, issues created through user-scoped flows are attached to the
// user's default personal team.
func (s *Store) CreateIssue(ctx context.Context, userID string, in coreissue.CreateInput) (*coreissue.Issue, error) {
	teamID, err := s.personalTeamIDForUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	return s.CreateIssueInTeam(ctx, teamID, userID, in)
}

// CreateIssueInTeam creates a team-scoped issue with default status todo.
func (s *Store) CreateIssueInTeam(ctx context.Context, teamID, createdBy string, in coreissue.CreateInput) (*coreissue.Issue, error) {
	now := time.Now().UTC()
	row := &issueRow{
		Title:       in.Title,
		Description: in.Description,
		Status:      coreissue.StatusTodo,
		Version:     1,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		creator, err := lookupKey(ctx, tx, "user", createdBy)
		if err != nil {
			return err
		}
		row.UserID = creator
		row.CreatedBy = creator
		if teamID != "" {
			teamKey, err := lookupKey(ctx, tx, "team", teamID)
			if err != nil {
				return err
			}
			row.TeamID = teamKey
		}
		if in.ParentIssueID != nil && *in.ParentIssueID != "" {
			parent, err := lookupKey(ctx, tx, "issue", *in.ParentIssueID)
			if err != nil {
				return err
			}
			row.ParentIssueID = &parent
		}
		return createWithPublicID(ctx, tx, "uq_issue_public_id",
			func(id string) { row.PublicID = id }, row)
	}); err != nil {
		return nil, err
	}
	return &coreissue.Issue{
		ID:            row.PublicID,
		UserID:        createdBy,
		TeamID:        teamID,
		ParentIssueID: in.ParentIssueID,
		Title:         in.Title,
		Description:   in.Description,
		Status:        coreissue.StatusTodo,
		CreatedBy:     createdBy,
		CreatedAt:     now,
		UpdatedAt:     now,
	}, nil
}

// ListIssuesByUser returns issues for the user ordered by updated_at DESC.
func (s *Store) ListIssuesByUser(ctx context.Context, userID string, limit, offset int) ([]coreissue.Issue, int, error) {
	limit, offset = capPage(limit, offset)
	userKey, err := lookupKey(ctx, s.db, "user", userID)
	if errors.Is(err, apierr.ErrNotFound) {
		return nil, 0, nil
	}
	if err != nil {
		return nil, 0, err
	}
	var total int64
	if err := s.db.WithContext(ctx).Model(&issueRow{}).Where("user_id = ?", userKey).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var list []issueReadRow
	q := s.issueSelect(ctx).Where("issue.user_id = ?", userKey).Order("issue.updated_at DESC")
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
func (s *Store) ListIssuesByTeam(ctx context.Context, teamID string, filter coreissue.ListFilter, limit, offset int) ([]coreissue.Issue, int, error) {
	limit, offset = capPage(limit, offset)
	teamKey, err := lookupKey(ctx, s.db, "team", teamID)
	if errors.Is(err, apierr.ErrNotFound) {
		return nil, 0, nil
	}
	if err != nil {
		return nil, 0, err
	}
	var parentKey *uint64
	if filter.ParentIssueID != "" {
		key, err := lookupKey(ctx, s.db, "issue", filter.ParentIssueID)
		if errors.Is(err, apierr.ErrNotFound) {
			return nil, 0, nil
		}
		if err != nil {
			return nil, 0, err
		}
		parentKey = &key
	}
	scope := func(q *gorm.DB, col string) *gorm.DB {
		q = q.Where(col+"team_id = ?", teamKey)
		switch {
		case filter.TopLevelOnly:
			q = q.Where(col + "parent_issue_id IS NULL")
		case parentKey != nil:
			q = q.Where(col+"parent_issue_id = ?", *parentKey)
		}
		if filter.AssigneeKind != "" && filter.AssigneeID != "" {
			q = q.Where(col+"assignee_kind = ? AND "+col+"assignee_id = ?", filter.AssigneeKind, filter.AssigneeID)
		}
		if filter.Status != "" {
			q = q.Where(col+"status = ?", filter.Status)
		}
		return q
	}
	var total int64
	if err := scope(s.db.WithContext(ctx).Model(&issueRow{}), "").Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var list []issueReadRow
	q := scope(s.issueSelect(ctx), "issue.").Order("issue.updated_at DESC")
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
func (s *Store) ListIssueChildren(ctx context.Context, parentIssueID string) ([]coreissue.Issue, error) {
	if parentIssueID == "" {
		return []coreissue.Issue{}, nil
	}
	parentKey, err := lookupKey(ctx, s.db, "issue", parentIssueID)
	if errors.Is(err, apierr.ErrNotFound) {
		return []coreissue.Issue{}, nil
	}
	if err != nil {
		return nil, err
	}
	var list []issueReadRow
	if err := s.issueSelect(ctx).
		Where("issue.parent_issue_id = ?", parentKey).
		Order("issue.created_at ASC, issue.id ASC").
		Find(&list).Error; err != nil {
		return nil, err
	}
	return toIssues(list), nil
}

// ChildStatsForIssues returns sub-issue progress for the given parents in one
// grouped query. Parents with no children are absent from the map.
func (s *Store) ChildStatsForIssues(ctx context.Context, issueIDs []string) (map[string]coreissue.ChildStats, error) {
	out := map[string]coreissue.ChildStats{}
	if len(issueIDs) == 0 {
		return out, nil
	}
	ids := make([]string, 0, len(issueIDs))
	for _, handle := range issueIDs {
		if id, ok := util.CanonicalPublicID(handle); ok {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return out, nil
	}
	// The parent's handle comes back through the join; the grouping is on its
	// row key, so one query still answers for every parent asked about.
	var rows []struct {
		ParentPublicID string
		Total          int
		Done           int
	}
	if err := s.db.WithContext(ctx).Model(&issueRow{}).
		Select("p.public_id AS parent_public_id, COUNT(*) AS total, SUM(CASE WHEN issue.status = ? THEN 1 ELSE 0 END) AS done", coreissue.StatusDone).
		Joins("INNER JOIN issue p ON p.id = issue.parent_issue_id").
		Where("p.public_id IN ?", ids).
		Group("p.public_id").
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		out[row.ParentPublicID] = coreissue.ChildStats{Total: row.Total, Done: row.Done}
	}
	return out, nil
}

// GetIssue returns the issue by issue_id, or (nil, nil) if not found.
func (s *Store) GetIssue(ctx context.Context, issueID string) (*coreissue.Issue, error) {
	id, ok := util.CanonicalPublicID(issueID)
	if !ok {
		return nil, nil
	}
	var issue issueReadRow
	err := s.issueSelect(ctx).Where("issue.public_id = ?", id).Take(&issue).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return toIssue(&issue), nil
}

// UpdateIssue updates only provided fields. Returns (nil, nil) if not found or not owned by user.
func (s *Store) UpdateIssue(ctx context.Context, issueID, userID string, in coreissue.UpdateInput) (*coreissue.Issue, error) {
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
func (s *Store) UpdateIssueInTeam(ctx context.Context, issueID, teamID string, in coreissue.UpdateInput) (*coreissue.Issue, error) {
	issue, err := s.GetIssue(ctx, issueID)
	if err != nil || issue == nil {
		return nil, err
	}
	if issue.TeamID != teamID {
		return nil, nil
	}
	return s.updateIssue(ctx, issueID, in)
}

func (s *Store) updateIssue(ctx context.Context, issueID string, in coreissue.UpdateInput) (*coreissue.Issue, error) {
	updates := map[string]interface{}{
		"updated_at": time.Now().UTC(),
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
			parent, err := lookupKey(ctx, s.db, "issue", *in.ParentIssueID)
			if err != nil {
				return nil, err
			}
			updates["parent_issue_id"] = parent
		}
	}
	id, ok := util.CanonicalPublicID(issueID)
	if !ok {
		return nil, nil
	}
	// The version guard is in the WHERE clause, not a read-then-check: only the
	// database can decide the race, and a check in this process would leave the
	// window it is supposed to close.
	updates["version"] = gorm.Expr("version + 1")
	res := s.db.WithContext(ctx).Model(&issueRow{}).
		Where("public_id = ? AND version = ?", id, in.IfVersion).
		Updates(updates)
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected == 0 {
		// Nothing matched. Re-read to say which of the two it was, so a caller
		// does not report a vanished issue as a stale one.
		current, err := s.GetIssue(ctx, issueID)
		if err != nil {
			return nil, err
		}
		if current == nil {
			return nil, nil
		}
		return nil, coreissue.ErrVersionConflict
	}
	return s.GetIssue(ctx, issueID)
}
