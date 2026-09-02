package db

import (
	"context"
	"errors"
	"time"

	"github.com/gougoujiang/buildmax/internal/core/apierr"
	coretask "github.com/gougoujiang/buildmax/internal/core/task"
	"github.com/gougoujiang/buildmax/internal/util"
)

const chatInputSnippetMaxLen = 200

// ListRunOutputsByConversation returns run outputs (artifacts) in the conversation, optionally filtered by task_id.
// Order: run created_at DESC. ArtifactID in the result is task_run_id for API compatibility.
func (s *Store) ListRunOutputsByConversation(ctx context.Context, conversationID string, taskID *string) ([]coretask.RunOutputListing, error) {
	convID, ok := util.CanonicalPublicID(conversationID)
	if !ok {
		return nil, nil
	}
	// Every join is on a row key; the handles the caller gets back are selected
	// from the joined rows rather than resolved one output at a time.
	q := `SELECT r.public_id AS artifact_id, c.public_id AS task_id, r.public_id AS task_run_id,
			v.public_id AS conversation_id, u.public_id AS user_id, r.created_at,
			LEFT(r.input, ?) AS task_input_snippet
		FROM task_run_artifact o
		JOIN task_run r ON o.task_run_id = r.id
		JOIN task c ON r.task_id = c.id
		JOIN conversation v ON c.conversation_id = v.id
		JOIN ` + "`user`" + ` u ON u.id = v.user_id
		WHERE v.public_id = ? AND r.status = 'SUCCEEDED'`
	args := []interface{}{chatInputSnippetMaxLen, convID}
	if taskID != nil {
		tid, ok := util.CanonicalPublicID(*taskID)
		if !ok {
			return nil, nil
		}
		q += ` AND c.public_id = ?`
		args = append(args, tid)
	}
	q += ` GROUP BY r.public_id, c.public_id, v.public_id, u.public_id, r.created_at, r.input ORDER BY r.created_at DESC`
	var rows []struct {
		ArtifactID       string
		TaskID           string
		TaskRunID        string
		ConversationID   string
		UserID           string
		CreatedAt        time.Time
		TaskInputSnippet string
	}
	if err := s.db.WithContext(ctx).Raw(q, args...).Scan(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]coretask.RunOutputListing, 0, len(rows))
	for _, r := range rows {
		out = append(out, coretask.RunOutputListing{
			ArtifactID:       r.ArtifactID,
			TaskID:           r.TaskID,
			TaskRunID:        r.TaskRunID,
			ConversationID:   r.ConversationID,
			UserID:           r.UserID,
			CreatedAt:        r.CreatedAt,
			TaskInputSnippet: r.TaskInputSnippet,
		})
	}
	return out, nil
}

// ListRunOutputsByTask returns every succeeded run output for one task. Task
// identity, rather than an optional conversation projection, is the scope.
func (s *Store) ListRunOutputsByTask(ctx context.Context, taskID string) ([]coretask.RunOutputListing, error) {
	tid, ok := util.CanonicalPublicID(taskID)
	if !ok {
		return nil, nil
	}
	q := `SELECT r.public_id AS artifact_id, t.public_id AS task_id, r.public_id AS task_run_id,
			c.public_id AS conversation_id, tm.public_id AS team_id, u.public_id AS user_id,
			r.created_at, LEFT(r.input, ?) AS task_input_snippet
		FROM task_run_artifact o
		JOIN task_run r ON o.task_run_id = r.id
		JOIN task t ON r.task_id = t.id
		JOIN team tm ON t.team_id = tm.id
		JOIN ` + "`user`" + ` u ON u.id = t.created_by
		LEFT JOIN conversation c ON t.conversation_id = c.id
		WHERE t.public_id = ? AND r.status = 'SUCCEEDED'
		GROUP BY r.public_id, t.public_id, c.public_id, tm.public_id, u.public_id, r.created_at, r.input
		ORDER BY r.created_at DESC`
	return s.scanRunOutputListings(ctx, q, chatInputSnippetMaxLen, tid)
}

func (s *Store) scanRunOutputListings(ctx context.Context, q string, args ...interface{}) ([]coretask.RunOutputListing, error) {
	var rows []struct {
		ArtifactID       string
		TaskID           string
		TaskRunID        string
		ConversationID   string
		TeamID           string
		UserID           string
		CreatedAt        time.Time
		TaskInputSnippet string
	}
	if err := s.db.WithContext(ctx).Raw(q, args...).Scan(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]coretask.RunOutputListing, 0, len(rows))
	for _, r := range rows {
		out = append(out, coretask.RunOutputListing{
			ArtifactID: r.ArtifactID, TaskID: r.TaskID, TaskRunID: r.TaskRunID,
			ConversationID: r.ConversationID, TeamID: r.TeamID, UserID: r.UserID,
			CreatedAt: r.CreatedAt, TaskInputSnippet: r.TaskInputSnippet,
		})
	}
	return out, nil
}

// GetTaskRunOutputFiles returns all artifact rows for the given task_run_id, ordered by relative_path.
func (s *Store) GetTaskRunOutputFiles(ctx context.Context, taskRunID string) ([]coretask.RunOutputFile, error) {
	runKey, err := lookupKey(ctx, s.db, "task_run", taskRunID)
	if errors.Is(err, apierr.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var items []taskRunArtifactRow
	err = s.db.WithContext(ctx).Where("task_run_id = ?", runKey).Order("relative_path ASC").Find(&items).Error
	return toTaskRunArtifacts(items), err
}

// TaskRunHasOutput returns true if the run has at least one output file (and thus is a valid "artifact" for content).
func (s *Store) TaskRunHasOutput(ctx context.Context, taskRunID string) (bool, error) {
	runKey, err := lookupKey(ctx, s.db, "task_run", taskRunID)
	if errors.Is(err, apierr.ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	var n int64
	err = s.db.WithContext(ctx).Model(&taskRunArtifactRow{}).Where("task_run_id = ?", runKey).Limit(1).Count(&n).Error
	if err != nil {
		return false, err
	}
	return n > 0, nil
}
