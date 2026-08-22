package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	mysqldriver "github.com/go-sql-driver/mysql"
	"gorm.io/gorm"

	"github.com/gougoujiang/buildmax/internal/core/model"
	"github.com/gougoujiang/buildmax/internal/util"
)

// This file is the translation boundary. A public ID is what every caller
// holds; a row key is what the schema joins on, and it does not leave this
// package. See docs/design/entity-identity.md.

// publicIDAttempts bounds the retry when a generated public ID collides.
//
// Three is not a tuning parameter. At 96 bits the first collision is already
// implausible, so a second is evidence that something other than chance is
// wrong -- a broken entropy source, or a caller inserting a value it chose --
// and retrying forever would turn that into a hang instead of an error.
const publicIDAttempts = 3

// ErrPublicIDExhausted means every generated public ID collided. It is
// reported rather than retried further: see publicIDAttempts.
var ErrPublicIDExhausted = errors.New("could not generate a free public id")

// mysqlDuplicateEntry is ER_DUP_ENTRY.
const mysqlDuplicateEntry = 1062

// newPublicIDBytes returns the stored form of a fresh public ID.
func newPublicIDBytes() ([]byte, error) {
	id, err := util.NewPublicID()
	if err != nil {
		return nil, err
	}
	raw, ok := util.ParsePublicID(id)
	if !ok {
		return nil, fmt.Errorf("generated public id %q is not canonical", id)
	}
	return raw, nil
}

// lookupKey resolves a caller's public handle to the row key that table joins
// on.
//
// A handle that is not canonical and one that names no row both return
// ErrNotFound. They are the same fact to a caller with no right to the row, and
// distinguishing them would make a well-formed identifier an existence oracle
// for rows in other teams.
func lookupKey(ctx context.Context, tx *gorm.DB, table, publicID string) (uint64, error) {
	raw, ok := util.ParsePublicID(publicID)
	if !ok {
		return 0, model.ErrNotFound
	}
	// Row().Scan rather than Take: the destination is one scalar, and GORM
	// reads a scalar destination as a row struct.
	var key uint64
	err := tx.WithContext(ctx).Table(table).
		Select("id").Where("public_id = ?", raw).Row().Scan(&key)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, model.ErrNotFound
	}
	if err != nil {
		return 0, err
	}
	return key, nil
}

// createWithPublicID inserts row, assigning it a fresh public ID and trying
// again when the unique index rejects that particular value.
//
// index names the constraint so a collision on the public ID can be told from
// every other uniqueness conflict -- a duplicate email, an idempotency key, a
// second live grant for one role. Those are the caller's answer and are
// returned unchanged; retrying one would hide a real conflict behind a
// generated value.
func createWithPublicID(ctx context.Context, tx *gorm.DB, index string, assign func([]byte), row any) error {
	for range publicIDAttempts {
		raw, err := newPublicIDBytes()
		if err != nil {
			return err
		}
		assign(raw)
		err = tx.WithContext(ctx).Create(row).Error
		if err == nil {
			return nil
		}
		if !isDuplicateOnIndex(err, index) {
			return err
		}
	}
	return ErrPublicIDExhausted
}

// isDuplicateKey reports whether the error is a unique-constraint violation.
func isDuplicateKey(err error) bool {
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return true
	}
	var mysqlErr *mysqldriver.MySQLError
	return errors.As(err, &mysqlErr) && mysqlErr.Number == mysqlDuplicateEntry
}

// isDuplicateOnIndex reports whether the error is a unique-constraint violation
// on the named index.
//
// MySQL names the index in the 1062 message and nowhere else in the error, so
// this reads it from there. A qualified name ("task.uq_task_public_id" on MySQL
// 8) and a bare one (5.7) both contain the index name, which is why the check
// is a substring and not an equality.
func isDuplicateOnIndex(err error, index string) bool {
	if !isDuplicateKey(err) {
		return false
	}
	var mysqlErr *mysqldriver.MySQLError
	if !errors.As(err, &mysqlErr) {
		// gorm.ErrDuplicatedKey without a driver error underneath says nothing
		// about which index. Treating that as a match would retry a duplicate
		// email three times and then report the wrong failure.
		return false
	}
	return strings.Contains(mysqlErr.Message, index)
}

// publicIDForKey is lookupKey backwards: the handle a row key belongs to.
//
// It exists for the reads that must not join. A locking read that joins its
// parent locks the parent row too, which turns one row's contention into the
// whole account's; resolving the handle in a second, non-locking statement
// keeps the lock where the write is.
func publicIDForKey(ctx context.Context, tx *gorm.DB, table string, key uint64) (string, error) {
	var raw []byte
	err := tx.WithContext(ctx).Table(table).
		Select("public_id").Where("id = ?", key).Row().Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return "", model.ErrNotFound
	}
	if err != nil {
		return "", err
	}
	return util.FormatPublicID(raw), nil
}

// mustParsePublicID is for a handle a caller already resolved through
// lookupKey: it parsed once, so it parses again, and returning nil rather than
// failing keeps the caller's happy path free of an impossible branch.
func mustParsePublicID(publicID string) []byte {
	raw, _ := util.ParsePublicID(publicID)
	return raw
}
