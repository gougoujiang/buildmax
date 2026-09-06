package db

import (
	"context"
	"errors"
	"time"

	"github.com/gougoujiang/buildmax/internal/core/apierr"
	coretask "github.com/gougoujiang/buildmax/internal/core/task"
	"github.com/gougoujiang/buildmax/internal/util"
	"gorm.io/gorm"
)

// ErrCheckpointConflict is the domain conflict returned when a checkpoint
// already exists for a (source_task_run_id, kind) pair with different bytes. It
// aliases the core error so callers above infra classify it without importing
// this package. See docs/design/task-workspace-checkpoints.md §8.
var ErrCheckpointConflict = coretask.ErrCheckpointConflict

// workspaceCheckpointRow is the workspace_checkpoint table. See
// docs/design/task-workspace-checkpoints.md §9.1.
//
// storage_key is infrastructure data and, like an artifact's, never reaches
// domain JSON, a worker-visible response, a log, or a trace. Uniqueness is
// (source_task_run_id, kind): a run has at most one seed, one successful, and
// one partial checkpoint. payload_sha256 is not unique because separate rows
// can intentionally name the same immutable payload with separate provenance.
type workspaceCheckpointRow struct {
	ID                uint64  `gorm:"primaryKey;autoIncrement"`
	PublicID          string  `gorm:"column:public_id;type:char(20) CHARACTER SET ascii COLLATE ascii_bin;uniqueIndex:uq_workspace_checkpoint_public_id;not null"`
	SpaceID           uint64  `gorm:"column:space_id;not null;index"`
	TaskID            uint64  `gorm:"column:task_id;not null;index:idx_workspace_checkpoint_task_created,priority:1"`
	SourceTaskRunID   uint64  `gorm:"column:source_task_run_id;not null;uniqueIndex:uq_workspace_checkpoint_run_kind,priority:1"`
	Kind              string  `gorm:"column:kind;type:varchar(32);not null;uniqueIndex:uq_workspace_checkpoint_run_kind,priority:2"`
	BaseCheckpointID  *uint64 `gorm:"column:base_checkpoint_id;index"`
	PayloadFormat     string  `gorm:"column:payload_format;type:varchar(32);not null"`
	PayloadSHA256     string  `gorm:"column:payload_sha256;type:char(64) CHARACTER SET ascii COLLATE ascii_bin;not null"`
	StorageKey        string  `gorm:"column:storage_key;type:varchar(1024);not null"`
	SizeBytes         int64   `gorm:"column:size_bytes;not null"`
	UncompressedBytes int64   `gorm:"column:uncompressed_bytes;not null"`
	EntryCount        int64   `gorm:"column:entry_count;not null"`

	CreatedAt time.Time `gorm:"autoCreateTime;index:idx_workspace_checkpoint_task_created,priority:2"`
}

func (workspaceCheckpointRow) TableName() string { return "workspace_checkpoint" }

