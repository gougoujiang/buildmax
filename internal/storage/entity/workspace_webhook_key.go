package entity

// UserWebhookKey is a webhook API key for a user. Plaintext key is returned only at creation; only key_hash is stored.
// JSON uses snake_case per project convention.
type UserWebhookKey struct {
	ID        uint   `gorm:"primaryKey;autoIncrement" json:"-"`
	KeyID     string `gorm:"type:varchar(64);uniqueIndex;not null" json:"key_id"`
	UserID    string `gorm:"type:varchar(64);not null;index" json:"user_id"`
	KeyHash   string `gorm:"type:varchar(128);not null;uniqueIndex" json:"-"` // SHA256 hex of plaintext key
	Name      string `gorm:"type:varchar(255)" json:"name,omitempty"`
	CreatedAt int64  `gorm:"autoCreateTime" json:"created_at"`
}

// TableName returns the table name for GORM (singular per project convention).
func (UserWebhookKey) TableName() string { return "user_webhook_key" }

// WebhookKeyMeta is key metadata returned by ListKeys (no plaintext).
type WebhookKeyMeta struct {
	KeyID     string `json:"key_id"`
	Name      string `json:"name,omitempty"`
	CreatedAt int64  `json:"created_at"`
}
