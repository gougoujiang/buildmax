package entity

import (
	"context"
	"errors"
	"time"

	"buildmax/internal/util"
	"gorm.io/gorm"
)

const (
	// TeamRoleOwner is the initial role for the user who creates a team.
	TeamRoleOwner = "owner"
	// TeamRoleAdmin can manage shared automation assets but not membership ownership.
	TeamRoleAdmin = "admin"
	// TeamRoleMember is the basic collaboration role for invited members.
	TeamRoleMember = "member"
	// DefaultPersonalTeamName is the initial UX-facing name for a user's own space.
	DefaultPersonalTeamName = "My Space"
)

// GetTeam returns the team by team_id, or (nil, nil) when not found.
func (s *Store) GetTeam(ctx context.Context, teamID string) (*Team, error) {
	var team Team
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
func (s *Store) GetPersonalTeamByUser(ctx context.Context, userID string) (*Team, error) {
	var team Team
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
func (s *Store) ListTeamsByUser(ctx context.Context, userID string) ([]Team, error) {
	var list []Team
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
func (s *Store) CreateTeam(ctx context.Context, name, createdBy, quotaTier string) (*Team, error) {
	now := time.Now().Unix()
	team := &Team{
		TeamID:    util.NewPrefixedID(util.PrefixTeam),
		Name:      name,
		QuotaTier: quotaTier,
		CreatedBy: createdBy,
		CreatedAt: now,
		UpdatedAt: now,
	}
	member := &TeamMember{
		TeamID:    team.TeamID,
		UserID:    createdBy,
		Role:      TeamRoleOwner,
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
func (s *Store) AddTeamMember(ctx context.Context, teamID, userID, role string) (*TeamMember, error) {
	member := &TeamMember{
		TeamID:    teamID,
		UserID:    userID,
		Role:      role,
		CreatedAt: time.Now().Unix(),
	}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var existing TeamMember
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
		Delete(&TeamMember{}).Error
}

// ListTeamMembers returns members of the team ordered by created_at ASC.
func (s *Store) ListTeamMembers(ctx context.Context, teamID string) ([]TeamMember, error) {
	var list []TeamMember
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
