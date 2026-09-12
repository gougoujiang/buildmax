package db

import (
	"context"
	"errors"
	"time"

	corespace "github.com/icloudbb/buildmax/internal/core/space"
	"github.com/icloudbb/buildmax/internal/util"
	"gorm.io/gorm"
)

type spaceInvitationRow struct {
	ID         uint64     `gorm:"primaryKey;autoIncrement"`
	PublicID   string     `gorm:"column:public_id;type:char(20) CHARACTER SET ascii COLLATE ascii_bin;uniqueIndex:uq_space_invitation_public_id;not null"`
	SpaceID    uint64     `gorm:"column:space_id;not null;index"`
	UserID     uint64     `gorm:"column:user_id;not null;index"`
	Role       string     `gorm:"type:varchar(32);not null"`
	InvitedBy  uint64     `gorm:"column:invited_by;not null"`
	ExpiresAt  time.Time  `gorm:"column:expires_at;not null"`
	AcceptedAt *time.Time `gorm:"column:accepted_at"`
	RevokedAt  *time.Time `gorm:"column:revoked_at"`
	CreatedAt  time.Time  `gorm:"autoCreateTime"`
}

func (spaceInvitationRow) TableName() string { return "space_invitation" }

// spaceInvitationReadRow is spaceInvitationRow plus the handles its three user
// and space references resolve to. See spaceReadRow for why the row is a named
// field rather than an anonymous one.
type spaceInvitationReadRow struct {
	Row               spaceInvitationRow `gorm:"embedded"`
	SpacePublicID     string             `gorm:"column:space_public_id"`
	UserPublicID      string             `gorm:"column:user_public_id"`
	InvitedByPublicID string             `gorm:"column:invited_by_public_id"`
}

func (s *Store) spaceInvitationSelect(ctx context.Context) *gorm.DB {
	return s.db.WithContext(ctx).Model(&spaceInvitationRow{}).
		Select("space_invitation.*, t.public_id AS space_public_id, u.public_id AS user_public_id, ib.public_id AS invited_by_public_id").
		Joins("INNER JOIN space t ON t.id = space_invitation.space_id").
		Joins("INNER JOIN `user` u ON u.id = space_invitation.user_id").
		Joins("INNER JOIN `user` ib ON ib.id = space_invitation.invited_by")
}

func toInvitation(row *spaceInvitationReadRow) *corespace.Invitation {
	if row == nil {
		return nil
	}
	return &corespace.Invitation{
		ID:         row.Row.PublicID,
		SpaceID:    row.SpacePublicID,
		UserID:     row.UserPublicID,
		Role:       row.Row.Role,
		InvitedBy:  row.InvitedByPublicID,
		ExpiresAt:  row.Row.ExpiresAt,
		AcceptedAt: row.Row.AcceptedAt,
		RevokedAt:  row.Row.RevokedAt,
		CreatedAt:  row.Row.CreatedAt,
	}
}

func toInvitations(rows []spaceInvitationReadRow) []corespace.Invitation {
	out := make([]corespace.Invitation, len(rows))
	for i := range rows {
		out[i] = *toInvitation(&rows[i])
	}
	return out
}

// CreateInvitation implements corespace.Store.
func (s *Store) CreateInvitation(ctx context.Context, spaceID, userID, role, invitedBy string, expiresAt time.Time) (*corespace.Invitation, error) {
	row := &spaceInvitationRow{Role: role, ExpiresAt: expiresAt}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		spaceKey, err := lookupKey(ctx, tx, "space", spaceID)
		if err != nil {
			return err
		}
		userKey, err := lookupKey(ctx, tx, "user", userID)
		if err != nil {
			return err
		}
		inviterKey, err := lookupKey(ctx, tx, "user", invitedBy)
		if err != nil {
			return err
		}
		row.SpaceID = spaceKey
		row.UserID = userKey
		row.InvitedBy = inviterKey
		return createWithPublicID(ctx, tx, "uq_space_invitation_public_id",
			func(id string) { row.PublicID = id }, row)
	})
	if err != nil {
		return nil, err
	}
	return &corespace.Invitation{
		ID: row.PublicID, SpaceID: spaceID, UserID: userID, Role: role,
		InvitedBy: invitedBy, ExpiresAt: expiresAt, CreatedAt: row.CreatedAt,
	}, nil
}

