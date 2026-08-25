package db

import (
	"context"
	"errors"
	"time"

	"github.com/gougoujiang/buildmax/internal/core/apierr"
	"github.com/gougoujiang/buildmax/internal/core/model"

	"github.com/gougoujiang/buildmax/internal/util"
	"gorm.io/gorm"
)

type teamRow struct {
	ID                uint64  `gorm:"primaryKey;autoIncrement"`
	PublicID          string  `gorm:"column:public_id;type:char(20) CHARACTER SET ascii COLLATE ascii_bin;uniqueIndex:uq_team_public_id;not null"`
	Name              string  `gorm:"type:varchar(255);not null"`
	PersonalForUserID *uint64 `gorm:"column:personal_for_user_id;uniqueIndex"`
	QuotaTier         string  `gorm:"column:quota_tier;type:varchar(64)"`
	// PluginCuration is who fills the team's plugin activation list. It
	// defaults to open: the gate that crosses teams is operator eligibility,
	// not a team's housekeeping. See docs/design/plugin-team-distribution.md.
	PluginCuration string    `gorm:"column:plugin_curation;type:varchar(16);not null;default:'open'"`
	CreatedBy      uint64    `gorm:"column:created_by;not null"`
	CreatedAt      time.Time `gorm:"autoCreateTime"`
	UpdatedAt      time.Time `gorm:"autoUpdateTime"`
}

func (teamRow) TableName() string { return "team" }

// teamReadRow is teamRow plus the handles its user references resolve to.
//
// Every read that returns a team joins for them, so a caller never sees a row
// key and no listing turns into one query per row. teamSelect is the one place
// the join set is written down.
//
// The row is a named field, not an anonymous one. GORM reads an anonymous
// embedded struct that has its own TableName as an association and scans none
// of its columns, which produces a zero-valued row rather than an error. Every
// read struct in this package follows this shape for that reason.
//
// A pointer field is one a LEFT JOIN may leave NULL.
type teamReadRow struct {
	Row                     teamRow `gorm:"embedded"`
	PersonalForUserPublicID *string `gorm:"column:personal_for_user_public_id"`
	CreatedByPublicID       *string `gorm:"column:created_by_public_id"`
}

type teamMemberRow struct {
	ID        uint64    `gorm:"primaryKey;autoIncrement"`
	TeamID    uint64    `gorm:"column:team_id;not null;uniqueIndex:uq_team_member_team_user"`
	UserID    uint64    `gorm:"column:user_id;not null;uniqueIndex:uq_team_member_team_user"`
	Role      string    `gorm:"type:varchar(32);not null"`
	CreatedAt time.Time `gorm:"autoCreateTime"`
}

func (teamMemberRow) TableName() string { return "team_member" }

// teamMemberReadRow is teamMemberRow plus the handles its two references
// resolve to. A membership has no handle of its own.
type teamMemberReadRow struct {
	Row          teamMemberRow `gorm:"embedded"`
	TeamPublicID string        `gorm:"column:team_public_id"`
	UserPublicID string        `gorm:"column:user_public_id"`
}

// teamSelect is the read shape for a team: the row plus the public handles of
// the users it names.
func (s *Store) teamSelect(ctx context.Context) *gorm.DB {
	return s.db.WithContext(ctx).Model(&teamRow{}).
		Select("team.*, pu.public_id AS personal_for_user_public_id, cb.public_id AS created_by_public_id").
		Joins("LEFT JOIN `user` pu ON pu.id = team.personal_for_user_id").
		Joins("LEFT JOIN `user` cb ON cb.id = team.created_by")
}

// teamMemberSelect is the read shape for a membership.
func (s *Store) teamMemberSelect(ctx context.Context) *gorm.DB {
	return s.db.WithContext(ctx).Model(&teamMemberRow{}).
		Select("team_member.*, t.public_id AS team_public_id, u.public_id AS user_public_id").
		Joins("INNER JOIN team t ON t.id = team_member.team_id").
		Joins("INNER JOIN `user` u ON u.id = team_member.user_id")
}

func toTeam(row *teamReadRow) *model.Team {
	if row == nil {
		return nil
	}
	out := &model.Team{
		ID:             row.Row.PublicID,
		Name:           row.Row.Name,
		QuotaTier:      row.Row.QuotaTier,
		PluginCuration: model.NormalizePluginCuration(row.Row.PluginCuration),
		CreatedBy:      derefPublicID(row.CreatedByPublicID),
		CreatedAt:      row.Row.CreatedAt,
		UpdatedAt:      row.Row.UpdatedAt,
	}
	if row.Row.PersonalForUserID != nil {
		personal := derefPublicID(row.PersonalForUserPublicID)
		out.PersonalForUserID = &personal
	}
	return out
}

