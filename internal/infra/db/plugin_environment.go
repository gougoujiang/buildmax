package db

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/icloudbb/buildmax/internal/core/apierr"
	coreplugin "github.com/icloudbb/buildmax/internal/core/plugin"
	"github.com/icloudbb/buildmax/internal/util"
	"gorm.io/gorm"
)

// ErrEnvironmentConflict is the domain conflict returned when a Plugin
// environment revision already exists for a source run with different entries.
// It aliases the core error so callers above infra classify it without
// importing this package. See docs/design/plugin-space-distribution.md §16.1.
var ErrEnvironmentConflict = coreplugin.ErrEnvironmentConflict

// pluginEnvironmentRow is the plugin_environment table: one immutable Plugin
// environment revision. See docs/design/plugin-space-distribution.md §16 and
// docs/design/task-workspace-checkpoints.md §9.
//
// entries is a JSON document rather than a child table for the reason
// plugin_release.inspection and task_run.plugin_pins are: it is written once,
// read whole, and nothing queries inside it. Uniqueness is source_task_run_id:
// a run commits at most one environment. base_environment_id chains a revision
// to the one it extends, mirroring workspace_checkpoint.base_checkpoint_id.
type pluginEnvironmentRow struct {
	ID                uint64  `gorm:"primaryKey;autoIncrement"`
	PublicID          string  `gorm:"column:public_id;type:char(20) CHARACTER SET ascii COLLATE ascii_bin;uniqueIndex:uq_plugin_environment_public_id;not null"`
	SpaceID           uint64  `gorm:"column:space_id;not null;index"`
	TaskID            uint64  `gorm:"column:task_id;not null;index:idx_plugin_environment_task_created,priority:1"`
	SourceTaskRunID   uint64  `gorm:"column:source_task_run_id;not null;uniqueIndex:uq_plugin_environment_run"`
	BaseEnvironmentID *uint64 `gorm:"column:base_environment_id;index"`
	Entries           string  `gorm:"column:entries;type:text;not null"`

	CreatedAt time.Time `gorm:"autoCreateTime;index:idx_plugin_environment_task_created,priority:2"`
}

func (pluginEnvironmentRow) TableName() string { return "plugin_environment" }