// GetInvitation implements corespace.Store.
func (s *Store) GetInvitation(ctx context.Context, invitationID string) (*corespace.Invitation, error) {
	id, ok := util.CanonicalPublicID(invitationID)
	if !ok {
		return nil, nil
	}
	var row spaceInvitationReadRow
	err := s.spaceInvitationSelect(ctx).Where("space_invitation.public_id = ?", id).Take(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return toInvitation(&row), nil
}

// ListPendingInvitationsBySpace implements corespace.Store.
func (s *Store) ListPendingInvitationsBySpace(ctx context.Context, spaceID string, now time.Time) ([]corespace.Invitation, error) {
	id, ok := util.CanonicalPublicID(spaceID)
	if !ok {
		return nil, nil
	}
	var rows []spaceInvitationReadRow
	err := s.spaceInvitationSelect(ctx).
		Where("t.public_id = ? AND space_invitation.accepted_at IS NULL AND space_invitation.revoked_at IS NULL AND space_invitation.expires_at > ?", id, now).
		Order("space_invitation.created_at DESC").
		Find(&rows).Error
	return toInvitations(rows), err
}

// ListPendingInvitationsByUser implements corespace.Store.
func (s *Store) ListPendingInvitationsByUser(ctx context.Context, userID string, now time.Time) ([]corespace.Invitation, error) {
	id, ok := util.CanonicalPublicID(userID)
	if !ok {
		return nil, nil
	}
	var rows []spaceInvitationReadRow
	err := s.spaceInvitationSelect(ctx).
		Where("u.public_id = ? AND space_invitation.accepted_at IS NULL AND space_invitation.revoked_at IS NULL AND space_invitation.expires_at > ?", id, now).
		Order("space_invitation.created_at DESC").
		Find(&rows).Error
	return toInvitations(rows), err
}

// AcceptInvitation implements corespace.Store. It is atomic with the resulting
// space_member row -- see the interface doc for why.
func (s *Store) AcceptInvitation(ctx context.Context, invitationID string, now time.Time) (*corespace.Invitation, error) {
	id, ok := util.CanonicalPublicID(invitationID)
	if !ok {
		return nil, nil
	}
	var result *corespace.Invitation
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row spaceInvitationRow
		findErr := tx.Where("public_id = ?", id).Take(&row).Error
		if errors.Is(findErr, gorm.ErrRecordNotFound) {
			return nil
		}
		if findErr != nil {
			return findErr
		}
		if row.AcceptedAt != nil || row.RevokedAt != nil || !now.Before(row.ExpiresAt) {
			return nil
		}
		row.AcceptedAt = &now
		if err := tx.Save(&row).Error; err != nil {
			return err
		}
		var existing spaceMemberRow
		memberErr := tx.Where("space_id = ? AND user_id = ?", row.SpaceID, row.UserID).First(&existing).Error
		switch {
		case errors.Is(memberErr, gorm.ErrRecordNotFound):
			if err := tx.Create(&spaceMemberRow{
				SpaceID: row.SpaceID, UserID: row.UserID, Role: row.Role, CreatedAt: now,
			}).Error; err != nil {
				return err
			}
		case memberErr != nil:
			return memberErr
		default:
			existing.Role = row.Role
			if err := tx.Save(&existing).Error; err != nil {
				return err
			}
		}
		spacePublicID, err := publicIDForKey(ctx, tx, "space", row.SpaceID)
		if err != nil {
			return err
		}
		userPublicID, err := publicIDForKey(ctx, tx, "user", row.UserID)
		if err != nil {
			return err
		}
		inviterPublicID, err := publicIDForKey(ctx, tx, "user", row.InvitedBy)
		if err != nil {
			return err
		}
		result = &corespace.Invitation{
			ID: row.PublicID, SpaceID: spacePublicID, UserID: userPublicID, Role: row.Role,
			InvitedBy: inviterPublicID, ExpiresAt: row.ExpiresAt, AcceptedAt: row.AcceptedAt,
			CreatedAt: row.CreatedAt,
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// RevokeInvitation implements corespace.Store.
func (s *Store) RevokeInvitation(ctx context.Context, invitationID string, now time.Time) error {
	id, ok := util.CanonicalPublicID(invitationID)
	if !ok {
		return nil
	}
	return s.db.WithContext(ctx).Model(&spaceInvitationRow{}).
		Where("public_id = ? AND accepted_at IS NULL AND revoked_at IS NULL", id).
		Update("revoked_at", now).Error
}