// FinalizeWorkspaceCheckpoint records a checkpoint and updates its source run
// and, for a head-advancing kind, its Task — atomically. It is the store side
// of the commit protocol's authoritative-pointer step (§8): the payload is
// already durable; this makes the checkpoint a checkpoint.
//
// It is idempotent by (source_task_run_id, kind): a repeat with identical bytes
// returns the existing checkpoint; a repeat with different bytes is
// ErrCheckpointConflict and never rewrites the accepted one. A seed establishes
// the Task head when it has none; a successful result advances an existing head
// only when it still points at the run's base (a linear advance); a partial
// never moves the head.
func (s *Store) FinalizeWorkspaceCheckpoint(ctx context.Context, in coretask.FinalizeCheckpointInput) (*coretask.WorkspaceCheckpoint, error) {
	if !coretask.ValidCheckpointKind(in.Kind) {
		return nil, apierr.ErrNotFound
	}
	var out *coretask.WorkspaceCheckpoint
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
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
		// A checkpoint can only belong to the Space and Task of its source run.
		var run taskRunRow
		if err := tx.Select("id", "task_id", "workspace_base_checkpoint_id").
			Where("id = ?", runKey).Take(&run).Error; err != nil {
			return err
		}
		if run.TaskID != taskKey {
			return apierr.ErrNotFound
		}
		var task taskRow
		if err := tx.Select("id", "space_id", "workspace_head_checkpoint_id").
			Where("id = ?", taskKey).Take(&task).Error; err != nil {
			return err
		}
		if task.SpaceID != spaceKey {
			return apierr.ErrNotFound
		}
		baseKey, err := optionalKey(ctx, tx, "workspace_checkpoint", in.BaseCheckpointID)
		if err != nil {
			return err
		}

		// Idempotency: an existing checkpoint for this run and kind wins.
		var existing workspaceCheckpointRow
		findErr := tx.Where("source_task_run_id = ? AND kind = ?", runKey, string(in.Kind)).
			Take(&existing).Error
		switch {
		case findErr == nil:
			if existing.PayloadSHA256 != in.PayloadSHA256 || existing.PayloadFormat != in.PayloadFormat {
				return ErrCheckpointConflict
			}
			out = toWorkspaceCheckpoint(&existing, in.SpaceID, in.TaskID, in.SourceTaskRunID, in.BaseCheckpointID)
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
		row := workspaceCheckpointRow{
			PublicID:          publicID,
			SpaceID:           spaceKey,
			TaskID:            taskKey,
			SourceTaskRunID:   runKey,
			Kind:              string(in.Kind),
			BaseCheckpointID:  baseKey,
			PayloadFormat:     in.PayloadFormat,
			PayloadSHA256:     in.PayloadSHA256,
			StorageKey:        in.StorageKey,
			SizeBytes:         in.SizeBytes,
			UncompressedBytes: in.UncompressedBytes,
			EntryCount:        in.EntryCount,
		}
		if err := tx.Create(&row).Error; err != nil {
			return err
		}

		if err := linkRunCheckpoint(ctx, tx, runKey, in.Kind, row.ID); err != nil {
			return err
		}
		if err := advanceTaskHead(ctx, tx, taskKey, in.Kind, row.ID, run.WorkspaceBaseCheckpointID, task.WorkspaceHeadCheckpointID); err != nil {
			return err
		}

		out = toWorkspaceCheckpoint(&row, in.SpaceID, in.TaskID, in.SourceTaskRunID, in.BaseCheckpointID)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// linkRunCheckpoint points the source run at the checkpoint it produced and
// records that the workspace checkpoint step committed.
func linkRunCheckpoint(ctx context.Context, tx *gorm.DB, runKey uint64, kind coretask.CheckpointKind, checkpointID uint64) error {
	updates := map[string]any{}
	switch kind {
	case coretask.CheckpointKindSeed:
		updates["workspace_base_checkpoint_id"] = checkpointID
	case coretask.CheckpointKindSuccessful:
		updates["workspace_result_checkpoint_id"] = checkpointID
		updates["workspace_checkpoint_status"] = string(coretask.WorkspaceCheckpointCommitted)
	case coretask.CheckpointKindPartial:
		updates["workspace_partial_checkpoint_id"] = checkpointID
	}
	return tx.WithContext(ctx).Model(&taskRunRow{}).Where("id = ?", runKey).Updates(updates).Error
}

// advanceTaskHead moves the Task's recoverable head under the §9.4 rules. A
// seed establishes it when absent; a successful result advances it only when it
// still points at the run's base; a partial never touches it. The WHERE clauses
// are the compare-and-set: a lost race is a no-op, not an override.
func advanceTaskHead(ctx context.Context, tx *gorm.DB, taskKey uint64, kind coretask.CheckpointKind, checkpointID uint64, runBase *uint64, currentHead *uint64) error {
	if !coretask.AdvancesWorkspaceHead(kind) {
		return nil
	}
	q := tx.WithContext(ctx).Model(&taskRow{}).Where("id = ?", taskKey)
	switch kind {
	case coretask.CheckpointKindSeed:
		// Establish the first head; if one already exists, leave it.
		q = q.Where("workspace_head_checkpoint_id IS NULL")
	case coretask.CheckpointKindSuccessful:
		// Linear advance: only when the head still points where this run began.
		if runBase == nil {
			q = q.Where("workspace_head_checkpoint_id IS NULL")
		} else {
			q = q.Where("workspace_head_checkpoint_id = ?", *runBase)
		}
	}
	// RowsAffected zero means a concurrent writer already moved the head; that
	// is the intended no-op, not an error.
	return q.Update("workspace_head_checkpoint_id", checkpointID).Error
}

// GetWorkspaceCheckpoint reads one checkpoint by its public handle.
func (s *Store) GetWorkspaceCheckpoint(ctx context.Context, publicID string) (*coretask.WorkspaceCheckpoint, error) {
	id, ok := util.CanonicalPublicID(publicID)
	if !ok {
		return nil, apierr.ErrNotFound
	}
	type readRow struct {
		Row           workspaceCheckpointRow `gorm:"embedded"`
		SpacePublicID string                 `gorm:"column:space_public_id"`
		TaskPublicID  string                 `gorm:"column:task_public_id"`
		RunPublicID   string                 `gorm:"column:run_public_id"`
		BasePublicID  *string                `gorm:"column:base_public_id"`
	}
	var r readRow
	err := s.db.WithContext(ctx).Model(&workspaceCheckpointRow{}).
		Select("workspace_checkpoint.*, s.public_id AS space_public_id, t.public_id AS task_public_id, tr.public_id AS run_public_id, b.public_id AS base_public_id").
		Joins("INNER JOIN space s ON s.id = workspace_checkpoint.space_id").
		Joins("INNER JOIN task t ON t.id = workspace_checkpoint.task_id").
		Joins("INNER JOIN task_run tr ON tr.id = workspace_checkpoint.source_task_run_id").
		Joins("LEFT JOIN workspace_checkpoint b ON b.id = workspace_checkpoint.base_checkpoint_id").
		Where("workspace_checkpoint.public_id = ?", id).Take(&r).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, apierr.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return toWorkspaceCheckpoint(&r.Row, r.SpacePublicID, r.TaskPublicID, r.RunPublicID, r.BasePublicID), nil
}

// GetRunWorkspaceBase reads the checkpoint a run is authorized to restore from,
// or (nil, nil) when the run has no base — the first run of a Task, which the
// worker seeds instead. The worker addresses the payload from its own space and
// the returned digest, so no storage key crosses this boundary.
func (s *Store) GetRunWorkspaceBase(ctx context.Context, taskRunID string) (*coretask.WorkspaceCheckpoint, error) {
	runKey, ok := util.CanonicalPublicID(taskRunID)
	if !ok {
		return nil, apierr.ErrNotFound
	}
	type readRow struct {
		Row           workspaceCheckpointRow `gorm:"embedded"`
		SpacePublicID string                 `gorm:"column:space_public_id"`
		TaskPublicID  string                 `gorm:"column:task_public_id"`
		RunPublicID   string                 `gorm:"column:run_public_id"`
		BasePublicID  *string                `gorm:"column:base_public_id"`
	}
	var r readRow
	err := s.db.WithContext(ctx).Model(&taskRunRow{}).
		Select("c.*, s.public_id AS space_public_id, t.public_id AS task_public_id, tr.public_id AS run_public_id, b.public_id AS base_public_id").
		Joins("INNER JOIN workspace_checkpoint c ON c.id = task_run.workspace_base_checkpoint_id").
		Joins("INNER JOIN space s ON s.id = c.space_id").
		Joins("INNER JOIN task t ON t.id = c.task_id").
		Joins("INNER JOIN task_run tr ON tr.id = c.source_task_run_id").
		Joins("LEFT JOIN workspace_checkpoint b ON b.id = c.base_checkpoint_id").
		Where("task_run.public_id = ?", runKey).Take(&r).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		// Either the run does not exist or it has no base. The caller cannot act
		// differently on the two, and the worker's own run token already proved
		// the run exists, so a nil base is the honest answer.
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return toWorkspaceCheckpoint(&r.Row, r.SpacePublicID, r.TaskPublicID, r.RunPublicID, r.BasePublicID), nil
}

// RecordWorkspaceRestore records how a run's base restoration ended. errMessage
// is bounded operator-facing text, stored only for a failure.
func (s *Store) RecordWorkspaceRestore(ctx context.Context, taskRunID string, status coretask.WorkspaceRestoreStatus, errMessage *string) error {
	runKey, ok := util.CanonicalPublicID(taskRunID)
	if !ok {
		return apierr.ErrNotFound
	}
	res := s.db.WithContext(ctx).Model(&taskRunRow{}).
		Where("public_id = ?", runKey).
		Updates(map[string]any{
			"workspace_restore_status": string(status),
			"workspace_restore_error":  errMessage,
		})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return apierr.ErrNotFound
	}
	return nil
}

// ReferencedCheckpointStorageKeys returns the set of every storage_key a
// workspace_checkpoint row names. The orphan sweep lists the payload store and
// deletes any blob this set does not contain: a payload with no row is an
// orphan, never a checkpoint (§8, §12.4). It is a set because payload_sha256 is
// intentionally non-unique — several rows may name the same immutable payload.
func (s *Store) ReferencedCheckpointStorageKeys(ctx context.Context) (map[string]struct{}, error) {
	var keys []string
	if err := s.db.WithContext(ctx).Model(&workspaceCheckpointRow{}).
		Distinct("storage_key").
		Pluck("storage_key", &keys).Error; err != nil {
		return nil, err
	}
	set := make(map[string]struct{}, len(keys))
	for _, k := range keys {
		set[k] = struct{}{}
	}
	return set, nil
}

func toWorkspaceCheckpoint(row *workspaceCheckpointRow, spaceID, taskID, runID string, baseID *string) *coretask.WorkspaceCheckpoint {
	return &coretask.WorkspaceCheckpoint{
		ID:                row.PublicID,
		SpaceID:           spaceID,
		TaskID:            taskID,
		SourceTaskRunID:   runID,
		BaseCheckpointID:  baseID,
		Kind:              coretask.CheckpointKind(row.Kind),
		PayloadFormat:     row.PayloadFormat,
		PayloadSHA256:     row.PayloadSHA256,
		SizeBytes:         row.SizeBytes,
		UncompressedBytes: row.UncompressedBytes,
		EntryCount:        row.EntryCount,
		CreatedAt:         row.CreatedAt,
	}
}
