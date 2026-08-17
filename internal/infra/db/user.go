package db

import (
	"context"
	"errors"
	"github.com/gougoujiang/buildmax/internal/core/model"
	"time"

	"github.com/gougoujiang/buildmax/internal/util"
	"gorm.io/gorm"
)

type userRow struct {
	ID     uint   `gorm:"primaryKey;autoIncrement"`
	UserID string `gorm:"type:varchar(64);uniqueIndex;not null"`
	Email  string `gorm:"type:varchar(255);uniqueIndex;not null"`
	Name   string `gorm:"type:varchar(255)"`
	// PasswordHash is NULL for an account that has never set one: created by an
	// operator and not yet claimed, or signing in with login codes only. It
	// stays nullable so that an account authenticated somewhere else — an
	// identity provider, when there is one — needs no local password to exist.
	PasswordHash      *string `gorm:"type:varchar(255)"`
	PasswordSetAt     *int64  `gorm:""`
	QuotaTier         string  `gorm:"type:varchar(64)"`
	LastLoginAt       *int64  `gorm:""`
	LastLoginPlatform *string `gorm:"type:varchar(32)"`
	CreatedAt         int64   `gorm:"autoCreateTime"`
}

func (userRow) TableName() string { return "user" }

func toUser(row *userRow) *model.User {
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
		// Whether a password exists, never what it is.
		HasPassword: row.PasswordHash != nil && *row.PasswordHash != "",
	}
}

func toUserRow(m *model.User) *userRow {
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
	return toUser(&u), nil
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
	return toUser(&u), nil
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
	userDB := toUserRow(&u)
	personalTeamDB := toTeamRow(&personalTeam)
	personalMemberDB := toTeamMemberRow(&personalMember)
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

// PasswordHash implements model.PasswordStore.
//
// A missing account and an account with no password both return "", because
// the caller does the same thing with either: refuse, having spent the same
// work. Distinguishing them here would only invite a handler to distinguish
// them in a response.
func (s *Store) PasswordHash(ctx context.Context, userID string) (string, error) {
	if userID == "" {
		return "", nil
	}
	var row userRow
	err := s.db.WithContext(ctx).Select("password_hash").Where("user_id = ?", userID).First(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", nil
		}
		return "", err
	}
	if row.PasswordHash == nil {
		return "", nil
	}
	return *row.PasswordHash, nil
}

// SetPassword implements model.PasswordStore.
func (s *Store) SetPassword(ctx context.Context, userID, encodedHash string, setAt int64) error {
	if userID == "" {
		return errors.New("set password: user id required")
	}
	if encodedHash == "" {
		return errors.New("set password: hash required")
	}
	res := s.db.WithContext(ctx).Model(&userRow{}).
		Where("user_id = ?", userID).
		Updates(map[string]any{"password_hash": encodedHash, "password_set_at": setAt})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return model.ErrUserNotFound
	}
	return nil
}
