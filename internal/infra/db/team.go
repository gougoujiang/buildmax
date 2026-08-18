package db

import (
	"context"
	"errors"
	"github.com/gougoujiang/buildmax/internal/core/model"
	"time"

	"github.com/gougoujiang/buildmax/internal/util"
	"gorm.io/gorm"
)

type teamRow struct {
	ID                uint    `gorm:"primaryKey;autoIncrement"`
	TeamID            string  `gorm:"column:team_id;type:varchar(64);uniqueIndex;not null"`
	Name              string  `gorm:"type:varchar(255);not null"`
	PersonalForUserID *string `gorm:"column:personal_for_user_id;type:varchar(64);uniqueIndex"`
	QuotaTier         string  `gorm:"column:quota_tier;type:varchar(64)"`
	CreatedBy         string  `gorm:"type:varchar(64);not null"`
	CreatedAt         int64   `gorm:"autoCreateTime"`
	UpdatedAt         int64   `gorm:"autoUpdateTime"`
}

func (teamRow) TableName() string { return "team" }

type teamMemberRow struct {
	ID        uint   `gorm:"primaryKey;autoIncrement"`
	TeamID    string `gorm:"column:team_id;type:varchar(64);not null;uniqueIndex:uq_team_member_team_user"`
	UserID    string `gorm:"column:user_id;type:varchar(64);not null;uniqueIndex:uq_team_member_team_user"`
	Role      string `gorm:"type:varchar(32);not null"`
	CreatedAt int64  `gorm:"autoCreateTime"`
}

func (teamMemberRow) TableName() string { return "team_member" }

func toTeam(row *teamRow) *model.Team {
	if row == nil {
		return nil
	}
	return &model.Team{
		ID:                row.ID,
		TeamID:            row.TeamID,
		Name:              row.Name,
		PersonalForUserID: row.PersonalForUserID,
		QuotaTier:         row.QuotaTier,
		CreatedBy:         row.CreatedBy,
		CreatedAt:         row.CreatedAt,
		UpdatedAt:         row.UpdatedAt,
	}
}

func toTeams(rows []teamRow) []model.Team {
	out := make([]model.Team, len(rows))
	for i := range rows {
		out[i] = *toTeam(&rows[i])
	}
	return out
}

func toTeamRow(m *model.Team) *teamRow {
	if m == nil {
		return nil
	}
	return &teamRow{
		ID:                m.ID,
		TeamID:            m.TeamID,
		Name:              m.Name,
		PersonalForUserID: m.PersonalForUserID,
		QuotaTier:         m.QuotaTier,
		CreatedBy:         m.CreatedBy,
		CreatedAt:         m.CreatedAt,
		UpdatedAt:         m.UpdatedAt,
	}
}

func toTeamMember(row *teamMemberRow) *model.TeamMember {
	if row == nil {
		return nil
	}
	return &model.TeamMember{
		ID:        row.ID,
		TeamID:    row.TeamID,
		UserID:    row.UserID,
		Role:      row.Role,
		CreatedAt: row.CreatedAt,
	}
}

func toTeamMembers(rows []teamMemberRow) []model.TeamMember {
	out := make([]model.TeamMember, len(rows))
	for i := range rows {
		out[i] = *toTeamMember(&rows[i])
	}
	return out
}

func toTeamMemberRow(m *model.TeamMember) *teamMemberRow {
	if m == nil {
		return nil
	}
	return &teamMemberRow{
		ID:        m.ID,
		TeamID:    m.TeamID,
		UserID:    m.UserID,
		Role:      m.Role,
		CreatedAt: m.CreatedAt,
	}
}

// GetTeam returns the team by team_id, or (nil, nil) when not found.
func (s *Store) GetTeam(ctx context.Context, teamID string) (*model.Team, error) {
	var team teamRow
	err := s.db.WithContext(ctx).Where("team_id = ?", teamID).First(&team).Error
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
	var team teamRow
	err := s.db.WithContext(ctx).Where("personal_for_user_id = ?", userID).First(&team).Error
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
	var list []teamRow
	err := s.db.WithContext(ctx).
		Table("team").
		Select("team.*").
		Joins("INNER JOIN team_member ON team_member.team_id = team.team_id").
		Where("team_member.user_id = ?", userID).
		Order("team.created_at ASC").
		Find(&list).Error
	return toTeams(list), err
}

// CreateTeam creates a new team and owner membership.
func (s *Store) CreateTeam(ctx context.Context, name, createdBy, quotaTier string) (*model.Team, error) {
	now := time.Now().Unix()
	team := &model.Team{
		TeamID:    util.NewPrefixedID(util.PrefixTeam),
		Name:      name,
		QuotaTier: quotaTier,
		CreatedBy: createdBy,
		CreatedAt: now,
		UpdatedAt: now,
	}
	member := &model.TeamMember{
		TeamID:    team.TeamID,
		UserID:    createdBy,
		Role:      model.TeamRoleOwner,
		CreatedAt: now,
	}
	teamDB := toTeamRow(team)
	memberDB := toTeamMemberRow(member)
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(teamDB).Error; err != nil {
			return err
		}
		return tx.Create(memberDB).Error
	})
	if err != nil {
		return nil, err
	}
	return team, nil
}

// AddTeamMember adds or updates a team membership.
func (s *Store) AddTeamMember(ctx context.Context, teamID, userID, role string) (*model.TeamMember, error) {
	member := &model.TeamMember{
		TeamID:    teamID,
		UserID:    userID,
		Role:      role,
		CreatedAt: time.Now().Unix(),
	}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var existing teamMemberRow
		findErr := tx.Where("team_id = ? AND user_id = ?", teamID, userID).First(&existing).Error
		switch {
		case errors.Is(findErr, gorm.ErrRecordNotFound):
			return tx.Create(toTeamMemberRow(member)).Error
		case findErr != nil:
			return findErr
		default:
			existing.Role = role
			member = toTeamMember(&existing)
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
	return s.db.WithContext(ctx).
		Where("team_id = ? AND user_id = ?", teamID, userID).
		Delete(&teamMemberRow{}).Error
}

// ListTeamMembers returns members of the team ordered by created_at ASC.
func (s *Store) ListTeamMembers(ctx context.Context, teamID string) ([]model.TeamMember, error) {
	var list []teamMemberRow
	err := s.db.WithContext(ctx).
		Where("team_id = ?", teamID).
		Order("created_at ASC").
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
	return team.TeamID, nil
}

// ListAllTeams implements model.TeamStore.
func (s *Store) ListAllTeams(ctx context.Context, query string, limit, offset int) ([]model.Team, int, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	if offset < 0 {
		offset = 0
	}
	q := s.db.WithContext(ctx).Model(&teamRow{})
	if query != "" {
		q = q.Where("name LIKE ?", "%"+query+"%")
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []teamRow
	if err := q.Order("created_at DESC, id DESC").Limit(limit).Offset(offset).Find(&rows).Error; err != nil {
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
	var rows []struct {
		TeamID string
		N      int
	}
	if err := s.db.WithContext(ctx).
		Model(&teamMemberRow{}).
		Select("team_id, count(*) as n").
		Where("team_id IN ?", teamIDs).
		Group("team_id").
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		out[row.TeamID] = row.N
	}
	return out, nil
}
