package db

import (
	"buildmax/internal/core/model"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"

	"buildmax/internal/util"

	"gorm.io/gorm"
)

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
	keyID = util.NewPrefixedID(util.PrefixWebhookKey)
	row := model.UserWebhookKey{
		KeyID:   keyID,
		UserID:  userID,
		KeyHash: keyHash,
		Name:    name,
	}
	if err := s.db.WithContext(ctx).Create(&row).Error; err != nil {
		return "", "", err
	}
	return plaintextKey, keyID, nil
}

// GetUserIDByKey looks up user_id by the plaintext key (hashed and matched).
func (s *Store) GetUserIDByKey(ctx context.Context, plaintextKey string) (userID string, err error) {
	hash := sha256.Sum256([]byte(plaintextKey))
	keyHash := hex.EncodeToString(hash[:])
	var row model.UserWebhookKey
	err = s.db.WithContext(ctx).Where("key_hash = ?", keyHash).First(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", nil
		}
		return "", err
	}
	return row.UserID, nil
}

// ListKeys returns key metadata for the user.
func (s *Store) ListKeys(ctx context.Context, userID string) ([]model.WebhookKeyMeta, error) {
	var rows []model.UserWebhookKey
	if err := s.db.WithContext(ctx).Where("user_id = ?", userID).Order("created_at ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]model.WebhookKeyMeta, len(rows))
	for i := range rows {
		out[i] = model.WebhookKeyMeta{KeyID: rows[i].KeyID, Name: rows[i].Name, CreatedAt: rows[i].CreatedAt}
	}
	return out, nil
}

// RevokeKey deletes the key if it belongs to the user.
func (s *Store) RevokeKey(ctx context.Context, userID, keyID string) error {
	res := s.db.WithContext(ctx).Where("user_id = ? AND key_id = ?", userID, keyID).Delete(&model.UserWebhookKey{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return fmt.Errorf("webhook key not found or not owned by user")
	}
	return nil
}
