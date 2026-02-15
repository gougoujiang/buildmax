// Package store provides user and workspace persistence (MySQL via GORM).
package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

// newUUID returns a new UUID string for use as a public entity id.
func newUUID() string {
	return uuid.New().String()
}

// User is the user model. JSON uses snake_case per project convention.
// Internal id is for DB only; API and JWT use user_id (UUID).
type User struct {
	ID        uint   `gorm:"primaryKey;autoIncrement" json:"-"`
	UserID    string `gorm:"type:varchar(36);uniqueIndex;not null" json:"user_id"`
	Email     string `gorm:"type:varchar(255);uniqueIndex;not null" json:"email"`
	Name      string `gorm:"type:varchar(255)" json:"name"`
	CreatedAt int64  `gorm:"autoCreateTime" json:"created_at"`
}

// TableName returns the table name for GORM (singular per project convention).
func (User) TableName() string {
	return "user"
}

// Workspace is the workspace model. JSON uses snake_case per project convention.
// Internal id is for DB only; API uses workspace_id (UUID).
type Workspace struct {
	ID           uint   `gorm:"primaryKey;autoIncrement" json:"-"`
	WorkspaceID  string `gorm:"type:varchar(36);uniqueIndex;not null" json:"workspace_id"`
	OwnerUserID  string `gorm:"type:varchar(36);not null;index" json:"owner_user_id"`
	Name         string `gorm:"type:varchar(255);not null" json:"name"`
	CreatedAt    int64  `gorm:"autoCreateTime" json:"created_at"`
}

// TableName returns the table name for GORM (singular per project convention).
func (Workspace) TableName() string {
	return "workspace"
}

// Project is the project model. JSON uses snake_case per project convention.
// Internal id is for DB only; API uses project_id (UUID).
type Project struct {
	ID          uint   `gorm:"primaryKey;autoIncrement" json:"-"`
	ProjectID   string `gorm:"type:varchar(36);uniqueIndex;not null" json:"project_id"`
	WorkspaceID string `gorm:"type:varchar(36);not null;index" json:"workspace_id"`
	Name        string `gorm:"type:varchar(255);not null" json:"name"`
	Description string `gorm:"type:text" json:"description"`
	CreatedAt   int64  `gorm:"autoCreateTime" json:"created_at"`
}

// TableName returns the table name for GORM (singular per project convention).
func (Project) TableName() string {
	return "project"
}

// Task is the task model. JSON uses snake_case per project convention.
// API exposes task_id as "id"; internal ID is for DB only.
type Task struct {
	ID           uint    `gorm:"primaryKey;autoIncrement" json:"-"` // internal only, not in API
	TaskID       string  `gorm:"type:varchar(36);uniqueIndex;not null" json:"task_id"`
	ProjectID    string  `gorm:"type:varchar(36);not null;index" json:"project_id"`
	Status       string  `gorm:"type:varchar(32);not null" json:"status"`
	Input        string  `gorm:"type:text;not null" json:"input"`
	Output       *string `gorm:"type:text" json:"output,omitempty"`
	CreatedBy    string  `gorm:"type:varchar(36);not null" json:"created_by"`
	CreatedAt    int64   `gorm:"autoCreateTime" json:"created_at"`
	StartedAt    *int64  `gorm:"" json:"started_at,omitempty"`
	EndedAt      *int64  `gorm:"" json:"ended_at,omitempty"`
	ErrorMessage *string `gorm:"type:text" json:"error_message,omitempty"`
}

// TableName returns the table name for GORM (singular per project convention).
func (Task) TableName() string {
	return "task"
}

// UserStore looks up users by email.
type UserStore interface {
	UserByEmail(ctx context.Context, email string) (*User, error)
}

// WorkspaceStore provides workspace persistence.
type WorkspaceStore interface {
	EnsureDefaultWorkspaceForUser(ctx context.Context, userID string) error
	ListWorkspacesByOwner(ctx context.Context, userID string) ([]Workspace, error)
	// WorkspaceBelongsToUser returns true if the workspace is owned by the user.
	WorkspaceBelongsToUser(ctx context.Context, workspaceID, userID string) (bool, error)
}

// ProjectStore provides project persistence.
type ProjectStore interface {
	GetProject(ctx context.Context, projectID string) (*Project, error)
	ListProjectsByWorkspace(ctx context.Context, workspaceID string) ([]Project, error)
	CreateProject(ctx context.Context, workspaceID, name, description string) (*Project, error)
}

// TaskStore provides task persistence.
type TaskStore interface {
	ListTasksByProject(ctx context.Context, projectID string) ([]Task, error)
	CreateTask(ctx context.Context, projectID, input, createdBy string) (*Task, error)
}

// Store implements UserStore, WorkspaceStore, ProjectStore, and TaskStore with a MySQL backend.
type Store struct {
	db *gorm.DB
}

// New opens a MySQL connection with the given DSN and runs AutoMigrate for User, Workspace, Project, and Task.
// The context can be used for connection timeout; the returned Store holds the DB for the process lifetime.
// Table names are singular (user, workspace, project, task). Existing DBs with plural tables require a one-time migration.
func New(ctx context.Context, dsn string) (*Store, error) {
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		return nil, fmt.Errorf("open mysql: %w", err)
	}
	if err := db.WithContext(ctx).AutoMigrate(&User{}, &Workspace{}, &Project{}, &Task{}); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return &Store{db: db}, nil
}

// UserByEmail returns the user with the given email, or (nil, nil) when not found.
func (s *Store) UserByEmail(ctx context.Context, email string) (*User, error) {
	var u User
	err := s.db.WithContext(ctx).Where("email = ?", email).First(&u).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &u, nil
}

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
		WorkspaceID: newUUID(),
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
		ProjectID:   newUUID(),
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

// ListTasksByProject returns all tasks for the given project_id, ordered by created_at.
func (s *Store) ListTasksByProject(ctx context.Context, projectID string) ([]Task, error) {
	var list []Task
	err := s.db.WithContext(ctx).Where("project_id = ?", projectID).Order("created_at ASC").Find(&list).Error
	return list, err
}

// CreateTask inserts a new task with status PENDING and returns it.
func (s *Store) CreateTask(ctx context.Context, projectID, input, createdBy string) (*Task, error) {
	t := &Task{
		TaskID:    newUUID(),
		ProjectID: projectID,
		Status:    "PENDING",
		Input:     input,
		CreatedBy: createdBy,
		CreatedAt: time.Now().Unix(),
	}
	if err := s.db.WithContext(ctx).Create(t).Error; err != nil {
		return nil, err
	}
	return t, nil
}

// Close closes the underlying DB connection. Optional for server lifecycle.
func (s *Store) Close() error {
	sqlDB, err := s.db.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}
