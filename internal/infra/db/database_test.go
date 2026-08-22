package db

import (
	"context"
	"errors"
	"strings"
	"testing"

	mysqldriver "github.com/go-sql-driver/mysql"
)

func TestMissingDatabase(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"unknown database", &mysqldriver.MySQLError{Number: errUnknownDatabase}, true},
		// What an account granted one schema by name is told about any other.
		{"access denied to database", &mysqldriver.MySQLError{Number: errDatabaseDenied}, true},
		{"wrapped", errors.Join(errors.New("open mysql"), &mysqldriver.MySQLError{Number: errUnknownDatabase}), true},
		{"bad credentials", &mysqldriver.MySQLError{Number: 1045}, false},
		{"unreachable", errors.New("dial tcp: connection refused"), false},
		{"nil", nil, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := missingDatabase(tc.err); got != tc.want {
				t.Errorf("missingDatabase(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}

// A schema name reaches CREATE DATABASE as an identifier, where a placeholder
// cannot go, so anything but a plain identifier is refused before it gets near
// the connection.
func TestEnsureDatabaseRefusesUnsafeName(t *testing.T) {
	err := ensureDatabase(context.Background(), "buildmax:buildmax@tcp(127.0.0.1:3306)/buildmax`; DROP DATABASE x")
	if err == nil {
		t.Fatal("ensureDatabase accepted a name that is not an identifier")
	}
	if !strings.Contains(err.Error(), "plain identifier") {
		t.Errorf("error = %v, want it to name the rule that was broken", err)
	}
}

func TestEnsureDatabaseNeedsADatabaseName(t *testing.T) {
	if err := ensureDatabase(context.Background(), "buildmax:buildmax@tcp(127.0.0.1:3306)/"); err == nil {
		t.Fatal("ensureDatabase accepted a DSN that names no database")
	}
}
