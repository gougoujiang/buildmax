package db

import (
	"context"
	"errors"
	"time"

	"github.com/gougoujiang/buildmax/internal/core/model"
	"github.com/gougoujiang/buildmax/internal/util"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// taskResultDeliveryRow is one owed report.
//
// No public handle: nothing addresses a delivery from outside. It is machinery
// the server owes itself, keyed by the run it reports.
type taskResultDeliveryRow struct {
	ID             uint64  `gorm:"primaryKey;autoIncrement"`
	TaskRunID      uint64  `gorm:"column:task_run_id;not null;uniqueIndex:uq_task_result_delivery_run"`
	ConversationID uint64  `gorm:"column:conversation_id;not null"`
	Status         string  `gorm:"type:varchar(16);not null;index:idx_task_result_delivery_due,priority:1"`
	Attempts       int     `gorm:"not null"`
	LastError      *string `gorm:"type:text"`
	NextAttemptAt  int64   `gorm:"column:next_attempt_at;not null;index:idx_task_result_delivery_due,priority:2"`
	CreatedAt      int64   `gorm:"autoCreateTime"`
	UpdatedAt      int64   `gorm:"autoUpdateTime"`
}

func (taskResultDeliveryRow) TableName() string { return "task_result_delivery" }

// taskResultDeliveryReadRow is the row plus the handles its references resolve to.
type taskResultDeliveryReadRow struct {
	Row                  taskResultDeliveryRow `gorm:"embedded"`
	TaskRunPublicID      string                `gorm:"column:task_run_public_id"`
	ConversationPublicID string                `gorm:"column:conversation_public_id"`
}

func (s *Store) taskResultDeliverySelect(ctx context.Context) *gorm.DB {
	return s.db.WithContext(ctx).Model(&taskResultDeliveryRow{}).
		Select("task_result_delivery.*, r.public_id AS task_run_public_id, c.public_id AS conversation_public_id").
		Joins("INNER JOIN task_run r ON r.id = task_result_delivery.task_run_id").
		Joins("INNER JOIN conversation c ON c.id = task_result_delivery.conversation_id")
}

func toTaskResultDelivery(row *taskResultDeliveryReadRow) model.TaskResultDelivery {
	return model.TaskResultDelivery{
		TaskRunID:      row.TaskRunPublicID,
		ConversationID: row.ConversationPublicID,
		Status:         row.Row.Status,
		Attempts:       row.Row.Attempts,
		LastError:      row.Row.LastError,
		NextAttemptAt:  row.Row.NextAttemptAt,
		CreatedAt:      row.Row.CreatedAt,
	}
}

// EnqueueTaskResultDelivery records that a run's outcome is owed.
//
// Idempotent per run: the unique index does the deduplicating, and a conflict
// is left alone rather than resetting a delivery that may already have been
// attempted or finished.
func (s *Store) EnqueueTaskResultDelivery(ctx context.Context, taskRunID, conversationID string, now int64) error {
	runKey, err := lookupKey(ctx, s.db, "task_run", taskRunID)
	if err != nil {
		return err
	}
	convKey, err := lookupKey(ctx, s.db, "conversation", conversationID)
	if err != nil {
		return err
	}
	row := &taskResultDeliveryRow{
		TaskRunID:      runKey,
		ConversationID: convKey,
		Status:         model.DeliveryPending,
		NextAttemptAt:  now,
		CreatedAt:      now,
	}
	return s.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(row).Error
}

// ListDueTaskResultDeliveries returns pending reports whose next attempt is due.
func (s *Store) ListDueTaskResultDeliveries(ctx context.Context, now int64, limit int) ([]model.TaskResultDelivery, error) {
	if limit <= 0 {
		return nil, nil
	}
	var rows []taskResultDeliveryReadRow
	err := s.taskResultDeliverySelect(ctx).
		Where("task_result_delivery.status = ? AND task_result_delivery.next_attempt_at <= ?", model.DeliveryPending, now).
		Order("task_result_delivery.next_attempt_at ASC").
		Limit(limit).
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make([]model.TaskResultDelivery, len(rows))
	for i := range rows {
		out[i] = toTaskResultDelivery(&rows[i])
	}
	return out, nil
}

// ClaimTaskResultDelivery takes one due delivery.
//
// The status and due-time conditions are in the UPDATE rather than checked
// first: two servers sweeping at once both see the same due row, and only the
// one whose update matched may report it.
func (s *Store) ClaimTaskResultDelivery(ctx context.Context, taskRunID string, now, nextAttemptAt int64) (*model.TaskResultDelivery, error) {
	runKey, err := lookupKey(ctx, s.db, "task_run", taskRunID)
	if err != nil {
		return nil, err
	}
	res := s.db.WithContext(ctx).Model(&taskResultDeliveryRow{}).
		Where("task_run_id = ? AND status = ? AND next_attempt_at <= ?", runKey, model.DeliveryPending, now).
		Updates(map[string]interface{}{
			"attempts":        gorm.Expr("attempts + 1"),
			"next_attempt_at": nextAttemptAt,
			"updated_at":      time.Now().Unix(),
		})
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected == 0 {
		return nil, nil
	}
	var row taskResultDeliveryReadRow
	if err := s.taskResultDeliverySelect(ctx).
		Where("task_result_delivery.task_run_id = ?", runKey).Take(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	claimed := toTaskResultDelivery(&row)
	return &claimed, nil
}

// FinishTaskResultDelivery closes a delivery. Both terminal statuses are final:
// nothing reopens one.
func (s *Store) FinishTaskResultDelivery(ctx context.Context, taskRunID, status string, lastError *string) error {
	runKey, err := lookupKey(ctx, s.db, "task_run", taskRunID)
	if err != nil {
		return err
	}
	updates := map[string]interface{}{"status": status, "updated_at": time.Now().Unix()}
	if lastError != nil {
		updates["last_error"] = util.TruncateRunes(*lastError, deliveryErrorMaxLen)
	}
	return s.db.WithContext(ctx).Model(&taskResultDeliveryRow{}).
		Where("task_run_id = ?", runKey).Updates(updates).Error
}

// deliveryErrorMaxLen bounds a stored failure reason. A provider error can carry
// a whole response body, and this column exists to say why, not to archive it.
const deliveryErrorMaxLen = 1000

// RecordTaskResultDeliveryFailure keeps a delivery pending and says why the last
// attempt did not succeed.
//
// It also brings the next attempt forward. The claim pushed that time out far
// enough to cover a turn still running; once the attempt has failed, waiting out
// the rest of that window would delay a retry for no reason.
func (s *Store) RecordTaskResultDeliveryFailure(ctx context.Context, taskRunID, lastError string, nextAttemptAt int64) error {
	runKey, err := lookupKey(ctx, s.db, "task_run", taskRunID)
	if err != nil {
		return err
	}
	return s.db.WithContext(ctx).Model(&taskResultDeliveryRow{}).
		Where("task_run_id = ? AND status = ?", runKey, model.DeliveryPending).
		Updates(map[string]interface{}{
			"last_error":      util.TruncateRunes(lastError, deliveryErrorMaxLen),
			"next_attempt_at": nextAttemptAt,
			"updated_at":      time.Now().Unix(),
		}).Error
}
