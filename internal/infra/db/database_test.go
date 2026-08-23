package db

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

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

// A DATETIME carries no zone, so the value that comes back is only correct if
// the driver and the session agree on how to read it. utcDSN is where that is
// decided, and it has to hold whatever the caller's DSN said — an operator's
// server.yaml, or BUILDMAX_TEST_DSN.
func TestUTCDSNPinsBothEndsOfTheConversion(t *testing.T) {
	for _, in := range []string{
		"buildmax:pw@tcp(127.0.0.1:3306)/buildmax?charset=utf8mb4&parseTime=True",
		// The awkward case: a caller that asked for something else.
		"buildmax:pw@tcp(127.0.0.1:3306)/buildmax?parseTime=false&loc=Local&time_zone=%27%2B08%3A00%27",
	} {
		got, err := utcDSN(in)
		if err != nil {
			t.Fatalf("utcDSN(%q): %v", in, err)
		}
		cfg, err := mysqldriver.ParseDSN(got)
		if err != nil {
			t.Fatalf("reparse %q: %v", got, err)
		}
		if cfg.Loc != time.UTC {
			t.Errorf("loc = %v, want UTC", cfg.Loc)
		}
		if !cfg.ParseTime {
			t.Error("parseTime is off; a DATETIME would arrive as bytes rather than a time.Time")
		}
		if want := "'+00:00'"; cfg.Params["time_zone"] != want {
			t.Errorf("time_zone = %q, want %q so NOW() agrees with what the server writes",
				cfg.Params["time_zone"], want)
		}
	}
}

func TestUTCDSNReportsAnUnparseableDSN(t *testing.T) {
	if _, err := utcDSN("not a dsn"); err == nil {
		t.Fatal("utcDSN accepted a DSN the driver cannot parse")
	}
}
