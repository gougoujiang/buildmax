// Package store provides user and workspace persistence (MySQL via GORM).
package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"buildmax/internal/id"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

// User is the user model. JSON uses snake_case per project convention.
// Internal id is for DB only; API and JWT use user_id.
type User struct {
	ID        uint   `gorm:"primaryKey;autoIncrement" json:"-"`
	UserID    string `gorm:"type:varchar(64);uniqueIndex;not null" json:"user_id"`
	Email     string `gorm:"type:varchar(255);uniqueIndex;not null" json:"email"`
	Name      string `gorm:"type:varchar(255)" json:"name"`
	CreatedAt int64  `gorm:"autoCreateTime" json:"created_at"`
}

// TableName returns the table name for GORM (singular per project convention).
func (User) TableName() string {
	return "user"
}

// Workspace is the workspace model. JSON uses snake_case per project convention.
// Internal id is for DB only; API uses workspace_id.
type Workspace struct {
	ID           uint   `gorm:"primaryKey;autoIncrement" json:"-"`
	WorkspaceID  string `gorm:"type:varchar(64);uniqueIndex;not null" json:"workspace_id"`
	OwnerUserID  string `gorm:"type:varchar(64);not null;index" json:"owner_user_id"`
	Name         string `gorm:"type:varchar(255);not null" json:"name"`
	CreatedAt    int64  `gorm:"autoCreateTime" json:"created_at"`
}

// TableName returns the table name for GORM (singular per project convention).
func (Workspace) TableName() string {
	return "workspace"
}

