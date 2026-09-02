package db

import "time"

// taskRunSecretRow is one non-secret audit snapshot of what a run was granted:
// which item of which Secret resolved, how it was delivered, and the outcome.
// It holds no ciphertext, plaintext, or hash of a value -- only enough to
// explain the run. Populated by the runtime (a later phase); defined here so
// the schema exists. See docs/design/team-secrets.md §5.2.
type taskRunSecretRow struct {
	ID uint64 `gorm:"primaryKey;autoIncrement"`

	TaskRunID uint64 `gorm:"column:task_run_id;not null;index:ix_task_run_secret_run"`
	SecretID  uint64 `gorm:"column:secret_id;not null;index"`
	ItemName  string `gorm:"column:item_name;type:varchar(128);not null"`

	// AgentRevisionID is the consumption configuration that authorized the
	// grant, stored as the agent's revision number alongside its key.
	AgentID       uint64 `gorm:"column:agent_id;not null"`
	AgentRevision int    `gorm:"column:agent_revision;not null"`

	Delivery   string `gorm:"type:varchar(8);not null"` // env | file
	EnvName    string `gorm:"column:env_name;type:varchar(256);not null;default:''"`
	FileTarget string `gorm:"column:file_target;type:varchar(512);not null;default:''"`

	Status    string     `gorm:"type:varchar(16);not null;default:'pending'"`
	ExpiresAt *time.Time `gorm:"column:expires_at"`

	MaterializedAt *time.Time `gorm:"column:materialized_at"`
	RevokedAt      *time.Time `gorm:"column:revoked_at"`
	CreatedAt      time.Time  `gorm:"autoCreateTime"`
}

func (taskRunSecretRow) TableName() string { return "task_run_secret" }
