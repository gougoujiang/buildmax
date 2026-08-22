package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"

	mysqldriver "github.com/go-sql-driver/mysql"
)

// MySQL error numbers for "the schema in the DSN is not there".
//
// 1049 is the plain answer. 1044 is the same fact seen by a user with no
// privilege on that name — the server refuses to say whether it exists — which
// is what a deployment gets whenever the account is granted one schema by name
// and the DSN asks for another.
const (
	errUnknownDatabase = 1049
	errDatabaseDenied  = 1044
)

// schemaNamePattern is what may be interpolated into CREATE DATABASE. The
// schema name comes from server.yaml, which is operator-controlled, but it
// reaches SQL as an identifier that cannot be a placeholder.
var schemaNamePattern = regexp.MustCompile(`^[A-Za-z0-9_]+$`)

// ensureDatabase creates the schema named in dsn when it does not exist.
//
// The server owns its schema: AutoMigrate already creates every table on first
// start, and a first start that fails only because the empty database around
// them is missing is a worse experience for no gain in safety. It runs only
// after the connection failed for that reason, so a healthy deployment never
// issues the statement, and a deployment whose account cannot create schemas
// gets an error naming the statement to run by hand.
func ensureDatabase(ctx context.Context, dsn string) error {
	cfg, err := mysqldriver.ParseDSN(dsn)
	if err != nil {
		return fmt.Errorf("parse dsn: %w", err)
	}
	name := cfg.DBName
	if name == "" {
		return errors.New("dsn names no database")
	}
	if !schemaNamePattern.MatchString(name) {
		return fmt.Errorf("database name %q is not a plain identifier", name)
	}

	serverCfg := *cfg
	serverCfg.DBName = ""
	conn, err := sql.Open("mysql", serverCfg.FormatDSN())
	if err != nil {
		return fmt.Errorf("connect without a database: %w", err)
	}
	defer conn.Close()

	statement := "CREATE DATABASE IF NOT EXISTS `" + name + "` CHARACTER SET utf8mb4"
	if _, err := conn.ExecContext(ctx, statement); err != nil {
		return fmt.Errorf("%s: %w", statement, err)
	}
	return nil
}

// missingDatabase reports whether err says the DSN's schema is not there.
func missingDatabase(err error) bool {
	var mysqlErr *mysqldriver.MySQLError
	if !errors.As(err, &mysqlErr) {
		return false
	}
	return mysqlErr.Number == errUnknownDatabase || mysqlErr.Number == errDatabaseDenied
}
