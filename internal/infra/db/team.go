package db

import (
	"buildmax/internal/core/model"
	"context"
	"errors"
	"time"

	"buildmax/internal/util"
	"gorm.io/gorm"
)

// GetTeam returns the team by team_id, or (nil, nil) when not found.
func (s *Store) GetTeam(ctx context.Context, teamID string) (*model.Team, error) {
	var team model.Team
	err := s.db.WithContext(ctx).Where("team_id = ?", teamID).First(&team).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &team, nil
}

// GetPersonalTeamByUser returns the default personal team for the user, or (nil, nil) when not found.
func (s *Store) GetPersonalTeamByUser(ctx context.Context, userID string) (*model.Team, error) {
	var team model.Team
	err := s.db.WithContext(ctx).Where("personal_for_user_id = ?", userID).First(&team).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &team, nil
}

// ListTeamsByUser returns all teams the user belongs to, ordered by created_at ASC.
func (s *Store) ListTeamsByUser(ctx context.Context, userID string) ([]model.Team, error) {
	var list []model.Team
	err := s.db.WithContext(ctx).
		Table("team").
		Select("team.*").
		Joins("INNER JOIN team_member ON team_member.team_id = team.team_id").
		Where("team_member.user_id = ?", userID).
		Order("team.created_at ASC").
		Find(&list).Error
	return list, err
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
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(team).Error; err != nil {
			return err
		}
		return tx.Create(member).Error
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
		var existing model.TeamMember
		findErr := tx.Where("team_id = ? AND user_id = ?", teamID, userID).First(&existing).Error
		switch {
		case errors.Is(findErr, gorm.ErrRecordNotFound):
			return tx.Create(member).Error
		case findErr != nil:
			return findErr
		default:
			existing.Role = role
			member = &existing
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
		Delete(&model.TeamMember{}).Error
}

// ListTeamMembers returns members of the team ordered by created_at ASC.
func (s *Store) ListTeamMembers(ctx context.Context, teamID string) ([]model.TeamMember, error) {
	var list []model.TeamMember
	err := s.db.WithContext(ctx).
		Where("team_id = ?", teamID).
		Order("created_at ASC").
		Find(&list).Error
	return list, err
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