func toTeams(rows []teamReadRow) []model.Team {
	out := make([]model.Team, len(rows))
	for i := range rows {
		out[i] = *toTeam(&rows[i])
	}
	return out
}

func toTeamMember(row *teamMemberReadRow) *model.TeamMember {
	if row == nil {
		return nil
	}
	return &model.TeamMember{
		TeamID:    row.TeamPublicID,
		UserID:    row.UserPublicID,
		Role:      row.Row.Role,
		CreatedAt: row.Row.CreatedAt,
	}
}

func toTeamMembers(rows []teamMemberReadRow) []model.TeamMember {
	out := make([]model.TeamMember, len(rows))
	for i := range rows {
		out[i] = *toTeamMember(&rows[i])
	}
	return out
}

// GetTeam returns the team by team_id, or (nil, nil) when not found.
func (s *Store) GetTeam(ctx context.Context, teamID string) (*model.Team, error) {
	id, ok := util.CanonicalPublicID(teamID)
	if !ok {
		return nil, nil
	}
	var team teamReadRow
	err := s.teamSelect(ctx).Where("team.public_id = ?", id).Take(&team).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return toTeam(&team), nil
}

// GetPersonalTeamByUser returns the default personal team for the user, or (nil, nil) when not found.
func (s *Store) GetPersonalTeamByUser(ctx context.Context, userID string) (*model.Team, error) {
	id, ok := util.CanonicalPublicID(userID)
	if !ok {
		return nil, nil
	}
	var team teamReadRow
	err := s.teamSelect(ctx).Where("pu.public_id = ?", id).Take(&team).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return toTeam(&team), nil
}

// ListTeamsByUser returns all teams the user belongs to, ordered by created_at ASC.
func (s *Store) ListTeamsByUser(ctx context.Context, userID string) ([]model.Team, error) {
	id, ok := util.CanonicalPublicID(userID)
	if !ok {
		return nil, nil
	}
	var list []teamReadRow
	err := s.teamSelect(ctx).
		Joins("INNER JOIN team_member ON team_member.team_id = team.id").
		Joins("INNER JOIN `user` mu ON mu.id = team_member.user_id").
		Where("mu.public_id = ?", id).
		Order("team.created_at ASC").
		Find(&list).Error
	return toTeams(list), err
}

// CreateTeam creates a new team and owner membership.
func (s *Store) CreateTeam(ctx context.Context, name, createdBy, quotaTier string) (*model.Team, error) {
	now := time.Now().UTC()
	teamDB := &teamRow{Name: name, QuotaTier: quotaTier, CreatedAt: now, UpdatedAt: now}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		creator, err := lookupKey(ctx, tx, "user", createdBy)
		if err != nil {
			return err
		}
		teamDB.CreatedBy = creator
		if err := createWithPublicID(ctx, tx, "uq_team_public_id",
			func(id string) { teamDB.PublicID = id }, teamDB); err != nil {
			return err
		}
		return tx.Create(&teamMemberRow{
			TeamID:    teamDB.ID,
			UserID:    creator,
			Role:      model.TeamRoleOwner,
			CreatedAt: now,
		}).Error
	})
	if err != nil {
		return nil, err
	}
	return &model.Team{
		ID:        teamDB.PublicID,
		Name:      name,
		QuotaTier: quotaTier,
		CreatedBy: createdBy,
		CreatedAt: now,
		UpdatedAt: now,
	}, nil
}

// AddTeamMember adds or updates a team membership.
func (s *Store) AddTeamMember(ctx context.Context, teamID, userID, role string) (*model.TeamMember, error) {
	member := &model.TeamMember{
		TeamID:    teamID,
		UserID:    userID,
		Role:      role,
		CreatedAt: time.Now().UTC(),
	}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		teamKey, err := lookupKey(ctx, tx, "team", teamID)
		if err != nil {
			return err
		}
		userKey, err := lookupKey(ctx, tx, "user", userID)
		if err != nil {
			return err
		}
		var existing teamMemberRow
		findErr := tx.Where("team_id = ? AND user_id = ?", teamKey, userKey).First(&existing).Error
		switch {
		case errors.Is(findErr, gorm.ErrRecordNotFound):
			return tx.Create(&teamMemberRow{
				TeamID:    teamKey,
				UserID:    userKey,
				Role:      role,
				CreatedAt: member.CreatedAt,
			}).Error
		case findErr != nil:
			return findErr
		default:
			existing.Role = role
			member.CreatedAt = existing.CreatedAt
			return tx.Save(&existing).Error
		}
	})
	if err != nil {
		return nil, err
	}
	return member, nil
}

