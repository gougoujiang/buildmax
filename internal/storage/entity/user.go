package entity

import (
	"context"
	"errors"
	"time"

	"buildmax/internal/util"
	"gorm.io/gorm"
)

// UserByEmail returns the user with the given email, or (nil, nil) when not found.
func (s *Store) UserByEmail(ctx context.Context, email string) (*User, error) {
	var u User
	err := s.db.WithContext(ctx).Where("email = ?", email).First(&u).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &u, nil
}

// GetUser returns the user by user_id, or (nil, nil) when not found.
func (s *Store) GetUser(ctx context.Context, userID string) (*User, error) {
	var u User
	err := s.db.WithContext(ctx).Where("user_id = ?", userID).First(&u).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &u, nil
}

// UpdateLoginMeta records the last login timestamp and platform for the user.
func (s *Store) UpdateLoginMeta(ctx context.Context, userID string, loginAt int64, platform string) error {
	return s.db.WithContext(ctx).
		Model(&User{}).
		Where("user_id = ?", userID).
		Updates(map[string]interface{}{
			"last_login_at":       loginAt,
			"last_login_platform": platform,
		}).Error
}

// CreateUser creates a user with the given email. Name is set to empty.
// When defaultQuotaTier is non-empty, User.QuotaTier is set to it.
// Returns ErrEmailExists if the email is already registered.
func (s *Store) CreateUser(ctx context.Context, email string, defaultQuotaTier string) (*User, error) {
	existing, err := s.UserByEmail(ctx, email)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return nil, ErrEmailExists
	}
	u := User{
		UserID:    util.NewPrefixedID(util.PrefixUser),
		Email:     email,
		Name:      "",
		CreatedAt: time.Now().Unix(),
	}
	if defaultQuotaTier != "" {
		u.QuotaTier = defaultQuotaTier
	}
	personalTeamID := util.NewPrefixedID(util.PrefixTeam)
	personalTeam := Team{
		TeamID:            personalTeamID,
		Name:              DefaultPersonalTeamName,
		PersonalForUserID: &u.UserID,
		QuotaTier:         defaultQuotaTier,
		CreatedBy:         u.UserID,
		CreatedAt:         u.CreatedAt,
		UpdatedAt:         u.CreatedAt,
	}
	personalMember := TeamMember{
		TeamID:    personalTeamID,
		UserID:    u.UserID,
		Role:      TeamRoleOwner,
		CreatedAt: u.CreatedAt,
	}
	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&u).Error; err != nil {
			return err
		}
		if err := tx.Create(&personalTeam).Error; err != nil {
			return err
		}
		return tx.Create(&personalMember).Error
	}); err != nil {
		return nil, err
	}
	return &u, nil
}
