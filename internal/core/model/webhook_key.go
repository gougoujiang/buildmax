package model

// UserWebhookKey is a webhook API key for a user. Plaintext key is returned only at creation; only key_hash is stored.
// JSON uses snake_case per project convention.
type UserWebhookKey struct {
	ID        uint   `json:"-"`
	KeyID     string `json:"key_id"`
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
