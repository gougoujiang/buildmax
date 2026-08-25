package db

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"time"

	"github.com/gougoujiang/buildmax/internal/core/apierr"
	"github.com/gougoujiang/buildmax/internal/core/model"
)

type loginCodeRow struct {
	ID        uint64    `gorm:"primaryKey;autoIncrement"`
	CodeHash  string    `gorm:"type:varchar(128);uniqueIndex;not null"`
	UserID    uint64    `gorm:"column:user_id;not null;index"`
	ExpiresAt time.Time `gorm:"not null;index"`
	UsedAt    *time.Time
	CreatedAt time.Time `gorm:"autoCreateTime"`
}

func (loginCodeRow) TableName() string { return "login_code" }

const (
	// The prefix makes a leaked code recognizable in a log or a chat history,
	// which is what secret scanners key on.
	loginCodePrefix = "bmxlogin_"
	loginCodeBytes  = 32
)

// hashLoginCode is the only representation that reaches the database. A stolen
// database backup therefore yields no usable codes.
func hashLoginCode(plaintext string) string {
	sum := sha256.Sum256([]byte(plaintext))
	return hex.EncodeToString(sum[:])
}

// CreateLoginCode implements model.LoginCodeStore.
func (s *Store) CreateLoginCode(ctx context.Context, userID string, ttl time.Duration) (string, time.Time, error) {
	if userID == "" {
		return "", time.Time{}, errors.New("login code: user id required")
	}
	if ttl <= 0 {
		ttl = model.LoginCodeTTLDefault
	}
	b := make([]byte, loginCodeBytes)
	if _, err := rand.Read(b); err != nil {
		return "", time.Time{}, err
	}
	userKey, err := lookupKey(ctx, s.db, "user", userID)
	if err != nil {
		return "", time.Time{}, err
	}
	plaintext := loginCodePrefix + hex.EncodeToString(b)
	expiresAt := time.Now().UTC().Add(ttl)
	row := loginCodeRow{
		CodeHash:  hashLoginCode(plaintext),
		UserID:    userKey,
		ExpiresAt: expiresAt,
	}
	if err := s.db.WithContext(ctx).Create(&row).Error; err != nil {
		return "", time.Time{}, err
	}
	return plaintext, expiresAt, nil
}

// ConsumeLoginCode implements model.LoginCodeStore.
//
// The redemption is a conditional UPDATE rather than a read-then-write: two
// requests racing with the same code both match the row, and only the one whose
// UPDATE reports a changed row is allowed to proceed.
//
// user_id is part of that condition, so a code submitted with somebody else's
// address matches nothing, changes nothing, and stays spendable.
func (s *Store) ConsumeLoginCode(ctx context.Context, plaintext, userID string, now time.Time) (bool, error) {
	if plaintext == "" || userID == "" {
		return false, nil
	}
	// An unknown account is not an error here: it matches no code, which is the
	// same refusal a wrong code gets and takes the same work.
	userKey, err := lookupKey(ctx, s.db, "user", userID)
	if errors.Is(err, apierr.ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	res := s.db.WithContext(ctx).Model(&loginCodeRow{}).
		Where("code_hash = ? AND user_id = ? AND used_at IS NULL AND expires_at > ?",
			hashLoginCode(plaintext), userKey, now).
		Update("used_at", now)
	if res.Error != nil {
		return false, res.Error
	}
	return res.RowsAffected == 1, nil
}

// DeleteExpiredLoginCodes removes codes that can no longer be redeemed. Spent
// and expired rows have no security value — the hash is not reversible — but
// letting them accumulate forever serves nobody either.
func (s *Store) DeleteExpiredLoginCodes(ctx context.Context, before time.Time) (int64, error) {
	res := s.db.WithContext(ctx).
		Where("expires_at <= ? OR used_at IS NOT NULL", before).
		Delete(&loginCodeRow{})
	return res.RowsAffected, res.Error
}
