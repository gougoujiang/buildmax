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
	ID       uint64 `gorm:"primaryKey;autoIncrement"`
	PublicID string `gorm:"column:public_id;type:char(20) CHARACTER SET ascii COLLATE ascii_bin;uniqueIndex:uq_system_grant_public_id;not null"`

	// The composite unique index is what keeps one user from holding two
	// active grants for the same role. RevokedAt participates in it so that
	// revoked rows do not collide with each other or with a later re-grant:
	// MySQL treats NULLs as distinct in a unique index, which here means at
	// most one live row per (user, role) and any number of retired ones.
	UserID    uint64 `gorm:"column:user_id;not null;uniqueIndex:idx_system_grant_live,priority:1;index:idx_system_grant_user"`
	Role      string `gorm:"type:varchar(32);not null;uniqueIndex:idx_system_grant_live,priority:2"`
	RevokedAt *int64 `gorm:"uniqueIndex:idx_system_grant_live,priority:3"`

	// GrantedBy stays an opaque handle. The operator who bootstraps the first
	// grant is a command line, not a user row, so this column cannot be a
	// reference to one.
	GrantedBy string `gorm:"type:varchar(64);not null"`
	GrantedAt int64  `gorm:"not null;index"`
}

func (systemGrantRow) TableName() string { return "system_grant" }

// systemGrantReadRow is the row plus the handle its holder resolves to.
type systemGrantReadRow struct {
	Row          systemGrantRow `gorm:"embedded"`
	UserPublicID string         `gorm:"column:user_public_id"`
}

func (s *Store) systemGrantSelect(ctx context.Context) *gorm.DB {
	return s.db.WithContext(ctx).Model(&systemGrantRow{}).
		Select("system_grant.*, u.public_id AS user_public_id").
		Joins("INNER JOIN `user` u ON u.id = system_grant.user_id")
}

func toSystemGrant(row *systemGrantReadRow) *model.SystemGrant {
	if row == nil {
		return nil
	}
	return &model.SystemGrant{
		ID:        row.Row.PublicID,
		UserID:    row.UserPublicID,
		Role:      row.Row.Role,
		GrantedBy: row.Row.GrantedBy,
		GrantedAt: row.Row.GrantedAt,
		RevokedAt: row.Row.RevokedAt,
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
	// One join rather than a resolve-then-query: this runs on every admin
	// request, and the extra round trip would be the cost of the boundary
	// rather than of the question.
	id, ok := util.CanonicalPublicID(userID)
	if !ok {
		return nil, nil
	}
	var roles []string
	if err := s.db.WithContext(ctx).
		Model(&systemGrantRow{}).
		Joins("INNER JOIN `user` u ON u.id = system_grant.user_id").
		Where("u.public_id = ? AND system_grant.revoked_at IS NULL", id).
		Pluck("role", &roles).Error; err != nil {
		return nil, err
	}
	return roles, nil
}

// ListSystemGrants returns grants newest first.
func (s *Store) ListSystemGrants(ctx context.Context, includeRevoked bool) ([]model.SystemGrant, error) {
	q := s.systemGrantSelect(ctx)
	if !includeRevoked {
		q = q.Where("system_grant.revoked_at IS NULL")
	}
	var rows []systemGrantReadRow
	if err := q.Order("system_grant.granted_at DESC, system_grant.id DESC").Find(&rows).Error; err != nil {
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
	userKey, err := lookupKey(ctx, s.db, "user", userID)
	if err != nil {
		return nil, err
	}
	var existing systemGrantRow
	err = s.db.WithContext(ctx).
		Where("user_id = ? AND role = ? AND revoked_at IS NULL", userKey, role).
		First(&existing).Error
	if err == nil {
		return nil, model.ErrSystemGrantExists
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	row := systemGrantRow{
		UserID:    userKey,
		Role:      role,
		GrantedBy: grantedBy,
		GrantedAt: now,
	}
	if err := createWithPublicID(ctx, s.db, "uq_system_grant_public_id",
		func(id string) { row.PublicID = id }, &row); err != nil {
		return nil, err
	}
	return toSystemGrant(&systemGrantReadRow{Row: row, UserPublicID: canonicalPublicID(userID)}), nil
}

// RevokeSystemRole revokes the active grant, reporting whether one was found.
func (s *Store) RevokeSystemRole(ctx context.Context, userID, role string, now int64) (bool, error) {
	userKey, err := lookupKey(ctx, s.db, "user", userID)
	if errors.Is(err, model.ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	res := s.db.WithContext(ctx).
		Model(&systemGrantRow{}).
		Where("user_id = ? AND role = ? AND revoked_at IS NULL", userKey, role).
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
