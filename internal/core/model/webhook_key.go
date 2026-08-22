package model

import "context"

// UserWebhookKey is a webhook API key for a user. Plaintext key is returned only at creation; only key_hash is stored.
// JSON uses snake_case per project convention.
type UserWebhookKey struct {
	ID        string `json:"id"`
	UserID    string `json:"user_id"`
	KeyHash   string `json:"-"` // SHA256 hex of plaintext key
	Name      string `json:"name,omitempty"`
	CreatedAt int64  `json:"created_at"`
}

// WebhookKeyMeta is key metadata returned by ListKeys (no plaintext).
type WebhookKeyMeta struct {
	KeyID     string `json:"key_id"`
	Name      string `json:"name,omitempty"`
	CreatedAt int64  `json:"created_at"`
}

// UserWebhookKeyStore provides per-user webhook API key persistence.
// Keys are stored by hash; plaintext is returned only from CreateKey.
type UserWebhookKeyStore interface {
	// CreateKey creates a new webhook key for the user. Returns plaintext key (e.g. whsec_...) and key_id. Caller must store plaintext securely; it is not persisted.
	CreateKey(ctx context.Context, userID, name string) (plaintextKey, keyID string, err error)
	// GetUserIDByKey looks up the user_id for the given plaintext key. Returns empty string if not found.
	GetUserIDByKey(ctx context.Context, plaintextKey string) (userID string, err error)
	// ListKeys returns key metadata for the user (no plaintext).
	ListKeys(ctx context.Context, userID string) ([]WebhookKeyMeta, error)
	// RevokeKey deletes the key by keyID if it belongs to the user.
	RevokeKey(ctx context.Context, userID, keyID string) error
}