// RemoveTeamMember removes a team membership when present.
func (s *Store) RemoveTeamMember(ctx context.Context, teamID, userID string) error {
	teamKey, err := lookupKey(ctx, s.db, "team", teamID)
	if errors.Is(err, apierr.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	userKey, err := lookupKey(ctx, s.db, "user", userID)
	if errors.Is(err, apierr.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	return s.db.WithContext(ctx).
		Where("team_id = ? AND user_id = ?", teamKey, userKey).
		Delete(&teamMemberRow{}).Error
}

// ListTeamMembers returns members of the team ordered by created_at ASC.
func (s *Store) ListTeamMembers(ctx context.Context, teamID string) ([]model.TeamMember, error) {
	id, ok := util.CanonicalPublicID(teamID)
	if !ok {
		return nil, nil
	}
	var list []teamMemberReadRow
	err := s.teamMemberSelect(ctx).
		Where("t.public_id = ?", id).
		Order("team_member.created_at ASC").
		Find(&list).Error
	return toTeamMembers(list), err
}

func (s *Store) personalTeamIDForUser(ctx context.Context, userID string) (string, error) {
	team, err := s.GetPersonalTeamByUser(ctx, userID)
	if err != nil {
		return "", err
	}
	if team == nil {
		return "", nil
	}
	return team.ID, nil
}

// ListAllTeams implements model.TeamStore.
func (s *Store) ListAllTeams(ctx context.Context, query string, limit, offset int) ([]model.Team, int, error) {
	limit, offset = clampPage(limit, offset)
	var total int64
	countQ := s.db.WithContext(ctx).Model(&teamRow{})
	if query != "" {
		countQ = countQ.Where("name LIKE ?", "%"+query+"%")
	}
	if err := countQ.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	q := s.teamSelect(ctx)
	if query != "" {
		q = q.Where("team.name LIKE ?", "%"+query+"%")
	}
	var rows []teamReadRow
	if err := q.Order("team.created_at DESC, team.id DESC").Limit(limit).Offset(offset).Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	out := make([]model.Team, 0, len(rows))
	for i := range rows {
		out = append(out, *toTeam(&rows[i]))
	}
	return out, int(total), nil
}

// CountTeamMembers implements model.TeamStore.
func (s *Store) CountTeamMembers(ctx context.Context, teamIDs []string) (map[string]int, error) {
	out := make(map[string]int, len(teamIDs))
	if len(teamIDs) == 0 {
		return out, nil
	}
	// Counting groups by the row key, so the handles the caller asked about are
	// carried through the join rather than resolved one at a time.
	ids := make([]string, 0, len(teamIDs))
	for _, teamID := range teamIDs {
		if id, ok := util.CanonicalPublicID(teamID); ok {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return out, nil
	}
	var rows []struct {
		PublicID string
		N        int
	}
	if err := s.db.WithContext(ctx).
		Model(&teamMemberRow{}).
		Select("t.public_id AS public_id, count(*) AS n").
		Joins("INNER JOIN team t ON t.id = team_member.team_id").
		Where("t.public_id IN ?", ids).
		Group("t.public_id").
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		out[row.PublicID] = row.N
	}
	return out, nil
}

// SetTeamPluginCuration records who fills the team's plugin activation list.
//
// The value is validated above this layer, which is why an unrecognized one
// reaches the column rather than being rejected here: this package translates,
// it does not decide what a mode means.
func (s *Store) SetTeamPluginCuration(ctx context.Context, teamID string, mode model.PluginCuration) error {
	key, err := lookupKey(ctx, s.db, "team", teamID)
	if err != nil {
		return err
	}
	// Zero rows affected is the mode already having that value: the team
	// resolved a moment ago, so it is not a missing row.
	return s.db.WithContext(ctx).Model(&teamRow{}).Where("id = ?", key).
		Update("plugin_curation", string(mode)).Error
}
