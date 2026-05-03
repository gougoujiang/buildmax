package db

import (
	"buildmax/internal/core/model"
	"context"
)

const chatInputSnippetMaxLen = 200

// ListRunOutputsByConversation returns run outputs (artifacts) in the conversation, optionally filtered by task_id.
// Order: run created_at DESC. ArtifactID in the result is task_run_id for API compatibility.
func (s *Store) ListRunOutputsByConversation(ctx context.Context, conversationID string, taskID *string) ([]model.ArtifactWithTask, error) {
	q := `SELECT r.task_run_id AS artifact_id, r.task_id, r.task_run_id, c.conversation_id, v.user_id, r.created_at, LEFT(r.input, ?) AS task_input_snippet
		FROM task_run_artifact o
		JOIN task_run r ON o.task_run_id = r.task_run_id
		JOIN task c ON r.task_id = c.task_id
		JOIN conversation v ON c.conversation_id = v.conversation_id
		WHERE c.conversation_id = ? AND r.status = 'SUCCEEDED'`
	args := []interface{}{chatInputSnippetMaxLen, conversationID}
	if taskID != nil {
		q += ` AND c.task_id = ?`
		args = append(args, *taskID)
	}
	q += ` GROUP BY r.task_run_id, r.task_id, c.conversation_id, v.user_id, r.created_at, r.input ORDER BY r.created_at DESC`
	var out []model.ArtifactWithTask
	err := s.db.WithContext(ctx).Raw(q, args...).Scan(&out).Error
	return out, err
}

// GetTaskRunOutputFiles returns all artifact rows for the given task_run_id, ordered by relative_path.
func (s *Store) GetTaskRunOutputFiles(ctx context.Context, taskRunID string) ([]model.TaskRunArtifact, error) {
	var items []taskRunArtifactRow
	err := s.db.WithContext(ctx).Where("task_run_id = ?", taskRunID).Order("relative_path ASC").Find(&items).Error
	return toTaskRunArtifacts(items), err
}

// TaskRunHasOutput returns true if the run has at least one output file (and thus is a valid "artifact" for content).
func (s *Store) TaskRunHasOutput(ctx context.Context, taskRunID string) (bool, error) {
	var n int64
	err := s.db.WithContext(ctx).Model(&taskRunArtifactRow{}).Where("task_run_id = ?", taskRunID).Limit(1).Count(&n).Error
	if err != nil {
		return false, err
	}
	return n > 0, nil
}
