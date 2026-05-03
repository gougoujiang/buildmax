package db

import (
	"buildmax/internal/core/model"
	"context"
	"errors"
	"time"

	"buildmax/internal/util"
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

func toModelTeam(row *teamRow) *model.Team {
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

func toModelTeams(rows []teamRow) []model.Team {
	out := make([]model.Team, len(rows))
	for i := range rows {
		out[i] = *toModelTeam(&rows[i])
	}
	return out
}

func fromModelTeam(m *model.Team) *teamRow {
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

func toModelTeamMember(row *teamMemberRow) *model.TeamMember {
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

func toModelTeamMembers(rows []teamMemberRow) []model.TeamMember {
	out := make([]model.TeamMember, len(rows))
	for i := range rows {
		out[i] = *toModelTeamMember(&rows[i])
	}
	return out
}

func fromModelTeamMember(m *model.TeamMember) *teamMemberRow {
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
	return toModelTeam(&team), nil
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
	return toModelTeam(&team), nil
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
	return toModelTeams(list), err
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
	teamDB := fromModelTeam(team)
	memberDB := fromModelTeamMember(member)
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
			return tx.Create(fromModelTeamMember(member)).Error
		case findErr != nil:
			return findErr
		default:
			existing.Role = role
			member = toModelTeamMember(&existing)
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
	return toModelTeamMembers(list), err
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
