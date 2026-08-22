package db

import (
	"context"
	"errors"
	"github.com/gougoujiang/buildmax/internal/core/model"
	"time"

	"github.com/gougoujiang/buildmax/internal/util"
	"gorm.io/gorm"
)

type agentRow struct {
	ID           uint   `gorm:"primaryKey;autoIncrement"`
	AgentID      string `gorm:"type:varchar(64);uniqueIndex;not null"`
	UserID       string `gorm:"type:varchar(64);not null;index"`
	TeamID       string `gorm:"type:varchar(64);index"`
	Name         string `gorm:"type:varchar(255);not null"`
	Description  string `gorm:"type:text"`
	Instructions string `gorm:"type:text"`
	Revision     int    `gorm:"column:revision;not null;default:1"`
	DeletedAt    *int64 `gorm:"column:deleted_at;index"`
	CreatedAt    int64  `gorm:"autoCreateTime"`
}

func (agentRow) TableName() string { return "agent" }

// agentRevisionRow is one recorded version of an agent definition. Rows are
// appended, never updated or deleted, and they outlive the agent: deleting an
// agent leaves its history in place.
type agentRevisionRow struct {
	ID              uint   `gorm:"primaryKey;autoIncrement"`
	AgentRevisionID string `gorm:"column:agent_revision_id;type:varchar(64);uniqueIndex;not null"`
	AgentID         string `gorm:"column:agent_id;type:varchar(64);not null;index:idx_agent_revision,unique,priority:1"`
	Revision        int    `gorm:"column:revision;not null;index:idx_agent_revision,unique,priority:2"`
	Name            string `gorm:"type:varchar(255);not null"`
	Description     string `gorm:"type:text"`
	Instructions    string `gorm:"type:text"`
	CreatedBy       string `gorm:"column:created_by;type:varchar(64);not null"`
	CreatedAt       int64  `gorm:"autoCreateTime"`
}

func (agentRevisionRow) TableName() string { return "agent_revision" }

func toAgent(row *agentRow) *model.Agent {
	if row == nil {
		return nil
	}
	return &model.Agent{
		ID:           row.AgentID,
		UserID:       row.UserID,
		TeamID:       row.TeamID,
		Name:         row.Name,
		Description:  row.Description,
		Instructions: row.Instructions,
		Revision:     row.Revision,
		DeletedAt:    row.DeletedAt,
		CreatedAt:    row.CreatedAt,
	}
}

func toAgentRevision(row *agentRevisionRow) *model.AgentRevision {
	if row == nil {
		return nil
	}
	return &model.AgentRevision{
		AgentID:      row.AgentID,
		Revision:     row.Revision,
		Name:         row.Name,
		Description:  row.Description,
		Instructions: row.Instructions,
		CreatedBy:    row.CreatedBy,
		CreatedAt:    row.CreatedAt,
	}
}

func toAgentRevisions(rows []agentRevisionRow) []model.AgentRevision {
	out := make([]model.AgentRevision, len(rows))
	for i := range rows {
		out[i] = *toAgentRevision(&rows[i])
	}
	return out
}

func toAgents(rows []agentRow) []model.Agent {
	out := make([]model.Agent, len(rows))
	for i := range rows {
		out[i] = *toAgent(&rows[i])
	}
	return out
}

func toAgentRow(m *model.Agent) *agentRow {
	if m == nil {
		return nil
	}
	return &agentRow{
		AgentID:      m.ID,
		UserID:       m.UserID,
		TeamID:       m.TeamID,
		Name:         m.Name,
		Description:  m.Description,
		Instructions: m.Instructions,
		Revision:     m.Revision,
		DeletedAt:    m.DeletedAt,
		CreatedAt:    m.CreatedAt,
	}
}

// appendAgentRevision records a's current content as its revision. It runs in
// the same transaction as the write it describes, so history cannot drift from
// the row: the unique (agent_id, revision) index makes a concurrent second
// write fail rather than record two different definitions under one number.
func appendAgentRevision(tx *gorm.DB, a *model.Agent, createdBy string) error {
	return tx.Create(&agentRevisionRow{
		AgentRevisionID: util.NewPrefixedID(util.PrefixAgentRevision),
		AgentID:         a.ID,
		Revision:        a.Revision,
		Name:            a.Name,
		Description:     a.Description,
		Instructions:    a.Instructions,
		CreatedBy:       createdBy,
		CreatedAt:       time.Now().Unix(),
	}).Error
}

// GetAgent returns the live agent by agent_id, or (nil, nil) when there is none.
// A deleted agent reads as not found here; see GetAgentIncludingDeleted.
func (s *Store) GetAgent(ctx context.Context, agentID string) (*model.Agent, error) {
	var a agentRow
	err := s.db.WithContext(ctx).Where("agent_id = ? AND deleted_at IS NULL", agentID).First(&a).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return toAgent(&a), nil
}

// GetAgentIncludingDeleted returns the agent whether or not it was deleted. It
// answers "what did this record refer to", not "what may I use".
func (s *Store) GetAgentIncludingDeleted(ctx context.Context, agentID string) (*model.Agent, error) {
	var a agentRow
	err := s.db.WithContext(ctx).Where("agent_id = ?", agentID).First(&a).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return toAgent(&a), nil
}

// ListAgentsByUser returns all agents for the given user_id, ordered by created_at ASC.
func (s *Store) ListAgentsByUser(ctx context.Context, userID string) ([]model.Agent, error) {
	var list []agentRow
	err := s.db.WithContext(ctx).Where("user_id = ? AND deleted_at IS NULL", userID).Order("created_at ASC").Find(&list).Error
	return toAgents(list), err
}

