package db

import (
	"context"
	"errors"
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
	ID       uint64 `gorm:"primaryKey;autoIncrement"`
	PublicID string `gorm:"column:public_id;type:char(20) CHARACTER SET ascii COLLATE ascii_bin;uniqueIndex:uq_audit_event_public_id;not null"`

	// TeamID is NULL for actions with no team, such as a login -- a pointer
	// rather than a zero, because zero is a row key someone owns. The composite
	// index leads with it because reading is always "this team, newest first".
	TeamID    *uint64   `gorm:"column:team_id;index:idx_audit_team_time,priority:1"`
	CreatedAt time.Time `gorm:"not null;index:idx_audit_team_time,priority:2"`

	// The actor and the target stay opaque handles. Their type is in a column
	// beside them and admits values that are not rows at all -- an operator, a
	// worker, a permission name -- so one numeric reference cannot name them.
	ActorType string `gorm:"type:varchar(16);not null"`
	ActorID   string `gorm:"type:varchar(64);not null;index"`

	Action     string `gorm:"type:varchar(64);not null;index"`
	TargetType string `gorm:"type:varchar(32)"`
	TargetID   string `gorm:"type:varchar(64)"`
	Detail     string `gorm:"type:varchar(255)"`
}

func (auditEventRow) TableName() string { return "audit_event" }

// auditEventReadRow is the row plus its team's handle. A pointer field is one
// a LEFT JOIN may leave NULL.
type auditEventReadRow struct {
	Row          auditEventRow `gorm:"embedded"`
	TeamPublicID *string       `gorm:"column:team_public_id"`
}

func (s *Store) auditSelect(ctx context.Context) *gorm.DB {
	return s.db.WithContext(ctx).Model(&auditEventRow{}).
		Select("audit_event.*, t.public_id AS team_public_id").
		Joins("LEFT JOIN team t ON t.id = audit_event.team_id")
}

func toAuditEvent(row *auditEventReadRow) *model.AuditEvent {
	if row == nil {
		return nil
	}
	return &model.AuditEvent{
		ID:         row.Row.PublicID,
		TeamID:     derefPublicID(row.TeamPublicID),
		ActorType:  row.Row.ActorType,
		ActorID:    row.Row.ActorID,
		Action:     row.Row.Action,
		TargetType: row.Row.TargetType,
		TargetID:   row.Row.TargetID,
		Detail:     row.Row.Detail,
		CreatedAt:  row.Row.CreatedAt,
	}
}

