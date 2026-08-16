package db

import (
	"context"
	"time"

	"github.com/gougoujiang/buildmax/internal/core/model"
	"github.com/gougoujiang/buildmax/internal/util"
)

// auditEventRow is the governance evidence table. It records that an action
// happened and who performed it — never what was said, generated, or run.
//
// There is no update or delete path anywhere in this file. A record that can be
// edited is not evidence.
type auditEventRow struct {
	ID           uint   `gorm:"primaryKey;autoIncrement"`
	AuditEventID string `gorm:"column:audit_event_id;type:varchar(64);uniqueIndex;not null"`

	// TeamID is empty for actions with no team, such as a login. The composite
	// index leads with it because reading is always "this team, newest first".
	TeamID    string `gorm:"type:varchar(64);index:idx_audit_team_time,priority:1"`
	CreatedAt int64  `gorm:"not null;index:idx_audit_team_time,priority:2"`

	ActorType string `gorm:"type:varchar(16);not null"`
	ActorID   string `gorm:"type:varchar(64);not null;index"`

	Action     string `gorm:"type:varchar(64);not null;index"`
	TargetType string `gorm:"type:varchar(32)"`
	TargetID   string `gorm:"type:varchar(64)"`
	Detail     string `gorm:"type:varchar(255)"`
}

func (auditEventRow) TableName() string { return "audit_event" }

func toAuditEvent(row *auditEventRow) *model.AuditEvent {
	if row == nil {
		return nil
	}
	return &model.AuditEvent{
		ID:           row.ID,
		AuditEventID: row.AuditEventID,
		TeamID:       row.TeamID,
		ActorType:    row.ActorType,
		ActorID:      row.ActorID,
		Action:       row.Action,
		TargetType:   row.TargetType,
		TargetID:     row.TargetID,
		Detail:       row.Detail,
		CreatedAt:    row.CreatedAt,
	}
}

// RecordAuditEvent appends one event.
func (s *Store) RecordAuditEvent(ctx context.Context, in model.AuditEvent) error {
	row := auditEventRow{
		AuditEventID: util.NewPrefixedID(util.PrefixAuditEvent),
		TeamID:       in.TeamID,
		ActorType:    in.ActorType,
		ActorID:      in.ActorID,
		Action:       in.Action,
		TargetType:   in.TargetType,
		TargetID:     in.TargetID,
		Detail:       truncateDetail(in.Detail),
		CreatedAt:    time.Now().Unix(),
	}
	return s.db.WithContext(ctx).Create(&row).Error
}

// truncateDetail bounds the one free-text column. Detail is meant for a role
// name or a model alias; bounding it means a caller that passes something
// larger loses the tail rather than failing the write and, with it, the record
// that the action happened.
func truncateDetail(s string) string {
	const max = 255
	if len(s) <= max {
		return s
	}
	return s[:max]
}

// ListAuditEvents returns a team's events, newest first, with the total count.
func (s *Store) ListAuditEvents(ctx context.Context, teamID string, limit, offset int) ([]model.AuditEvent, int, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	if offset < 0 {
		offset = 0
	}
	q := s.db.WithContext(ctx).Model(&auditEventRow{}).Where("team_id = ?", teamID)

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []auditEventRow
	if err := q.Order("created_at DESC, id DESC").Limit(limit).Offset(offset).Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	out := make([]model.AuditEvent, 0, len(rows))
	for i := range rows {
		out = append(out, *toAuditEvent(&rows[i]))
	}
	return out, int(total), nil
}
