package entity

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
)

const taskInputSnippetMaxLen = 200

// CreateArtifactWithItem creates one artifact row, one artifact_item row, and updates task.last_artifact_id in a transaction.
func (s *Store) CreateArtifactWithItem(ctx context.Context, taskID, artifactID string, seq int, relativePath string) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		art := Artifact{
			TaskID:     taskID,
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
		return tx.Model(&Task{}).Where("task_id = ?", taskID).Update("last_artifact_id", artifactID).Error
	})
}

// ListArtifactsByWorkspace returns artifacts in the workspace, optionally filtered by task_id and project_id, ordered by created_at DESC. Task_input_snippet is truncated to 200 chars.
func (s *Store) ListArtifactsByWorkspace(ctx context.Context, workspaceID string, taskID, projectID *string) ([]ArtifactWithTask, error) {
	q := `SELECT a.artifact_id, a.task_id, a.created_at, a.seq, t.workspace_id, t.project_id, LEFT(t.input, ?) AS task_input_snippet FROM artifact a JOIN task t ON a.task_id = t.task_id WHERE t.workspace_id = ?`
	args := []interface{}{taskInputSnippetMaxLen, workspaceID}
	if taskID != nil {
		q += ` AND t.task_id = ?`
		args = append(args, *taskID)
	}
	if projectID != nil {
		q += ` AND t.project_id = ?`
		args = append(args, *projectID)
	}
	q += ` ORDER BY a.created_at DESC`
	var out []ArtifactWithTask
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