// Project is the project model. JSON uses snake_case per project convention.
// Internal id is for DB only; API uses project_id.
type Project struct {
	ID          uint   `gorm:"primaryKey;autoIncrement" json:"-"`
	ProjectID   string `gorm:"type:varchar(64);uniqueIndex;not null" json:"project_id"`
	WorkspaceID string `gorm:"type:varchar(64);not null;index" json:"workspace_id"`
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
// Tasks belong to a workspace; project is optional.
type Task struct {
	ID           uint    `gorm:"primaryKey;autoIncrement" json:"-"` // internal only, not in API
	TaskID       string  `gorm:"type:varchar(64);uniqueIndex;not null" json:"task_id"`
	WorkspaceID  string  `gorm:"type:varchar(64);not null;index" json:"workspace_id"`
	ProjectID    *string `gorm:"type:varchar(64);index" json:"project_id,omitempty"`
	Status       string  `gorm:"type:varchar(32);not null" json:"status"`
	Input        string  `gorm:"type:text;not null" json:"input"`
	Output       *string `gorm:"type:text" json:"output,omitempty"`
	CreatedBy    string  `gorm:"type:varchar(64);not null" json:"created_by"`
	CreatedAt    int64   `gorm:"autoCreateTime" json:"created_at"`
	StartedAt    *int64  `gorm:"" json:"started_at,omitempty"`
	EndedAt      *int64  `gorm:"" json:"ended_at,omitempty"`
	ErrorMessage *string `gorm:"type:text" json:"error_message,omitempty"`
	SessionID    *string `gorm:"type:varchar(36)" json:"session_id,omitempty"`
}

// TableName returns the table name for GORM (singular per project convention).
func (Task) TableName() string {
	return "task"
}

// ErrEmailExists is returned by CreateUser when the email is already registered.
var ErrEmailExists = errors.New("email already exists")

// UserStore looks up users by email and creates new users.
type UserStore interface {
	UserByEmail(ctx context.Context, email string) (*User, error)
	// CreateUser creates a user with the given email. Returns ErrEmailExists if the email is already registered.
	CreateUser(ctx context.Context, email string) (*User, error)
}

// WorkspaceStore provides workspace persistence.
type WorkspaceStore interface {
	EnsureDefaultWorkspaceForUser(ctx context.Context, userID string) error
	ListWorkspacesByOwner(ctx context.Context, userID string) ([]Workspace, error)
	// WorkspaceBelongsToUser returns true if the workspace is owned by the user.
	WorkspaceBelongsToUser(ctx context.Context, workspaceID, userID string) (bool, error)
	// CreateWorkspace creates a new workspace for the user and returns it.
	CreateWorkspace(ctx context.Context, userID, name string) (*Workspace, error)
}

// ProjectStore provides project persistence.
type ProjectStore interface {
	GetProject(ctx context.Context, projectID string) (*Project, error)
	ListProjectsByWorkspace(ctx context.Context, workspaceID string) ([]Project, error)
	CreateProject(ctx context.Context, workspaceID, name, description string) (*Project, error)
}

// TaskStore provides task persistence. Tasks belong to a workspace; project is optional.
type TaskStore interface {
	// ListTasksByWorkspace returns tasks in the workspace. If projectID is non-nil, filter by that project.
	ListTasksByWorkspace(ctx context.Context, workspaceID string, projectID *string) ([]Task, error)
	// GetTaskBySessionID returns the task that has the given session_id, or (nil, nil) if none.
	GetTaskBySessionID(ctx context.Context, sessionID string) (*Task, error)
	// CreateTask inserts a new task with status PENDING. projectID is optional (nil = no project).
	CreateTask(ctx context.Context, workspaceID string, projectID *string, input, createdBy string) (*Task, error)
	// GetNextPendingTask returns the oldest task with status PENDING (by created_at), or (nil, nil) if none.
	GetNextPendingTask(ctx context.Context) (*Task, error)
	// UpdateTaskStatus updates a task's status and optional fields (started_at, ended_at, output, error_message, session_id).
	// Only non-nil pointer fields are updated.
	UpdateTaskStatus(ctx context.Context, taskID, status string, startedAt, endedAt *int64, output, errorMessage, sessionID *string) error
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

// CreateUser creates a user with the given email. Name is set to empty.
// Returns ErrEmailExists if the email is already registered.
func (s *Store) CreateUser(ctx context.Context, email string) (*User, error) {
	existing, err := s.UserByEmail(ctx, email)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return nil, ErrEmailExists
	}
	u := User{
		UserID:    id.New(),
		Email:     email,
		Name:      "",
		CreatedAt: time.Now().Unix(),
	}
	if err := s.db.WithContext(ctx).Create(&u).Error; err != nil {
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
		WorkspaceID: id.New(),
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
		WorkspaceID: id.New(),
		OwnerUserID: userID,
		Name:        name,
		CreatedAt:   time.Now().Unix(),
	}
	if err := s.db.WithContext(ctx).Create(w).Error; err != nil {
		return nil, err
	}
	return w, nil
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
		ProjectID:   id.New(),
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

// ListTasksByWorkspace returns tasks in the workspace, ordered by created_at.
// If projectID is non-nil, only tasks with that project_id are returned.
func (s *Store) ListTasksByWorkspace(ctx context.Context, workspaceID string, projectID *string) ([]Task, error) {
	var list []Task
	q := s.db.WithContext(ctx).Where("workspace_id = ?", workspaceID)
	if projectID != nil {
		q = q.Where("project_id = ?", *projectID)
	}
	err := q.Order("created_at ASC").Find(&list).Error
	return list, err
}

// GetTaskBySessionID returns the task with the given session_id, or (nil, nil) if not found.
func (s *Store) GetTaskBySessionID(ctx context.Context, sessionID string) (*Task, error) {
	var t Task
	err := s.db.WithContext(ctx).Where("session_id = ?", sessionID).First(&t).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &t, nil
}

// CreateTask inserts a new task with status PENDING and returns it.
// projectID is optional (nil = task with no project).
func (s *Store) CreateTask(ctx context.Context, workspaceID string, projectID *string, input, createdBy string) (*Task, error) {
	t := &Task{
		TaskID:      id.New(),
		WorkspaceID: workspaceID,
		ProjectID:   projectID,
		Status:      "PENDING",
		Input:       input,
		CreatedBy:   createdBy,
		CreatedAt:   time.Now().Unix(),
	}
	if err := s.db.WithContext(ctx).Create(t).Error; err != nil {
		return nil, err
	}
	return t, nil
}

// GetNextPendingTask returns the oldest task with status PENDING (by created_at), or (nil, nil) if none.
func (s *Store) GetNextPendingTask(ctx context.Context) (*Task, error) {
	var t Task
	err := s.db.WithContext(ctx).Where("status = ?", "PENDING").Order("created_at ASC").First(&t).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &t, nil
}

// UpdateTaskStatus updates a task's status and optional fields.
// Only non-nil pointer fields are written; status is always set.
func (s *Store) UpdateTaskStatus(ctx context.Context, taskID, status string, startedAt, endedAt *int64, output, errorMessage, sessionID *string) error {
	updates := map[string]interface{}{"status": status}
	if startedAt != nil {
		updates["started_at"] = *startedAt
	}
	if endedAt != nil {
		updates["ended_at"] = *endedAt
	}
	if output != nil {
		updates["output"] = *output
	}
	if errorMessage != nil {
		updates["error_message"] = *errorMessage
	}
	if sessionID != nil {
		updates["session_id"] = *sessionID
	}
	return s.db.WithContext(ctx).Model(&Task{}).Where("task_id = ?", taskID).Updates(updates).Error
}

// BackfillTaskWorkspaceID fills workspace_id for existing tasks that have a project_id
// but no workspace_id. Idempotent — safe to call on every startup.
func (s *Store) BackfillTaskWorkspaceID(ctx context.Context) error {
	return s.db.WithContext(ctx).Exec(`
		UPDATE task t
		JOIN project p ON t.project_id = p.project_id
		SET t.workspace_id = p.workspace_id
		WHERE t.workspace_id = '' OR t.workspace_id IS NULL
	`).Error
}

// Close closes the underlying DB connection. Optional for server lifecycle.
func (s *Store) Close() error {
	sqlDB, err := s.db.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}
