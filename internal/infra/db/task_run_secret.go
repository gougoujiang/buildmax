package db

import (
	"context"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	coresecret "github.com/gougoujiang/buildmax/internal/core/secret"
)

// taskRunSecretRow is one non-secret audit snapshot of what a run was granted:
// which item of which Secret resolved, how it was delivered, and the outcome.
// It holds no ciphertext, plaintext, or hash of a value -- only enough to
// explain the run. Populated by the runtime (a later phase); defined here so
// the schema exists. See docs/design/team-secrets.md §5.2.
type taskRunSecretRow struct {
	ID uint64 `gorm:"primaryKey;autoIncrement"`

	// One row per (run, secret, item) so a worker that retries the secrets
	// fetch records the same materialization once rather than a duplicate.
	TaskRunID uint64 `gorm:"column:task_run_id;not null;uniqueIndex:ux_task_run_secret,priority:1"`
	SecretID  uint64 `gorm:"column:secret_id;not null;uniqueIndex:ux_task_run_secret,priority:2"`
	ItemName  string `gorm:"column:item_name;type:varchar(128);not null;uniqueIndex:ux_task_run_secret,priority:3"`

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

// RecordEnvGrant records one materialized env grant. It is idempotent on
// (task_run_id, secret_id, item_name): a worker that retries the secrets fetch
// re-records the same materialization once. Public IDs are resolved to keys
// here; an unknown reference is skipped rather than erroring, because this is a
// fail-open audit write beside a run that already succeeded in getting its
// grant.
func (s *Store) RecordEnvGrant(ctx context.Context, in coresecret.GrantRecord) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		runKey, err := lookupKey(ctx, tx, "task_run", in.TaskRunID)
		if err != nil {
			return err
		}
		secretKey, err := lookupKey(ctx, tx, "secret", in.SecretID)
		if err != nil {
			return err
		}
		agentKey, err := lookupKey(ctx, tx, "agent", in.AgentID)
		if err != nil {
			return err
		}
		now := time.Now().UTC()
		row := &taskRunSecretRow{
			TaskRunID:      runKey,
			SecretID:       secretKey,
			ItemName:       in.ItemName,
			AgentID:        agentKey,
			AgentRevision:  in.AgentRevision,
			Delivery:       "env",
			EnvName:        in.EnvName,
			Status:         "materialized",
			MaterializedAt: &now,
		}
		// Idempotent: a retry updates the same row's outcome rather than
		// inserting a duplicate or failing the unique constraint.
		return tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "task_run_id"}, {Name: "secret_id"}, {Name: "item_name"}},
			DoUpdates: clause.AssignmentColumns([]string{"agent_id", "agent_revision", "env_name", "status", "materialized_at"}),
		}).Create(row).Error
	})
}
