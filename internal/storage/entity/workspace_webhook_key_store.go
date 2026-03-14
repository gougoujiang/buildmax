package entity

import (
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

// CreateKey creates a new webhook key for the workspace. Returns plaintext key and key_id.
func (s *Store) CreateKey(ctx context.Context, workspaceID, name string) (plaintextKey, keyID string, err error) {
	b := make([]byte, webhookKeyBytes)
	if _, err := rand.Read(b); err != nil {
		return "", "", err
	}
	plaintextKey = webhookKeyPrefix + hex.EncodeToString(b)
	hash := sha256.Sum256([]byte(plaintextKey))
	keyHash := hex.EncodeToString(hash[:])
	keyID = util.NewPrefixedID(util.PrefixWebhookKey)
	row := WorkspaceWebhookKey{
		KeyID:       keyID,
		WorkspaceID: workspaceID,
		KeyHash:     keyHash,
		Name:        name,
	}
	if err := s.db.WithContext(ctx).Create(&row).Error; err != nil {
		return "", "", err
	}
	return plaintextKey, keyID, nil
}

// GetWorkspaceIDByKey looks up workspace_id by the plaintext key (hashed and matched).
func (s *Store) GetWorkspaceIDByKey(ctx context.Context, plaintextKey string) (workspaceID string, err error) {
	hash := sha256.Sum256([]byte(plaintextKey))
	keyHash := hex.EncodeToString(hash[:])
	var row WorkspaceWebhookKey
	err = s.db.WithContext(ctx).Where("key_hash = ?", keyHash).First(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", nil
		}
		return "", err
	}
	return row.WorkspaceID, nil
}

// ListKeys returns key metadata for the workspace.
func (s *Store) ListKeys(ctx context.Context, workspaceID string) ([]WebhookKeyMeta, error) {
	var rows []WorkspaceWebhookKey
	if err := s.db.WithContext(ctx).Where("workspace_id = ?", workspaceID).Order("created_at ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]WebhookKeyMeta, len(rows))
	for i := range rows {
		out[i] = WebhookKeyMeta{KeyID: rows[i].KeyID, Name: rows[i].Name, CreatedAt: rows[i].CreatedAt}
	}
	return out, nil
}

// RevokeKey deletes the key if it belongs to the workspace.
func (s *Store) RevokeKey(ctx context.Context, workspaceID, keyID string) error {
	res := s.db.WithContext(ctx).Where("workspace_id = ? AND key_id = ?", workspaceID, keyID).Delete(&WorkspaceWebhookKey{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return fmt.Errorf("webhook key not found or not in workspace")
	}
	return nil
}