// ListAgentsByTeam returns all agents for the given team_id, ordered by created_at ASC.
func (s *Store) ListAgentsByTeam(ctx context.Context, teamID string) ([]model.Agent, error) {
	var list []agentRow
	err := s.db.WithContext(ctx).Where("team_id = ? AND deleted_at IS NULL", teamID).Order("created_at ASC").Find(&list).Error
	return toAgents(list), err
}

// CreateAgent inserts a new agent and returns it.
func (s *Store) CreateAgent(ctx context.Context, userID, name, description, instructions string) (*model.Agent, error) {
	teamID, err := s.personalTeamIDForUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	return s.CreateAgentInTeam(ctx, teamID, userID, name, description, instructions)
}

// CreateAgentInTeam inserts a new team-scoped agent and returns it.
func (s *Store) CreateAgentInTeam(ctx context.Context, teamID, userID, name, description, instructions string) (*model.Agent, error) {
	a := &model.Agent{
		ID:           util.NewPrefixedID(util.PrefixAgent),
		UserID:       userID,
		TeamID:       teamID,
		Name:         name,
		Description:  description,
		Instructions: instructions,
		Revision:     1,
		CreatedAt:    time.Now().Unix(),
	}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(toAgentRow(a)).Error; err != nil {
			return err
		}
		return appendAgentRevision(tx, a, userID)
	})
	if err != nil {
		return nil, err
	}
	return a, nil
}

// UpdateAgent updates name, description, and instructions for the agent. Returns the updated agent, or (nil, nil) if not found or user does not match.
func (s *Store) UpdateAgent(ctx context.Context, agentID, userID, name, description, instructions string) (*model.Agent, error) {
	a, err := s.GetAgent(ctx, agentID)
	if err != nil || a == nil {
		return nil, err
	}
	if a.UserID != userID {
		return nil, nil
	}
	return s.updateAgent(ctx, a, userID, name, description, instructions)
}

// UpdateAgentInTeam updates a team-scoped agent. Returns (nil, nil) if not found or team does not match.
func (s *Store) UpdateAgentInTeam(ctx context.Context, agentID, teamID, updatedBy, name, description, instructions string) (*model.Agent, error) {
	a, err := s.GetAgent(ctx, agentID)
	if err != nil || a == nil {
		return nil, err
	}
	if a.TeamID != teamID {
		return nil, nil
	}
	return s.updateAgent(ctx, a, updatedBy, name, description, instructions)
}

// updateAgent writes new content and records it as the next revision. An update
// that changes nothing is not a revision: a save with no edit should not add a
// row that a reader has to compare against its predecessor to dismiss.
func (s *Store) updateAgent(ctx context.Context, a *model.Agent, updatedBy, name, description, instructions string) (*model.Agent, error) {
	if a.Name == name && a.Description == description && a.Instructions == instructions {
		return a, nil
	}
	updated := *a
	updated.Name = name
	updated.Description = description
	updated.Instructions = instructions
	updated.Revision = nextRevision(a.Revision)
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Save(toAgentRow(&updated)).Error; err != nil {
			return err
		}
		return appendAgentRevision(tx, &updated, updatedBy)
	})
	if err != nil {
		return nil, err
	}
	return &updated, nil
}

// ListAgentRevisions returns an agent's revisions, newest first.
func (s *Store) ListAgentRevisions(ctx context.Context, agentID string, limit, offset int) ([]model.AgentRevision, int, error) {
	limit, offset = capPage(limit, offset)
	rows, total, err := listRevisions[agentRevisionRow](ctx, s.db, "agent_id", agentID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	return toAgentRevisions(rows), total, nil
}

// GetAgentRevision returns one revision, or (nil, nil) when there is no such revision.
func (s *Store) GetAgentRevision(ctx context.Context, agentID string, revision int) (*model.AgentRevision, error) {
	row, err := getRevision[agentRevisionRow](ctx, s.db, "agent_id", agentID, revision)
	if err != nil || row == nil {
		return nil, err
	}
	return toAgentRevision(row), nil
}

// DeleteAgent marks the agent deleted if it exists and belongs to the user.
// Returns model.ErrNotFound if there is no such live agent for that user.
func (s *Store) DeleteAgent(ctx context.Context, agentID, userID string) error {
	a, err := s.GetAgent(ctx, agentID)
	if err != nil {
		return err
	}
	if a == nil || a.UserID != userID {
		return model.ErrNotFound
	}
	return s.markAgentDeleted(ctx, a.ID)
}

// DeleteAgentInTeam marks the agent deleted if it exists and belongs to the team.
func (s *Store) DeleteAgentInTeam(ctx context.Context, agentID, teamID string) error {
	a, err := s.GetAgent(ctx, agentID)
	if err != nil {
		return err
	}
	if a == nil || a.TeamID != teamID {
		return model.ErrNotFound
	}
	return s.markAgentDeleted(ctx, a.ID)
}

// markAgentDeleted stamps deleted_at instead of removing the row.
//
// Tasks, workflow step runs, and revisions all name an agent by ID. Dropping
// the row turned every one of those into a dangling reference and broke any
// workflow run still in flight at its next step; keeping it costs one column
// and leaves the record readable.
func (s *Store) markAgentDeleted(ctx context.Context, agentID string) error {
	return s.db.WithContext(ctx).
		Model(&agentRow{}).
		Where("agent_id = ? AND deleted_at IS NULL", agentID).
		Update("deleted_at", time.Now().Unix()).Error
}
