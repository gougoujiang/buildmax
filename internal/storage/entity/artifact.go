package entity

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
)

const chatInputSnippetMaxLen = 200

// CreateArtifactWithItem creates one artifact row (with chat_run_id), one artifact_item row, and updates chat.last_artifact_id in a transaction.
func (s *Store) CreateArtifactWithItem(ctx context.Context, chatID, chatRunID, artifactID string, seq int, relativePath string) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		art := Artifact{
			ChatID:     chatID,
			ChatRunID:  chatRunID,
			ArtifactID: artifactID,
			CreatedAt:  time.Now().Unix(),
			Seq:        seq,
		}
		if err := tx.Create(&art).Error; err != nil {
			return err
		}
		item := ArtifactItem{
			ArtifactID:   artifactID,
			RelativePath: relativePath,
		}
		if err := tx.Create(&item).Error; err != nil {
			return err
		}
		return tx.Model(&Chat{}).Where("chat_id = ?", chatID).Updates(map[string]interface{}{"last_artifact_id": artifactID}).Error
	})
}

// ListArtifactsByWorkspace returns artifacts in the workspace, optionally filtered by chat_id, ordered by created_at DESC. Includes chat_run_id and chat_input_snippet from run input.
func (s *Store) ListArtifactsByWorkspace(ctx context.Context, workspaceID string, chatID *string) ([]ArtifactWithChat, error) {
	q := `SELECT a.artifact_id, a.chat_id, a.chat_run_id, a.created_at, a.seq, c.workspace_id, LEFT(r.input, ?) AS chat_input_snippet
		FROM artifact a
		JOIN chat c ON a.chat_id = c.chat_id
		JOIN chat_run r ON a.chat_run_id = r.chat_run_id
		WHERE c.workspace_id = ?`
	args := []interface{}{chatInputSnippetMaxLen, workspaceID}
	if chatID != nil {
		q += ` AND c.chat_id = ?`
		args = append(args, *chatID)
	}
	q += ` ORDER BY a.created_at DESC`
	var out []ArtifactWithChat
	err := s.db.WithContext(ctx).Raw(q, args...).Scan(&out).Error
	return out, err
}

// GetArtifactByID returns the artifact by artifact_id, or (nil, nil) if not found.
func (s *Store) GetArtifactByID(ctx context.Context, artifactID string) (*Artifact, error) {
	var a Artifact
	err := s.db.WithContext(ctx).Where("artifact_id = ?", artifactID).First(&a).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &a, nil
}

// ListArtifactItems returns all artifact_item rows for the given artifact_id, ordered by id.
func (s *Store) ListArtifactItems(ctx context.Context, artifactID string) ([]ArtifactItem, error) {
	var items []ArtifactItem
	err := s.db.WithContext(ctx).Where("artifact_id = ?", artifactID).Order("id ASC").Find(&items).Error
	return items, err
}
