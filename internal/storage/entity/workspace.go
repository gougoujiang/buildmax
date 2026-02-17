package entity

import (
	"context"
	"time"

	"buildmax/internal/util"
)

// EnsureDefaultWorkspaceForUser creates a "Default" workspace for the user if they have none.
// userID is the user's user_id (UUID).
func (s *Store) EnsureDefaultWorkspaceForUser(ctx context.Context, userID string) error {
	var count int64
	if err := s.db.WithContext(ctx).Model(&Workspace{}).Where("owner_user_id = ?", userID).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	w := Workspace{
		WorkspaceID: util.NewID(),
		OwnerUserID: userID,
		Name:        "Default",
		CreatedAt:   time.Now().Unix(),
	}
	return s.db.WithContext(ctx).Create(&w).Error
}

// ListWorkspacesByOwner returns all workspaces for the given owner (user_id), ordered by created_at.
func (s *Store) ListWorkspacesByOwner(ctx context.Context, userID string) ([]Workspace, error) {
	var list []Workspace
	err := s.db.WithContext(ctx).Where("owner_user_id = ?", userID).Order("created_at ASC").Find(&list).Error
	return list, err
}

// WorkspaceBelongsToUser returns true if the workspace exists and is owned by the user.
func (s *Store) WorkspaceBelongsToUser(ctx context.Context, workspaceID, userID string) (bool, error) {
	var count int64
	err := s.db.WithContext(ctx).Model(&Workspace{}).Where("workspace_id = ? AND owner_user_id = ?", workspaceID, userID).Count(&count).Error
	return count > 0, err
}

// CreateWorkspace creates a new workspace for the user and returns it.
func (s *Store) CreateWorkspace(ctx context.Context, userID, name string) (*Workspace, error) {
	w := &Workspace{
		WorkspaceID: util.NewID(),
		OwnerUserID: userID,
		Name:        name,
		CreatedAt:   time.Now().Unix(),
	}
	if err := s.db.WithContext(ctx).Create(w).Error; err != nil {
		return nil, err
	}
	return w, nil
}
