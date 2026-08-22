package db

import (
	"context"
	"errors"

	"github.com/gougoujiang/buildmax/internal/core/model"
	"github.com/gougoujiang/buildmax/internal/util"
	"gorm.io/gorm"
)

// systemGrantRow is one deployment-scoped authority held by one user.
//
// Revocation sets revoked_at; nothing here deletes. The table answers "who
// could operate this deployment, and when", and a deleted row cannot answer it.
type systemGrantRow struct {
	ID            uint   `gorm:"primaryKey;autoIncrement"`
	SystemGrantID string `gorm:"column:system_grant_id;type:varchar(64);uniqueIndex;not null"`

	// The composite unique index is what keeps one user from holding two
	// active grants for the same role. RevokedAt participates in it so that
	// revoked rows do not collide with each other or with a later re-grant:
	// MySQL treats NULLs as distinct in a unique index, which here means at
	// most one live row per (user, role) and any number of retired ones.
	UserID    string `gorm:"type:varchar(64);not null;uniqueIndex:idx_system_grant_live,priority:1;index:idx_system_grant_user"`
	Role      string `gorm:"type:varchar(32);not null;uniqueIndex:idx_system_grant_live,priority:2"`
	RevokedAt *int64 `gorm:"uniqueIndex:idx_system_grant_live,priority:3"`

	GrantedBy string `gorm:"type:varchar(64);not null"`
	GrantedAt int64  `gorm:"not null;index"`
}

func (systemGrantRow) TableName() string { return "system_grant" }

func toSystemGrant(row *systemGrantRow) *model.SystemGrant {
	if row == nil {
		return nil
	}
	return &model.SystemGrant{
		ID:        row.SystemGrantID,
		UserID:    row.UserID,
		Role:      row.Role,
		GrantedBy: row.GrantedBy,
		GrantedAt: row.GrantedAt,
		RevokedAt: row.RevokedAt,
	}
}

// ActiveSystemRoles returns the roles the user currently holds.
//
// This runs on every authenticated request to an admin route, so it selects
// one column against the user index and nothing else.
func (s *Store) ActiveSystemRoles(ctx context.Context, userID string) ([]string, error) {
	if userID == "" {
		return nil, nil
	}
	var roles []string
	if err := s.db.WithContext(ctx).
		Model(&systemGrantRow{}).
		Where("user_id = ? AND revoked_at IS NULL", userID).
		Pluck("role", &roles).Error; err != nil {
		return nil, err
	}
	return roles, nil
}

// ListSystemGrants returns grants newest first.
func (s *Store) ListSystemGrants(ctx context.Context, includeRevoked bool) ([]model.SystemGrant, error) {
	q := s.db.WithContext(ctx).Model(&systemGrantRow{})
	if !includeRevoked {
		q = q.Where("revoked_at IS NULL")
	}
	var rows []systemGrantRow
	if err := q.Order("granted_at DESC, id DESC").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]model.SystemGrant, 0, len(rows))
	for i := range rows {
		out = append(out, *toSystemGrant(&rows[i]))
	}
	return out, nil
}

// GrantSystemRole grants role to userID.
//
// The existence check and the insert are not in one transaction, and do not
// need to be: the unique index on (user_id, role, revoked_at) is what actually
// enforces one active grant, so a lost race ends as a duplicate-key error
// rather than as a second row. The check exists to turn the common case into a
// clear message instead of a driver error.
func (s *Store) GrantSystemRole(ctx context.Context, userID, role, grantedBy string, now int64) (*model.SystemGrant, error) {
	if !model.ValidSystemRole(role) {
		return nil, model.ErrSystemRoleUnknown
	}
	var existing systemGrantRow
	err := s.db.WithContext(ctx).
		Where("user_id = ? AND role = ? AND revoked_at IS NULL", userID, role).
		First(&existing).Error
	if err == nil {
		return nil, model.ErrSystemGrantExists
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	publicID, err := util.NewPublicID()
	if err != nil {
		return nil, err
	}
	row := systemGrantRow{
		SystemGrantID: publicID,
		UserID:        userID,
		Role:          role,
		GrantedBy:     grantedBy,
		GrantedAt:     now,
	}
	if err := s.db.WithContext(ctx).Create(&row).Error; err != nil {
		return nil, err
	}
	return toSystemGrant(&row), nil
}

// RevokeSystemRole revokes the active grant, reporting whether one was found.
func (s *Store) RevokeSystemRole(ctx context.Context, userID, role string, now int64) (bool, error) {
	res := s.db.WithContext(ctx).
		Model(&systemGrantRow{}).
		Where("user_id = ? AND role = ? AND revoked_at IS NULL", userID, role).
		Update("revoked_at", now)
	if res.Error != nil {
		return false, res.Error
	}
	return res.RowsAffected > 0, nil
}

// CountActiveSystemGrants counts live grants for role.
func (s *Store) CountActiveSystemGrants(ctx context.Context, role string) (int, error) {
	var n int64
	if err := s.db.WithContext(ctx).
		Model(&systemGrantRow{}).
		Where("role = ? AND revoked_at IS NULL", role).
		Count(&n).Error; err != nil {
		return 0, err
	}
	return int(n), nil
}
