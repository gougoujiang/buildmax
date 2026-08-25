package db

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/gougoujiang/buildmax/internal/core/apierr"
	"github.com/gougoujiang/buildmax/internal/core/model"

	"github.com/gougoujiang/buildmax/internal/util"

	"gorm.io/gorm"
)

type userWebhookKeyRow struct {
	ID        uint64    `gorm:"primaryKey;autoIncrement"`
	PublicID  string    `gorm:"column:public_id;type:char(20) CHARACTER SET ascii COLLATE ascii_bin;uniqueIndex:uq_user_webhook_key_public_id;not null"`
	UserID    uint64    `gorm:"column:user_id;not null;index"`
	KeyHash   string    `gorm:"type:varchar(128);not null;uniqueIndex"`
	Name      string    `gorm:"type:varchar(255)"`
	CreatedAt time.Time `gorm:"autoCreateTime"`
}

func (userWebhookKeyRow) TableName() string { return "user_webhook_key" }

const webhookKeyPrefix = "whsec_"
const webhookKeyBytes = 32

// CreateKey creates a new webhook key for the user. Returns plaintext key and key_id.
func (s *Store) CreateKey(ctx context.Context, userID, name string) (plaintextKey, keyID string, err error) {
	b := make([]byte, webhookKeyBytes)
	if _, err := rand.Read(b); err != nil {
		return "", "", err
	}
	plaintextKey = webhookKeyPrefix + hex.EncodeToString(b)
	hash := sha256.Sum256([]byte(plaintextKey))
	keyHash := hex.EncodeToString(hash[:])
	userKey, err := lookupKey(ctx, s.db, "user", userID)
	if err != nil {
		return "", "", err
	}
	row := userWebhookKeyRow{
		UserID:  userKey,
		KeyHash: keyHash,
		Name:    name,
	}
	if err := createWithPublicID(ctx, s.db, "uq_user_webhook_key_public_id",
		func(id string) { row.PublicID = id }, &row); err != nil {
		return "", "", err
	}
	return plaintextKey, row.PublicID, nil
}

// GetUserIDByKey looks up user_id by the plaintext key (hashed and matched).
func (s *Store) GetUserIDByKey(ctx context.Context, plaintextKey string) (userID string, err error) {
	hash := sha256.Sum256([]byte(plaintextKey))
	keyHash := hex.EncodeToString(hash[:])
	var row struct {
		UserPublicID string
	}
	err = s.db.WithContext(ctx).Model(&userWebhookKeyRow{}).
		Select("u.public_id AS user_public_id").
		Joins("INNER JOIN `user` u ON u.id = user_webhook_key.user_id").
		Where("user_webhook_key.key_hash = ?", keyHash).
		Take(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", nil
		}
		return "", err
	}
	return row.UserPublicID, nil
}

// ListKeys returns key metadata for the user.
func (s *Store) ListKeys(ctx context.Context, userID string) ([]model.WebhookKeyMeta, error) {
	userKey, err := lookupKey(ctx, s.db, "user", userID)
	if errors.Is(err, apierr.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var rows []userWebhookKeyRow
	if err := s.db.WithContext(ctx).Where("user_id = ?", userKey).Order("created_at ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]model.WebhookKeyMeta, len(rows))
	for i := range rows {
		out[i] = model.WebhookKeyMeta{KeyID: rows[i].PublicID, Name: rows[i].Name, CreatedAt: rows[i].CreatedAt}
	}
	return out, nil
}

// RevokeKey deletes the key if it belongs to the user.
func (s *Store) RevokeKey(ctx context.Context, userID, keyID string) error {
	notOwned := fmt.Errorf("webhook key not found or not owned by user")
	userKey, err := lookupKey(ctx, s.db, "user", userID)
	if errors.Is(err, apierr.ErrNotFound) {
		return notOwned
	}
	if err != nil {
		return err
	}
	id, ok := util.CanonicalPublicID(keyID)
	if !ok {
		return notOwned
	}
	res := s.db.WithContext(ctx).Where("user_id = ? AND public_id = ?", userKey, id).Delete(&userWebhookKeyRow{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return fmt.Errorf("webhook key not found or not owned by user")
	}
	return nil
}
