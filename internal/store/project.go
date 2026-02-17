package store

import (
	"context"
	"errors"
	"time"

	"buildmax/internal/util"
	"gorm.io/gorm"
)

// GetProject returns the project by project_id, or (nil, nil) when not found.
func (s *Store) GetProject(ctx context.Context, projectID string) (*Project, error) {
	var p Project
	err := s.db.WithContext(ctx).Where("project_id = ?", projectID).First(&p).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &p, nil
}

// ListProjectsByWorkspace returns all projects for the given workspace_id, ordered by created_at.
func (s *Store) ListProjectsByWorkspace(ctx context.Context, workspaceID string) ([]Project, error) {
	var list []Project
	err := s.db.WithContext(ctx).Where("workspace_id = ?", workspaceID).Order("created_at ASC").Find(&list).Error
	return list, err
}

// CreateProject inserts a new project and returns it.
func (s *Store) CreateProject(ctx context.Context, workspaceID, name, description string) (*Project, error) {
	p := &Project{
		ProjectID:   util.NewID(),
		WorkspaceID: workspaceID,
		Name:        name,
		Description: description,
		CreatedAt:   time.Now().Unix(),
	}
	if err := s.db.WithContext(ctx).Create(p).Error; err != nil {
		return nil, err
	}
	return p, nil
}
