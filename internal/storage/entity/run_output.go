package entity

import (
	"context"
)

const chatInputSnippetMaxLen = 200

// ListRunOutputsByWorkspace returns run outputs (artifacts) in the workspace, optionally filtered by chat_id.
// Order: run created_at DESC. ArtifactID in the result is chat_run_id for API compatibility.
func (s *Store) ListRunOutputsByWorkspace(ctx context.Context, workspaceID string, chatID *string) ([]ArtifactWithChat, error) {
	q := `SELECT r.chat_run_id AS artifact_id, r.chat_id, r.chat_run_id, c.workspace_id, r.created_at, LEFT(r.input, ?) AS chat_input_snippet
		FROM chat_run_artifact o
		JOIN chat_run r ON o.chat_run_id = r.chat_run_id
		JOIN chat c ON r.chat_id = c.chat_id
		WHERE c.workspace_id = ? AND r.status = 'SUCCEEDED'`
	args := []interface{}{chatInputSnippetMaxLen, workspaceID}
	if chatID != nil {
		q += ` AND c.chat_id = ?`
		args = append(args, *chatID)
	}
	q += ` GROUP BY r.chat_run_id, r.chat_id, c.workspace_id, r.created_at, r.input ORDER BY r.created_at DESC`
	var out []ArtifactWithChat
	err := s.db.WithContext(ctx).Raw(q, args...).Scan(&out).Error
	return out, err
}

// GetChatRunOutputFiles returns all artifact rows for the given chat_run_id, ordered by relative_path.
func (s *Store) GetChatRunOutputFiles(ctx context.Context, chatRunID string) ([]ChatRunArtifact, error) {
	var items []ChatRunArtifact
	err := s.db.WithContext(ctx).Where("chat_run_id = ?", chatRunID).Order("relative_path ASC").Find(&items).Error
	return items, err
}

// ChatRunHasOutput returns true if the run has at least one output file (and thus is a valid "artifact" for content).
func (s *Store) ChatRunHasOutput(ctx context.Context, chatRunID string) (bool, error) {
	var n int64
	err := s.db.WithContext(ctx).Model(&ChatRunArtifact{}).Where("chat_run_id = ?", chatRunID).Limit(1).Count(&n).Error
	if err != nil {
		return false, err
	}
	return n > 0, nil
}