// FinalizePluginEnvironment records one immutable revision and returns it.
//
// It is idempotent by source_task_run_id: a repeat with identical entries
// returns the existing revision; a repeat with different entries is
// ErrEnvironmentConflict and never rewrites the accepted one. It records the
// revision only; linking the source run's result pointer and advancing the
// Task's environment head belong to the capability-handoff orchestration (§16.1)
// so that this store method changes no run or Task behavior on its own.
func (s *Store) FinalizePluginEnvironment(ctx context.Context, in coreplugin.CreateEnvironmentInput) (*coreplugin.PluginEnvironment, error) {
	if len(in.Entries) == 0 {
		return nil, coreplugin.ErrEnvironmentEmpty
	}
	for _, e := range in.Entries {
		if !coreplugin.ValidEntryInstaller(e.Installer) || !coreplugin.ValidEntryScope(e.Scope) {
			return nil, coreplugin.ErrEnvironmentInvalidEntry
		}
	}
	encoded, err := json.Marshal(in.Entries)
	if err != nil {
		return nil, fmt.Errorf("encode entries: %w", err)
	}

	var out *coreplugin.PluginEnvironment
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		spaceKey, err := lookupKey(ctx, tx, "space", in.SpaceID)
		if err != nil {
			return err
		}
		taskKey, err := lookupKey(ctx, tx, "task", in.TaskID)
		if err != nil {
			return err
		}
		runKey, err := lookupKey(ctx, tx, "task_run", in.SourceTaskRunID)
		if err != nil {
			return err
		}
		// A revision can only belong to the Space and Task of its source run.
		var run taskRunRow
		if err := tx.Select("id", "task_id").Where("id = ?", runKey).Take(&run).Error; err != nil {
			return err
		}
		if run.TaskID != taskKey {
			return apierr.ErrNotFound
		}
		var task taskRow
		if err := tx.Select("id", "space_id").Where("id = ?", taskKey).Take(&task).Error; err != nil {
			return err
		}
		if task.SpaceID != spaceKey {
			return apierr.ErrNotFound
		}
		baseKey, err := optionalKey(ctx, tx, "plugin_environment", in.BaseEnvironmentID)
		if err != nil {
			return err
		}

		// Idempotency: an existing revision for this run wins.
		var existing pluginEnvironmentRow
		findErr := tx.Where("source_task_run_id = ?", runKey).Take(&existing).Error
		switch {
		case findErr == nil:
			if existing.Entries != string(encoded) {
				return ErrEnvironmentConflict
			}
			out = toPluginEnvironment(&existing, in.SpaceID, in.TaskID, in.SourceTaskRunID, in.BaseEnvironmentID)
			return nil
		case errors.Is(findErr, gorm.ErrRecordNotFound):
			// fall through to insert
		default:
			return findErr
		}

		publicID, err := util.NewPublicID()
		if err != nil {
			return err
		}
		row := pluginEnvironmentRow{
			PublicID:          publicID,
			SpaceID:           spaceKey,
			TaskID:            taskKey,
			SourceTaskRunID:   runKey,
			BaseEnvironmentID: baseKey,
			Entries:           string(encoded),
		}
		if err := tx.Create(&row).Error; err != nil {
			return err
		}
		out = toPluginEnvironment(&row, in.SpaceID, in.TaskID, in.SourceTaskRunID, in.BaseEnvironmentID)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// GetPluginEnvironment reads one revision by its public handle.
func (s *Store) GetPluginEnvironment(ctx context.Context, publicID string) (*coreplugin.PluginEnvironment, error) {
	id, ok := util.CanonicalPublicID(publicID)
	if !ok {
		return nil, apierr.ErrNotFound
	}
	type readRow struct {
		Row           pluginEnvironmentRow `gorm:"embedded"`
		SpacePublicID string               `gorm:"column:space_public_id"`
		TaskPublicID  string               `gorm:"column:task_public_id"`
		RunPublicID   string               `gorm:"column:run_public_id"`
		BasePublicID  *string              `gorm:"column:base_public_id"`
	}
	var r readRow
	err := s.db.WithContext(ctx).Model(&pluginEnvironmentRow{}).
		Select("plugin_environment.*, s.public_id AS space_public_id, t.public_id AS task_public_id, tr.public_id AS run_public_id, b.public_id AS base_public_id").
		Joins("INNER JOIN space s ON s.id = plugin_environment.space_id").
		Joins("INNER JOIN task t ON t.id = plugin_environment.task_id").
		Joins("INNER JOIN task_run tr ON tr.id = plugin_environment.source_task_run_id").
		Joins("LEFT JOIN plugin_environment b ON b.id = plugin_environment.base_environment_id").
		Where("plugin_environment.public_id = ?", id).Take(&r).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, apierr.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return toPluginEnvironment(&r.Row, r.SpacePublicID, r.TaskPublicID, r.RunPublicID, r.BasePublicID), nil
}

// GetRunPluginEnvironmentBase reads the environment a run materialized, resolved
// through task_run.plugin_environment_base_id, or (nil, nil) when the run has no
// base — the common case, a run that derives its set from the Agent revision and
// Space activation. It mirrors GetRunWorkspaceBase.
func (s *Store) GetRunPluginEnvironmentBase(ctx context.Context, taskRunID string) (*coreplugin.PluginEnvironment, error) {
	runKey, ok := util.CanonicalPublicID(taskRunID)
	if !ok {
		return nil, apierr.ErrNotFound
	}
	type readRow struct {
		Row           pluginEnvironmentRow `gorm:"embedded"`
		SpacePublicID string               `gorm:"column:space_public_id"`
		TaskPublicID  string               `gorm:"column:task_public_id"`
		RunPublicID   string               `gorm:"column:run_public_id"`
		BasePublicID  *string              `gorm:"column:base_public_id"`
	}
	var r readRow
	err := s.db.WithContext(ctx).Model(&taskRunRow{}).
		Select("e.*, s.public_id AS space_public_id, t.public_id AS task_public_id, tr.public_id AS run_public_id, b.public_id AS base_public_id").
		Joins("INNER JOIN plugin_environment e ON e.id = task_run.plugin_environment_base_id").
		Joins("INNER JOIN space s ON s.id = e.space_id").
		Joins("INNER JOIN task t ON t.id = e.task_id").
		Joins("INNER JOIN task_run tr ON tr.id = e.source_task_run_id").
		Joins("LEFT JOIN plugin_environment b ON b.id = e.base_environment_id").
		Where("task_run.public_id = ?", runKey).Take(&r).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return toPluginEnvironment(&r.Row, r.SpacePublicID, r.TaskPublicID, r.RunPublicID, r.BasePublicID), nil
}

func toPluginEnvironment(row *pluginEnvironmentRow, spaceID, taskID, runID string, baseID *string) *coreplugin.PluginEnvironment {
	out := &coreplugin.PluginEnvironment{
		ID:                row.PublicID,
		SpaceID:           spaceID,
		TaskID:            taskID,
		SourceTaskRunID:   runID,
		BaseEnvironmentID: baseID,
		CreatedAt:         row.CreatedAt,
	}
	// A document that will not decode costs the entries, not the revision's
	// identity; the digest of each package still lived in the same bytes.
	if row.Entries != "" {
		_ = json.Unmarshal([]byte(row.Entries), &out.Entries)
	}
	return out
}