// RecordAuditEvent appends one event.
func (s *Store) RecordAuditEvent(ctx context.Context, in model.AuditEvent) error {
	publicID, err := util.NewPublicID()
	if err != nil {
		return err
	}
	// An event with no team is a login or an account action; it is still
	// evidence, so an unresolvable team is not a reason to lose the record.
	var teamKey *uint64
	if in.TeamID != "" {
		key, err := lookupKey(ctx, s.db, "team", in.TeamID)
		if err != nil && !errors.Is(err, model.ErrNotFound) {
			return err
		}
		if err == nil {
			teamKey = &key
		}
	}
	row := auditEventRow{
		PublicID:   publicID,
		TeamID:     teamKey,
		ActorType:  in.ActorType,
		ActorID:    in.ActorID,
		Action:     in.Action,
		TargetType: in.TargetType,
		TargetID:   in.TargetID,
		Detail:     truncateDetail(in.Detail),
		CreatedAt:  time.Now().UTC(),
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
	limit, offset = clampPage(limit, offset)
	teamKey, err := lookupKey(ctx, s.db, "team", teamID)
	if errors.Is(err, model.ErrNotFound) {
		return nil, 0, nil
	}
	if err != nil {
		return nil, 0, err
	}
	var total int64
	if err := s.db.WithContext(ctx).Model(&auditEventRow{}).Where("team_id = ?", teamKey).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []auditEventReadRow
	if err := s.auditSelect(ctx).Where("audit_event.team_id = ?", teamKey).
		Order("audit_event.created_at DESC, audit_event.id DESC").Limit(limit).Offset(offset).Find(&rows).Error; err != nil {
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
	limit, offset = clampPage(limit, offset)
	teamKey, err := s.auditFilterTeamKey(ctx, filter)
	if err != nil {
		return nil, 0, err
	}
	var total int64
	if err := applyAuditFilter(s.db.WithContext(ctx).Model(&auditEventRow{}), "", filter, teamKey).
		Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []auditEventReadRow
	if err := applyAuditFilter(s.auditSelect(ctx), "audit_event.", filter, teamKey).
		Order("audit_event.created_at DESC, audit_event.id DESC").Limit(limit).Offset(offset).Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	out := make([]model.AuditEvent, 0, len(rows))
	for i := range rows {
		out = append(out, *toAuditEvent(&rows[i]))
	}
	return out, int(total), nil
}

// auditFilterTeamKey resolves the filter's team once. A filter naming a team
// that does not exist matches nothing, which is reported as an empty page
// rather than an error: a search is allowed to find nothing.
func (s *Store) auditFilterTeamKey(ctx context.Context, filter model.AuditFilter) (*uint64, error) {
	if filter.WithoutTeam || filter.TeamID == "" {
		return nil, nil
	}
	key, err := lookupKey(ctx, s.db, "team", filter.TeamID)
	if errors.Is(err, model.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &key, nil
}

// applyAuditFilter narrows a query to the events a filter names. It is shared
// by the paged search and the export so the two cannot drift into answering
// different questions from the same parameters.
func applyAuditFilter(q *gorm.DB, col string, filter model.AuditFilter, teamKey *uint64) *gorm.DB {
	switch {
	case filter.WithoutTeam:
		q = q.Where(col + "team_id IS NULL")
	case teamKey != nil:
		q = q.Where(col+"team_id = ?", *teamKey)
	}
	if filter.ActorID != "" {
		q = q.Where(col+"actor_id = ?", filter.ActorID)
	}
	if filter.Action != "" {
		q = q.Where(col+"action = ?", filter.Action)
	}
	if !filter.Since.IsZero() {
		q = q.Where(col+"created_at >= ?", filter.Since)
	}
	if !filter.Until.IsZero() {
		q = q.Where(col+"created_at < ?", filter.Until)
	}
	return q
}

// applyAuditCursor continues a walk from where the last page stopped.
//
// The comparison is on the pair, not on the timestamp: created_at has
// one-second resolution, so several events can share it, and a `created_at <`
// bound alone would drop the ones that tied with the last row of the previous
// page.
//
// The tie-break is the row key, which no caller may hold, so the cursor names
// the event by its public handle and the key is resolved in a subquery. That
// keeps the translation where every other one lives -- inside this package --
// and costs one indexed lookup per page rather than a round trip per page.
func (s *Store) applyAuditCursor(ctx context.Context, q *gorm.DB, after model.AuditCursor) *gorm.DB {
	if after.Zero() {
		return q
	}
	id, ok := util.CanonicalPublicID(after.ID)
	if !ok {
		return q
	}
	key := s.db.WithContext(ctx).Model(&auditEventRow{}).Select("id").Where("public_id = ?", id)
	return q.Where("audit_event.created_at < ? OR (audit_event.created_at = ? AND audit_event.id < (?))",
		after.CreatedAt, after.CreatedAt, key)
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

func auditRowsToEvents(rows []auditEventReadRow) []model.AuditEvent {
	out := make([]model.AuditEvent, 0, len(rows))
	for i := range rows {
		out = append(out, *toAuditEvent(&rows[i]))
	}
	return out
}

// ExportTeamAuditEvents returns one page of a team's events, newest first,
// continuing from after.
func (s *Store) ExportTeamAuditEvents(ctx context.Context, teamID string, after model.AuditCursor, limit int) ([]model.AuditEvent, error) {
	teamKey, err := lookupKey(ctx, s.db, "team", teamID)
	if errors.Is(err, model.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	q := s.auditSelect(ctx).Where("audit_event.team_id = ?", teamKey)
	q = s.applyAuditCursor(ctx, q, after)

	var rows []auditEventReadRow
	if err := q.Order("audit_event.created_at DESC, audit_event.id DESC").Limit(auditPageSize(limit)).Find(&rows).Error; err != nil {
		return nil, err
	}
	return auditRowsToEvents(rows), nil
}

// ExportAuditEvents returns one page of events across every team, newest first,
// continuing from after.
func (s *Store) ExportAuditEvents(ctx context.Context, filter model.AuditFilter, after model.AuditCursor, limit int) ([]model.AuditEvent, error) {
	teamKey, err := s.auditFilterTeamKey(ctx, filter)
	if err != nil {
		return nil, err
	}
	q := applyAuditFilter(s.auditSelect(ctx), "audit_event.", filter, teamKey)
	q = s.applyAuditCursor(ctx, q, after)

	var rows []auditEventReadRow
	if err := q.Order("audit_event.created_at DESC, audit_event.id DESC").Limit(auditPageSize(limit)).Find(&rows).Error; err != nil {
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
func (s *Store) PruneAuditEvents(ctx context.Context, before time.Time, limit int) (int64, error) {
	if before.IsZero() {
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
func (s *Store) OldestAuditEventAt(ctx context.Context) (time.Time, error) {
	var oldest []time.Time
	if err := s.db.WithContext(ctx).Model(&auditEventRow{}).
		Order("created_at ASC, id ASC").
		Limit(1).
		Pluck("created_at", &oldest).Error; err != nil {
		return time.Time{}, err
	}
	if len(oldest) == 0 {
		return time.Time{}, nil
	}
	return oldest[0], nil
}
