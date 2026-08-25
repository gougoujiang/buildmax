package db

import (
	"context"
	"errors"

	"gorm.io/gorm"
)

// nextRevision returns the revision number that follows current. Rows that
// predate revision tracking are treated as revision 1, so their first edit is 2.
func nextRevision(current int) int {
	if current < 1 {
		return 2
	}
	return current + 1
}

// Both revision tables are the same query shape: append-only rows keyed by an
// owner column and a revision number, newest first. Only the row type, the
// column name, and the join set differ, so they are parameters rather than two
// copies.
//
// table and ownerCol reach a query as text, so both must stay constants from
// this package and never values from a request. Both are qualified inside: the
// read joins for the handles it returns, and an unqualified `revision` is
// ambiguous once the owner table -- which has a revision of its own -- is in
// the query.
func listRevisions[R any](ctx context.Context, db, sel *gorm.DB, table, ownerCol string, ownerKey uint64, limit, offset int) ([]R, int, error) {
	limit, offset = capPage(limit, offset)
	var total int64
	if err := db.WithContext(ctx).Table(table).Where(ownerCol+" = ?", ownerKey).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	q := sel.Where(table+"."+ownerCol+" = ?", ownerKey).Order(table + ".revision DESC")
	if limit > 0 {
		q = q.Limit(limit)
	}
	if offset > 0 {
		q = q.Offset(offset)
	}
	var rows []R
	if err := q.Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	return rows, int(total), nil
}

// getRevision returns (nil, nil) when there is no such revision, which is how
// both stores already reported it.
func getRevision[R any](sel *gorm.DB, table, ownerCol string, ownerKey uint64, revision int) (*R, error) {
	var row R
	err := sel.Where(table+"."+ownerCol+" = ? AND "+table+".revision = ?", ownerKey, revision).Take(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &row, nil
}
