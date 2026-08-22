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
	ID           uint64 `gorm:"primaryKey;autoIncrement"`
	PublicID     string `gorm:"column:public_id;type:char(20) CHARACTER SET ascii COLLATE ascii_bin;uniqueIndex:uq_agent_public_id;not null"`
	UserID       uint64 `gorm:"column:user_id;not null;index"`
	TeamID       uint64 `gorm:"column:team_id;index"`
	Name         string `gorm:"type:varchar(255);not null"`
	Description  string `gorm:"type:text"`
	Instructions string `gorm:"type:text"`
	Revision     int    `gorm:"column:revision;not null;default:1"`
	DeletedAt    *int64 `gorm:"column:deleted_at;index"`
	CreatedAt    int64  `gorm:"autoCreateTime"`
}

func (agentRow) TableName() string { return "agent" }

// agentReadRow is the row plus the handles its references resolve to. A
// pointer field is one a LEFT JOIN may leave NULL.
type agentReadRow struct {
	Row          agentRow `gorm:"embedded"`
	UserPublicID string   `gorm:"column:user_public_id"`
	TeamPublicID *string  `gorm:"column:team_public_id"`
}

func (s *Store) agentSelect(ctx context.Context) *gorm.DB {
	return agentSelectTx(s.db.WithContext(ctx))
}

func agentSelectTx(tx *gorm.DB) *gorm.DB {
	return tx.Model(&agentRow{}).
		Select("agent.*, u.public_id AS user_public_id, t.public_id AS team_public_id").
		Joins("INNER JOIN `user` u ON u.id = agent.user_id").
		Joins("LEFT JOIN team t ON t.id = agent.team_id")
}

// agentRevisionRow is one recorded version of an agent definition. Rows are
// appended, never updated or deleted, and they outlive the agent: deleting an
// agent leaves its history in place.
type agentRevisionRow struct {
	ID           uint64 `gorm:"primaryKey;autoIncrement"`
	AgentID      uint64 `gorm:"column:agent_id;not null;index:idx_agent_revision,unique,priority:1"`
	Revision     int    `gorm:"column:revision;not null;index:idx_agent_revision,unique,priority:2"`
	Name         string `gorm:"type:varchar(255);not null"`
	Description  string `gorm:"type:text"`
	Instructions string `gorm:"type:text"`
	CreatedBy    uint64 `gorm:"column:created_by;not null"`
	CreatedAt    int64  `gorm:"autoCreateTime"`
}

func (agentRevisionRow) TableName() string { return "agent_revision" }

// agentRevisionReadRow is the row plus the handles its references resolve to.
// The revision itself has none: it is addressed by its agent plus its number.
type agentRevisionReadRow struct {
	Row               agentRevisionRow `gorm:"embedded"`
	AgentPublicID     string           `gorm:"column:agent_public_id"`
	CreatedByPublicID string           `gorm:"column:created_by_public_id"`
}

func (s *Store) agentRevisionSelect(ctx context.Context) *gorm.DB {
	return s.db.WithContext(ctx).Model(&agentRevisionRow{}).
		Select("agent_revision.*, a.public_id AS agent_public_id, cb.public_id AS created_by_public_id").
		Joins("INNER JOIN agent a ON a.id = agent_revision.agent_id").
		Joins("INNER JOIN `user` cb ON cb.id = agent_revision.created_by")
}

func toAgent(row *agentReadRow) *model.Agent {
	if row == nil {
		return nil
	}
	return &model.Agent{
		ID:           row.Row.PublicID,
		UserID:       row.UserPublicID,
		TeamID:       derefPublicID(row.TeamPublicID),
		Name:         row.Row.Name,
		Description:  row.Row.Description,
		Instructions: row.Row.Instructions,
		Revision:     row.Row.Revision,
		DeletedAt:    row.Row.DeletedAt,
		CreatedAt:    row.Row.CreatedAt,
	}
}

func toAgentRevision(row *agentRevisionReadRow) *model.AgentRevision {
	if row == nil {
		return nil
	}
	return &model.AgentRevision{
		AgentID:      row.AgentPublicID,
		Revision:     row.Row.Revision,
		Name:         row.Row.Name,
		Description:  row.Row.Description,
		Instructions: row.Row.Instructions,
		CreatedBy:    row.CreatedByPublicID,
		CreatedAt:    row.Row.CreatedAt,
	}
}

func toAgentRevisions(rows []agentRevisionReadRow) []model.AgentRevision {
	out := make([]model.AgentRevision, len(rows))
	for i := range rows {
		out[i] = *toAgentRevision(&rows[i])
	}
	return out
}

func toAgents(rows []agentReadRow) []model.Agent {
	out := make([]model.Agent, len(rows))
	for i := range rows {
		out[i] = *toAgent(&rows[i])
	}
	return out
}

