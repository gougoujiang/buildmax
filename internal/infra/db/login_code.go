package db

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"time"

	"github.com/gougoujiang/buildmax/internal/core/model"

	"gorm.io/gorm"
)

type loginCodeRow struct {
	ID        uint   `gorm:"primaryKey;autoIncrement"`
	CodeHash  string `gorm:"type:varchar(128);uniqueIndex;not null"`
	UserID    string `gorm:"type:varchar(64);not null;index"`
	ExpiresAt int64  `gorm:"not null;index"`
	UsedAt    *int64
	CreatedAt int64 `gorm:"autoCreateTime"`
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
func (s *Store) CreateLoginCode(ctx context.Context, userID string, ttl time.Duration) (string, int64, error) {
	if userID == "" {
		return "", 0, errors.New("login code: user id required")
	}
	if ttl <= 0 {
		ttl = model.LoginCodeTTLDefault
	}
	b := make([]byte, loginCodeBytes)
	if _, err := rand.Read(b); err != nil {
		return "", 0, err
	}
	plaintext := loginCodePrefix + hex.EncodeToString(b)
	expiresAt := time.Now().Add(ttl).Unix()
	row := loginCodeRow{
		CodeHash:  hashLoginCode(plaintext),
		UserID:    userID,
		ExpiresAt: expiresAt,
	}
	if err := s.db.WithContext(ctx).Create(&row).Error; err != nil {
		return "", 0, err
	}
	return plaintext, expiresAt, nil
}

// ConsumeLoginCode implements model.LoginCodeStore.
//
// The redemption is a conditional UPDATE rather than a read-then-write: two
// requests racing with the same code both match the row, and only the one whose
// UPDATE reports a changed row is allowed to proceed.
func (s *Store) ConsumeLoginCode(ctx context.Context, plaintext string, now int64) (string, error) {
	if plaintext == "" {
		return "", nil
	}
	hash := hashLoginCode(plaintext)
	var userID string
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		res := tx.Model(&loginCodeRow{}).
			Where("code_hash = ? AND used_at IS NULL AND expires_at > ?", hash, now).
			Update("used_at", now)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return nil // unknown, spent, or expired — indistinguishable on purpose
		}
		var row loginCodeRow
		if err := tx.Where("code_hash = ?", hash).First(&row).Error; err != nil {
			return err
		}
		userID = row.UserID
		return nil
	})
	if err != nil {
		return "", err
	}
	return userID, nil
}

// DeleteExpiredLoginCodes removes codes that can no longer be redeemed. Spent
// and expired rows have no security value — the hash is not reversible — but
// letting them accumulate forever serves nobody either.
func (s *Store) DeleteExpiredLoginCodes(ctx context.Context, before int64) (int64, error) {
	res := s.db.WithContext(ctx).
		Where("expires_at <= ? OR used_at IS NOT NULL", before).
		Delete(&loginCodeRow{})
	return res.RowsAffected, res.Error
}
