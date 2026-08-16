package db

import (
	"context"
	"os"
	"testing"

	"github.com/gougoujiang/buildmax/internal/config"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

// TestMigrationsAreWellFormed runs without a database, because the properties
// it checks are what make the list safe to append to and are easy to break in
// a hurry.
func TestMigrationsAreWellFormed(t *testing.T) {
	seen := make(map[string]bool, len(migrations))
	for i, m := range migrations {
		if m.ID == "" {
			t.Errorf("migration %d has no ID; schema_migration cannot record it", i)
		}
		if m.Apply == nil {
			t.Errorf("migration %q has no Apply", m.ID)
		}
		// A duplicate ID means the second one is recorded as already applied
		// and silently never runs.
		if seen[m.ID] {
			t.Errorf("duplicate migration ID %q", m.ID)
		}
		seen[m.ID] = true
	}
}

// TestMigrationIDsAreStable is the append-only guard. These IDs are recorded in
// every deployed database; renaming one makes that migration run a second time
// on every existing install, and reordering changes what an upgraded database
// gets relative to a fresh one. Adding an entry to the end is the only edit
// this test should ever need.
func TestMigrationIDsAreStable(t *testing.T) {
	want := []string{
		"0001_artifact_tables_to_task_run_artifact",
		"0002_task_run_output_file_to_task_run_artifact",
	}
	if len(migrations) < len(want) {
		t.Fatalf("migrations shrank to %d; entries are append-only", len(migrations))
	}
	for i, id := range want {
		if migrations[i].ID != id {
			t.Errorf("migration %d = %q, want %q — existing IDs and their order are permanent",
				i, migrations[i].ID, id)
		}
	}
}

// TestWarnIfSchemaIsAhead exercises the N-1 promise: a binary one release
// behind a migrated database must keep running, so an unknown migration is a
// warning rather than a refusal. The function returning normally is the
// assertion — a future refusal here would break every rolling upgrade.
func TestWarnIfSchemaIsAhead(t *testing.T) {
	warnIfSchemaIsAhead(map[string]bool{
		"0001_artifact_tables_to_task_run_artifact": true,
		"9999_from_a_newer_release":                 true,
	})
	warnIfSchemaIsAhead(nil)
}

func testDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv(config.EnvKeyBuildmaxTestDSN)
	if dsn == "" {
		t.Skip(config.EnvKeyBuildmaxTestDSN + " not set, skipping migration integration test")
	}
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	return db
}

// TestRunMigrationsRecordsAndSkips asserts the property the schema_migration
// table exists for: a migration runs once, and a second start does not run it
// again.
func TestRunMigrationsRecordsAndSkips(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	if err := runMigrations(ctx, db); err != nil {
		t.Fatalf("first run: %v", err)
	}
	applied, err := appliedMigrations(ctx, db)
	if err != nil {
		t.Fatalf("appliedMigrations: %v", err)
	}
	for _, m := range migrations {
		if !applied[m.ID] {
			t.Errorf("migration %q was not recorded", m.ID)
		}
	}

	// A second start must be a no-op. Applying twice is what the recording
	// prevents, and an Apply that ran again would be visible here as an error
	// from a repeated DDL statement.
	if err := runMigrations(ctx, db); err != nil {
		t.Fatalf("second run should be a no-op: %v", err)
	}
	after, err := appliedMigrations(ctx, db)
	if err != nil {
		t.Fatalf("appliedMigrations: %v", err)
	}
	if len(after) != len(applied) {
		t.Errorf("second run changed the record count: %d then %d", len(applied), len(after))
	}
}