// appendAgentRevision records a's current content as its revision. It runs in
// the same transaction as the write it describes, so history cannot drift from
// the row: the unique (agent_id, revision) index makes a concurrent second
// write fail rather than record two different definitions under one number.
func appendAgentRevision(ctx context.Context, tx *gorm.DB, agentKey uint64, a *model.Agent, createdBy string) error {
	creator, err := lookupKey(ctx, tx, "user", createdBy)
	if err != nil {
		return err
	}
	return tx.Create(&agentRevisionRow{
		AgentID:      agentKey,
		Revision:     a.Revision,
		Name:         a.Name,
		Description:  a.Description,
		Instructions: a.Instructions,
		CreatedBy:    creator,
		CreatedAt:    time.Now().Unix(),
	}).Error
}

// GetAgent returns the live agent by agent_id, or (nil, nil) when there is none.
// A deleted agent reads as not found here; see GetAgentIncludingDeleted.
func (s *Store) GetAgent(ctx context.Context, agentID string) (*model.Agent, error) {
	id, ok := util.CanonicalPublicID(agentID)
	if !ok {
		return nil, nil
	}
	var a agentReadRow
	err := s.agentSelect(ctx).Where("agent.public_id = ? AND agent.deleted_at IS NULL", id).Take(&a).Error
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
	id, ok := util.CanonicalPublicID(agentID)
	if !ok {
		return nil, nil
	}
	var a agentReadRow
	err := s.agentSelect(ctx).Where("agent.public_id = ?", id).Take(&a).Error
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
	id, ok := util.CanonicalPublicID(userID)
	if !ok {
		return nil, nil
	}
	var list []agentReadRow
	err := s.agentSelect(ctx).Where("u.public_id = ? AND agent.deleted_at IS NULL", id).
		Order("agent.created_at ASC").Find(&list).Error
	return toAgents(list), err
}

// ListAgentsByTeam returns all agents for the given team_id, ordered by created_at ASC.
func (s *Store) ListAgentsByTeam(ctx context.Context, teamID string) ([]model.Agent, error) {
	id, ok := util.CanonicalPublicID(teamID)
	if !ok {
		return nil, nil
	}
	var list []agentReadRow
	err := s.agentSelect(ctx).Where("t.public_id = ? AND agent.deleted_at IS NULL", id).
		Order("agent.created_at ASC").Find(&list).Error
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
		UserID:       userID,
		TeamID:       teamID,
		Name:         name,
		Description:  description,
		Instructions: instructions,
		Revision:     1,
		CreatedAt:    time.Now().Unix(),
	}
	row := &agentRow{
		Name:         name,
		Description:  description,
		Instructions: instructions,
		Revision:     1,
		CreatedAt:    a.CreatedAt,
	}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		userKey, err := lookupKey(ctx, tx, "user", userID)
		if err != nil {
			return err
		}
		row.UserID = userKey
		if teamID != "" {
			teamKey, err := lookupKey(ctx, tx, "team", teamID)
			if err != nil {
				return err
			}
			row.TeamID = teamKey
		}
		if err := createWithPublicID(ctx, tx, "uq_agent_public_id",
			func(id string) { row.PublicID = id }, row); err != nil {
			return err
		}
		return appendAgentRevision(ctx, tx, row.ID, a, userID)
	})
	if err != nil {
		return nil, err
	}
	a.ID = row.PublicID
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
		// Addressed by its handle rather than by saving a row built from the
		// model: the model carries no row key, so Save would see a zero primary
		// key and insert a second agent instead of updating this one.
		agentKey, err := lookupKey(ctx, tx, "agent", updated.ID)
		if err != nil {
			return err
		}
		res := tx.Model(&agentRow{}).Where("id = ?", agentKey).Updates(map[string]any{
			"name":         updated.Name,
			"description":  updated.Description,
			"instructions": updated.Instructions,
			"revision":     updated.Revision,
		})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return model.ErrNotFound
		}
		return appendAgentRevision(ctx, tx, agentKey, &updated, updatedBy)
	})
	if err != nil {
		return nil, err
	}
	return &updated, nil
}

// ListAgentRevisions returns an agent's revisions, newest first.
func (s *Store) ListAgentRevisions(ctx context.Context, agentID string, limit, offset int) ([]model.AgentRevision, int, error) {
	limit, offset = capPage(limit, offset)
	agentKey, err := lookupKey(ctx, s.db, "agent", agentID)
	if errors.Is(err, model.ErrNotFound) {
		return nil, 0, nil
	}
	if err != nil {
		return nil, 0, err
	}
	rows, total, err := listRevisions[agentRevisionReadRow](ctx, s.db, s.agentRevisionSelect(ctx),
		"agent_revision", "agent_id", agentKey, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	return toAgentRevisions(rows), total, nil
}

// GetAgentRevision returns one revision, or (nil, nil) when there is no such revision.
func (s *Store) GetAgentRevision(ctx context.Context, agentID string, revision int) (*model.AgentRevision, error) {
	agentKey, err := lookupKey(ctx, s.db, "agent", agentID)
	if errors.Is(err, model.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	row, err := getRevision[agentRevisionReadRow](s.agentRevisionSelect(ctx),
		"agent_revision", "agent_id", agentKey, revision)
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
