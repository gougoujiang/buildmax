package db

import (
	"context"
	"errors"

	"gorm.io/gorm"
)

// Both revision tables are the same query shape: append-only rows keyed by an
// owner column and a revision number, newest first. Only the row type and the
// column name differ, so they are parameters rather than two copies.
//
// ownerCol reaches a WHERE clause as text, so it must stay a constant from this
// package and never a value from a request.

func listRevisions[R any](ctx context.Context, db *gorm.DB, ownerCol, ownerID string, limit, offset int) ([]R, int, error) {
	limit, offset = capPage(limit, offset)
	var total int64
	if err := db.WithContext(ctx).Model(new(R)).Where(ownerCol+" = ?", ownerID).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	q := db.WithContext(ctx).Where(ownerCol+" = ?", ownerID).Order("revision DESC")
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
func getRevision[R any](ctx context.Context, db *gorm.DB, ownerCol, ownerID string, revision int) (*R, error) {
	var row R
	err := db.WithContext(ctx).Where(ownerCol+" = ? AND revision = ?", ownerID, revision).First(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &row, nil
}
