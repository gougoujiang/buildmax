package db

import (
	"buildmax/internal/core/model"
	"context"
	"errors"
	"time"

	"buildmax/internal/util"
	"gorm.io/gorm"
)

type userRow struct {
	ID                uint    `gorm:"primaryKey;autoIncrement"`
	UserID            string  `gorm:"type:varchar(64);uniqueIndex;not null"`
	Email             string  `gorm:"type:varchar(255);uniqueIndex;not null"`
	Name              string  `gorm:"type:varchar(255)"`
	QuotaTier         string  `gorm:"type:varchar(64)"`
	LastLoginAt       *int64  `gorm:""`
	LastLoginPlatform *string `gorm:"type:varchar(32)"`
	CreatedAt         int64   `gorm:"autoCreateTime"`
}

func (userRow) TableName() string { return "user" }

func toModelUser(row *userRow) *model.User {
	if row == nil {
		return nil
	}
	return &model.User{
		ID:                row.ID,
		UserID:            row.UserID,
		Email:             row.Email,
		Name:              row.Name,
		QuotaTier:         row.QuotaTier,
		LastLoginAt:       row.LastLoginAt,
		LastLoginPlatform: row.LastLoginPlatform,
		CreatedAt:         row.CreatedAt,
	}
}

func toModelUsers(rows []userRow) []model.User {
	out := make([]model.User, len(rows))
	for i := range rows {
		out[i] = *toModelUser(&rows[i])
	}
	return out
}

func fromModelUser(m *model.User) *userRow {
	if m == nil {
		return nil
	}
	return &userRow{
		ID:                m.ID,
		UserID:            m.UserID,
		Email:             m.Email,
		Name:              m.Name,
		QuotaTier:         m.QuotaTier,
		LastLoginAt:       m.LastLoginAt,
		LastLoginPlatform: m.LastLoginPlatform,
		CreatedAt:         m.CreatedAt,
	}
}

// UserByEmail returns the user with the given email, or (nil, nil) when not found.
func (s *Store) UserByEmail(ctx context.Context, email string) (*model.User, error) {
	var u userRow
	err := s.db.WithContext(ctx).Where("email = ?", email).First(&u).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return toModelUser(&u), nil
}

// GetUser returns the user by user_id, or (nil, nil) when not found.
func (s *Store) GetUser(ctx context.Context, userID string) (*model.User, error) {
	var u userRow
	err := s.db.WithContext(ctx).Where("user_id = ?", userID).First(&u).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return toModelUser(&u), nil
}

// UpdateLoginMeta records the last login timestamp and platform for the user.
func (s *Store) UpdateLoginMeta(ctx context.Context, userID string, loginAt int64, platform string) error {
	return s.db.WithContext(ctx).
		Model(&userRow{}).
		Where("user_id = ?", userID).
		Updates(map[string]interface{}{
			"last_login_at":       loginAt,
			"last_login_platform": platform,
		}).Error
}

// CreateUser creates a user with the given email. Name is set to empty.
// When defaultQuotaTier is non-empty, User.QuotaTier is set to it.
// Returns ErrEmailExists if the email is already registered.
func (s *Store) CreateUser(ctx context.Context, email string, defaultQuotaTier string) (*model.User, error) {
	existing, err := s.UserByEmail(ctx, email)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return nil, model.ErrEmailExists
	}
	u := model.User{
		UserID:    util.NewPrefixedID(util.PrefixUser),
		Email:     email,
		Name:      "",
		CreatedAt: time.Now().Unix(),
	}
	if defaultQuotaTier != "" {
		u.QuotaTier = defaultQuotaTier
	}
	personalTeamID := util.NewPrefixedID(util.PrefixTeam)
	personalTeam := model.Team{
		TeamID:            personalTeamID,
		Name:              model.DefaultPersonalTeamName,
		PersonalForUserID: &u.UserID,
		QuotaTier:         defaultQuotaTier,
		CreatedBy:         u.UserID,
		CreatedAt:         u.CreatedAt,
		UpdatedAt:         u.CreatedAt,
	}
	personalMember := model.TeamMember{
		TeamID:    personalTeamID,
		UserID:    u.UserID,
		Role:      model.TeamRoleOwner,
		CreatedAt: u.CreatedAt,
	}
	userDB := fromModelUser(&u)
	personalTeamDB := fromModelTeam(&personalTeam)
	personalMemberDB := fromModelTeamMember(&personalMember)
	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(userDB).Error; err != nil {
			return err
		}
		if err := tx.Create(personalTeamDB).Error; err != nil {
			return err
		}
		return tx.Create(personalMemberDB).Error
	}); err != nil {
		return nil, err
	}
	return &u, nil
}
