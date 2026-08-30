package db

import (
	"context"
	"errors"
	"time"

	coreteam "github.com/gougoujiang/buildmax/internal/core/team"
	"github.com/gougoujiang/buildmax/internal/util"
	"gorm.io/gorm"
)

type teamInvitationRow struct {
	ID         uint64     `gorm:"primaryKey;autoIncrement"`
	PublicID   string     `gorm:"column:public_id;type:char(20) CHARACTER SET ascii COLLATE ascii_bin;uniqueIndex:uq_team_invitation_public_id;not null"`
	TeamID     uint64     `gorm:"column:team_id;not null;index"`
	UserID     uint64     `gorm:"column:user_id;not null;index"`
	Role       string     `gorm:"type:varchar(32);not null"`
	InvitedBy  uint64     `gorm:"column:invited_by;not null"`
	ExpiresAt  time.Time  `gorm:"column:expires_at;not null"`
	AcceptedAt *time.Time `gorm:"column:accepted_at"`
	RevokedAt  *time.Time `gorm:"column:revoked_at"`
	CreatedAt  time.Time  `gorm:"autoCreateTime"`
}

func (teamInvitationRow) TableName() string { return "team_invitation" }

// teamInvitationReadRow is teamInvitationRow plus the handles its three user
// and team references resolve to. See teamReadRow for why the row is a named
// field rather than an anonymous one.
type teamInvitationReadRow struct {
	Row               teamInvitationRow `gorm:"embedded"`
	TeamPublicID      string            `gorm:"column:team_public_id"`
	UserPublicID      string            `gorm:"column:user_public_id"`
	InvitedByPublicID string            `gorm:"column:invited_by_public_id"`
}

func (s *Store) teamInvitationSelect(ctx context.Context) *gorm.DB {
	return s.db.WithContext(ctx).Model(&teamInvitationRow{}).
		Select("team_invitation.*, t.public_id AS team_public_id, u.public_id AS user_public_id, ib.public_id AS invited_by_public_id").
		Joins("INNER JOIN team t ON t.id = team_invitation.team_id").
		Joins("INNER JOIN `user` u ON u.id = team_invitation.user_id").
		Joins("INNER JOIN `user` ib ON ib.id = team_invitation.invited_by")
}

func toInvitation(row *teamInvitationReadRow) *coreteam.Invitation {
	if row == nil {
		return nil
	}
	return &coreteam.Invitation{
		ID:         row.Row.PublicID,
		TeamID:     row.TeamPublicID,
		UserID:     row.UserPublicID,
		Role:       row.Row.Role,
		InvitedBy:  row.InvitedByPublicID,
		ExpiresAt:  row.Row.ExpiresAt,
		AcceptedAt: row.Row.AcceptedAt,
		RevokedAt:  row.Row.RevokedAt,
		CreatedAt:  row.Row.CreatedAt,
	}
}

func toInvitations(rows []teamInvitationReadRow) []coreteam.Invitation {
	out := make([]coreteam.Invitation, len(rows))
	for i := range rows {
		out[i] = *toInvitation(&rows[i])
	}
	return out
}

// CreateInvitation implements coreteam.Store.
func (s *Store) CreateInvitation(ctx context.Context, teamID, userID, role, invitedBy string, expiresAt time.Time) (*coreteam.Invitation, error) {
	row := &teamInvitationRow{Role: role, ExpiresAt: expiresAt}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		teamKey, err := lookupKey(ctx, tx, "team", teamID)
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
		row.TeamID = teamKey
		row.UserID = userKey
		row.InvitedBy = inviterKey
		return createWithPublicID(ctx, tx, "uq_team_invitation_public_id",
			func(id string) { row.PublicID = id }, row)
	})
	if err != nil {
		return nil, err
	}
	return &coreteam.Invitation{
		ID: row.PublicID, TeamID: teamID, UserID: userID, Role: role,
		InvitedBy: invitedBy, ExpiresAt: expiresAt, CreatedAt: row.CreatedAt,
	}, nil
}

// GetInvitation implements coreteam.Store.
func (s *Store) GetInvitation(ctx context.Context, invitationID string) (*coreteam.Invitation, error) {
	id, ok := util.CanonicalPublicID(invitationID)
	if !ok {
		return nil, nil
	}
	var row teamInvitationReadRow
	err := s.teamInvitationSelect(ctx).Where("team_invitation.public_id = ?", id).Take(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return toInvitation(&row), nil
}

// ListPendingInvitationsByTeam implements coreteam.Store.
func (s *Store) ListPendingInvitationsByTeam(ctx context.Context, teamID string, now time.Time) ([]coreteam.Invitation, error) {
	id, ok := util.CanonicalPublicID(teamID)
	if !ok {
		return nil, nil
	}
	var rows []teamInvitationReadRow
	err := s.teamInvitationSelect(ctx).
		Where("t.public_id = ? AND team_invitation.accepted_at IS NULL AND team_invitation.revoked_at IS NULL AND team_invitation.expires_at > ?", id, now).
		Order("team_invitation.created_at DESC").
		Find(&rows).Error
	return toInvitations(rows), err
}

// ListPendingInvitationsByUser implements coreteam.Store.
func (s *Store) ListPendingInvitationsByUser(ctx context.Context, userID string, now time.Time) ([]coreteam.Invitation, error) {
	id, ok := util.CanonicalPublicID(userID)
	if !ok {
		return nil, nil
	}
	var rows []teamInvitationReadRow
	err := s.teamInvitationSelect(ctx).
		Where("u.public_id = ? AND team_invitation.accepted_at IS NULL AND team_invitation.revoked_at IS NULL AND team_invitation.expires_at > ?", id, now).
		Order("team_invitation.created_at DESC").
		Find(&rows).Error
	return toInvitations(rows), err
}

// AcceptInvitation implements coreteam.Store. It is atomic with the resulting
// team_member row -- see the interface doc for why.
func (s *Store) AcceptInvitation(ctx context.Context, invitationID string, now time.Time) (*coreteam.Invitation, error) {
	id, ok := util.CanonicalPublicID(invitationID)
	if !ok {
		return nil, nil
	}
	var result *coreteam.Invitation
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row teamInvitationRow
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
		var existing teamMemberRow
		memberErr := tx.Where("team_id = ? AND user_id = ?", row.TeamID, row.UserID).First(&existing).Error
		switch {
		case errors.Is(memberErr, gorm.ErrRecordNotFound):
			if err := tx.Create(&teamMemberRow{
				TeamID: row.TeamID, UserID: row.UserID, Role: row.Role, CreatedAt: now,
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
		teamPublicID, err := publicIDForKey(ctx, tx, "team", row.TeamID)
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
		result = &coreteam.Invitation{
			ID: row.PublicID, TeamID: teamPublicID, UserID: userPublicID, Role: row.Role,
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

// RevokeInvitation implements coreteam.Store.
func (s *Store) RevokeInvitation(ctx context.Context, invitationID string, now time.Time) error {
	id, ok := util.CanonicalPublicID(invitationID)
	if !ok {
		return nil
	}
	return s.db.WithContext(ctx).Model(&teamInvitationRow{}).
		Where("public_id = ? AND accepted_at IS NULL AND revoked_at IS NULL", id).
		Update("revoked_at", now).Error
}
