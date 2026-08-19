package db

import (
	"context"
	"time"

	"gorm.io/gorm"

	"github.com/gougoujiang/buildmax/internal/core/model"
	"github.com/gougoujiang/buildmax/internal/util"
)

// auditEventRow is the governance evidence table. It records that an action
// happened and who performed it — never what was said, generated, or run.
//
// Nothing updates a row here. A record that can be edited is not evidence. The
// one delete is PruneAuditEvents, which expires rows by age under the
// deployment's retention window and cannot be aimed at a particular record; the
// sweep that calls it writes down what it removed.
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

// SearchAuditEvents returns events across every team, newest first.
//
// The composite index leads with team_id, so a team-filtered search uses it and
// an unfiltered one is an ordered scan of a table that only grows by
// deliberate action. If that stops being true the answer is a second index on
// created_at, not a smaller retention — losing evidence to make a query fast is
// the wrong trade.
func (s *Store) SearchAuditEvents(ctx context.Context, filter model.AuditFilter, limit, offset int) ([]model.AuditEvent, int, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	if offset < 0 {
		offset = 0
	}
	q := applyAuditFilter(s.db.WithContext(ctx).Model(&auditEventRow{}), filter)

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

// applyAuditFilter narrows a query to the events a filter names. It is shared
// by the paged search and the export so the two cannot drift into answering
// different questions from the same parameters.
func applyAuditFilter(q *gorm.DB, filter model.AuditFilter) *gorm.DB {
	switch {
	case filter.WithoutTeam:
		q = q.Where("team_id = ?", "")
	case filter.TeamID != "":
		q = q.Where("team_id = ?", filter.TeamID)
	}
	if filter.ActorID != "" {
		q = q.Where("actor_id = ?", filter.ActorID)
	}
	if filter.Action != "" {
		q = q.Where("action = ?", filter.Action)
	}
	if filter.Since > 0 {
		q = q.Where("created_at >= ?", filter.Since)
	}
	if filter.Until > 0 {
		q = q.Where("created_at < ?", filter.Until)
	}
	return q
}

// applyAuditCursor continues a walk from where the last page stopped.
//
// The comparison is on the pair, not on the timestamp: created_at has
// one-second resolution, so several events can share it, and a `created_at <`
// bound alone would drop the ones that tied with the last row of the previous
// page.
func applyAuditCursor(q *gorm.DB, after model.AuditCursor) *gorm.DB {
	if after.Zero() {
		return q
	}
	return q.Where("created_at < ? OR (created_at = ? AND id < ?)", after.CreatedAt, after.CreatedAt, after.ID)
}

func auditPageSize(limit int) int {
	if limit <= 0 {
		return 200
	}
	if limit > 1000 {
		return 1000
	}
	return limit
}

func auditRowsToEvents(rows []auditEventRow) []model.AuditEvent {
	out := make([]model.AuditEvent, 0, len(rows))
	for i := range rows {
		out = append(out, *toAuditEvent(&rows[i]))
	}
	return out
}

// ExportTeamAuditEvents returns one page of a team's events, newest first,
// continuing from after.
func (s *Store) ExportTeamAuditEvents(ctx context.Context, teamID string, after model.AuditCursor, limit int) ([]model.AuditEvent, error) {
	q := s.db.WithContext(ctx).Model(&auditEventRow{}).Where("team_id = ?", teamID)
	q = applyAuditCursor(q, after)

	var rows []auditEventRow
	if err := q.Order("created_at DESC, id DESC").Limit(auditPageSize(limit)).Find(&rows).Error; err != nil {
		return nil, err
	}
	return auditRowsToEvents(rows), nil
}

// ExportAuditEvents returns one page of events across every team, newest first,
// continuing from after.
func (s *Store) ExportAuditEvents(ctx context.Context, filter model.AuditFilter, after model.AuditCursor, limit int) ([]model.AuditEvent, error) {
	q := applyAuditFilter(s.db.WithContext(ctx).Model(&auditEventRow{}), filter)
	q = applyAuditCursor(q, after)

	var rows []auditEventRow
	if err := q.Order("created_at DESC, id DESC").Limit(auditPageSize(limit)).Find(&rows).Error; err != nil {
		return nil, err
	}
	return auditRowsToEvents(rows), nil
}

// PruneAuditEvents deletes events recorded before the cutoff, at most limit of
// them, and returns how many went.
//
// This is the only delete anywhere near this table, and it is a retention
// policy rather than an edit: it removes rows by age and cannot be pointed at a
// particular record. The sweep that calls it records what it removed, so a
// trail that starts partway through says why it does — see
// model.AuditEventsPruned.
func (s *Store) PruneAuditEvents(ctx context.Context, before int64, limit int) (int64, error) {
	if before <= 0 {
		return 0, nil
	}
	if limit <= 0 {
		limit = 1000
	}
	// Select the ids first, then delete those. A bounded DELETE is not portable
	// SQL, and going through the primary key means the batch this sweep removes
	// is exactly the batch it chose — an unbounded predicate delete would be at
	// the mercy of whatever the driver did with the limit.
	var ids []uint
	if err := s.db.WithContext(ctx).Model(&auditEventRow{}).
		Where("created_at < ?", before).
		Order("created_at ASC, id ASC").
		Limit(limit).
		Pluck("id", &ids).Error; err != nil {
		return 0, err
	}
	if len(ids) == 0 {
		return 0, nil
	}
	res := s.db.WithContext(ctx).Where("id IN ?", ids).Delete(&auditEventRow{})
	return res.RowsAffected, res.Error
}

// OldestAuditEventAt returns the timestamp of the oldest event, or zero when
// there are none.
func (s *Store) OldestAuditEventAt(ctx context.Context) (int64, error) {
	var oldest []int64
	if err := s.db.WithContext(ctx).Model(&auditEventRow{}).
		Order("created_at ASC, id ASC").
		Limit(1).
		Pluck("created_at", &oldest).Error; err != nil {
		return 0, err
	}
	if len(oldest) == 0 {
		return 0, nil
	}
	return oldest[0], nil
}
