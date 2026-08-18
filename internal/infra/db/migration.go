package db

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/gougoujiang/buildmax/internal/core/model"
	"github.com/gougoujiang/buildmax/internal/util"
	"gorm.io/gorm"
)

// schemaMigrationRow records one applied migration. Rows are never deleted:
// the table is the answer to "what has this database had done to it", and a
// missing row means a migration runs again.
type schemaMigrationRow struct {
	// ID is the migration's permanent identifier. 191 characters is the
	// longest indexable varchar under MySQL's utf8mb4 index limit.
	ID        string `gorm:"column:id;type:varchar(191);primaryKey"`
	AppliedAt int64  `gorm:"column:applied_at;not null"`
}

func (schemaMigrationRow) TableName() string { return "schema_migration" }

// AppliedMigrations implements model.SchemaStore.
//
// AutoMigrate's additive DDL is not in this table — only the steps a struct
// cannot express. A reader should take it as "what has been done beyond the row
// structs", not as a complete schema version.
func (s *Store) AppliedMigrations(ctx context.Context) ([]model.SchemaMigration, error) {
	var rows []schemaMigrationRow
	if err := s.db.WithContext(ctx).Order("applied_at ASC, id ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]model.SchemaMigration, 0, len(rows))
	for _, row := range rows {
		out = append(out, model.SchemaMigration{ID: row.ID, AppliedAt: row.AppliedAt})
	}
	return out, nil
}

// Migration is one forward step in the schema's history.
//
// There is deliberately no Down. BuildMax's schema moves forward only:
// compatibility with the previous release is carried by each change being
// additive, not by an undo path. A rollback is a rollback of the binary, and
// the schema it left behind must keep serving it — see
// docs/contribute/architecture/data-model.md.
type Migration struct {
	// ID is permanent and unique. It is what schema_migration records, so
	// renaming one makes it run a second time on every existing database.
	ID string
	// Apply performs the change.
	//
	// It must tolerate being run against a database that already has the
	// change: a crash between applying and recording leaves the migration
	// pending, and the next start will retry it.
	Apply func(ctx context.Context, db *gorm.DB) error
}

// migrations run in the order listed.
//
// Append only. An existing entry has already been recorded in deployments, so
// editing or reordering one changes what an upgraded database gets relative to
// a fresh one — which is exactly the divergence this list exists to prevent.
var migrations = []Migration{
	{
		ID:    "0001_artifact_tables_to_task_run_artifact",
		Apply: migrateFromArtifactTables,
	},
	{
		ID:    "0002_task_run_output_file_to_task_run_artifact",
		Apply: migrateTaskRunOutputFileToArtifact,
	},
	{
		ID:    "0003_seed_first_agent_and_workflow_revision",
		Apply: seedFirstRevisions,
	},
}

// runMigrations applies every migration this binary knows and the database has
// not recorded.
//
// AutoMigrate has already run by this point and owns additive DDL — new tables,
// new columns, new indexes. This list owns everything AutoMigrate cannot
// express: backfills, drops, and renames. Keeping the split sharp is what makes
// "the row structs are the schema" true while still leaving a record of the
// changes a struct cannot describe.
func runMigrations(ctx context.Context, db *gorm.DB) error {
	if err := db.WithContext(ctx).AutoMigrate(&schemaMigrationRow{}); err != nil {
		return fmt.Errorf("create schema_migration: %w", err)
	}
	applied, err := appliedMigrations(ctx, db)
	if err != nil {
		return err
	}
	for _, m := range migrations {
		if applied[m.ID] {
			continue
		}
		if err := m.Apply(ctx, db); err != nil {
			return fmt.Errorf("migration %s: %w", m.ID, err)
		}
		row := schemaMigrationRow{ID: m.ID, AppliedAt: time.Now().Unix()}
		if err := db.WithContext(ctx).Create(&row).Error; err != nil {
			return fmt.Errorf("record migration %s: %w", m.ID, err)
		}
		slog.Info("applied schema migration", "id", m.ID)
	}
	warnIfSchemaIsAhead(applied)
	return nil
}

func appliedMigrations(ctx context.Context, db *gorm.DB) (map[string]bool, error) {
	var rows []schemaMigrationRow
	if err := db.WithContext(ctx).Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("read schema_migration: %w", err)
	}
	out := make(map[string]bool, len(rows))
	for _, r := range rows {
		out[r.ID] = true
	}
	return out, nil
}

// warnIfSchemaIsAhead reports migrations the database has and this binary does
// not.
//
// It warns rather than refuses, because that state is the N-1 promise working
// as intended: a server one release behind a migrated database is supposed to
// keep serving. A server several releases behind has no such promise, and this
// log line is the only signal an operator gets that they are in that position.
func warnIfSchemaIsAhead(applied map[string]bool) {
	known := make(map[string]bool, len(migrations))
	for _, m := range migrations {
		known[m.ID] = true
	}
	var unknown []string
	for id := range applied {
		if !known[id] {
			unknown = append(unknown, id)
		}
	}
	if len(unknown) == 0 {
		return
	}
	slog.Warn("database schema is ahead of this binary; supported one release back, not more",
		"unknown_migrations", unknown)
}

// migrateFromArtifactTables copies artifact_item into task_run_artifact (by task_run_id from artifact),
// then drops artifact and artifact_item, and drops legacy task artifact columns if present.
func migrateFromArtifactTables(ctx context.Context, db *gorm.DB) error {
	var exists int
	err := db.WithContext(ctx).Raw("SELECT 1 FROM information_schema.tables WHERE table_schema = DATABASE() AND table_name = 'artifact' LIMIT 1").Scan(&exists).Error
	if err != nil || exists == 0 {
		return nil
	}
	err = db.WithContext(ctx).Exec(`INSERT INTO task_run_artifact (task_run_id, relative_path)
		SELECT a.task_run_id, i.relative_path FROM artifact_item i INNER JOIN artifact a ON i.artifact_id = a.artifact_id
		ON DUPLICATE KEY UPDATE relative_path = VALUES(relative_path)`).Error
	if err != nil {
		return err
	}
	_ = db.WithContext(ctx).Exec("DROP TABLE IF EXISTS artifact_item").Error
	_ = db.WithContext(ctx).Exec("DROP TABLE IF EXISTS artifact").Error
	_ = db.WithContext(ctx).Exec("ALTER TABLE task DROP COLUMN last_artifact_id").Error
	_ = db.WithContext(ctx).Exec("ALTER TABLE task DROP COLUMN artifact_seq").Error
	_ = db.WithContext(ctx).Exec("ALTER TABLE chat DROP COLUMN last_artifact_id").Error
	_ = db.WithContext(ctx).Exec("ALTER TABLE chat DROP COLUMN artifact_seq").Error
	return nil
}

// migrateTaskRunOutputFileToArtifact copies task_run_output_file into task_run_artifact and drops the old table.
func migrateTaskRunOutputFileToArtifact(ctx context.Context, db *gorm.DB) error {
	var exists int
	err := db.WithContext(ctx).Raw("SELECT 1 FROM information_schema.tables WHERE table_schema = DATABASE() AND table_name = 'task_run_output_file' LIMIT 1").Scan(&exists).Error
	if err != nil || exists == 0 {
		return nil
	}
	err = db.WithContext(ctx).Exec(`INSERT INTO task_run_artifact (task_run_id, relative_path)
		SELECT task_run_id, relative_path FROM task_run_output_file`).Error
	if err != nil {
		return err
	}
	return db.WithContext(ctx).Exec("DROP TABLE task_run_output_file").Error
}

// seedFirstRevisions gives every agent and workflow that predates revision
// history a revision 1 holding its current content.
//
// The alternative — an empty history until the next edit — would silently lose
// the definition an edit replaced, which is the one thing history exists to
// keep. The seeded row is an approximation and is documented as one: its author
// is whoever created the record, not whoever last changed it, and its timestamp
// is when the content last moved, not when this content was written.
//
// It only inserts for records that have no revision at all, so a retry after a
// crash between applying and recording adds nothing.
func seedFirstRevisions(ctx context.Context, db *gorm.DB) error {
	if err := seedFirstAgentRevisions(ctx, db); err != nil {
		return err
	}
	return seedFirstWorkflowRevisions(ctx, db)
}

func seedFirstAgentRevisions(ctx context.Context, db *gorm.DB) error {
	var agents []agentRow
	err := db.WithContext(ctx).
		Where("NOT EXISTS (SELECT 1 FROM agent_revision WHERE agent_revision.agent_id = agent.agent_id)").
		Find(&agents).Error
	if err != nil {
		return err
	}
	if len(agents) == 0 {
		return nil
	}
	revisions := make([]agentRevisionRow, len(agents))
	for i := range agents {
		revisions[i] = agentRevisionRow{
			AgentRevisionID: util.NewPrefixedID(util.PrefixAgentRevision),
			AgentID:         agents[i].AgentID,
			Revision:        firstRevision(agents[i].Revision),
			Name:            agents[i].Name,
			Description:     agents[i].Description,
			Instructions:    agents[i].Instructions,
			CreatedBy:       agents[i].UserID,
			CreatedAt:       agents[i].CreatedAt,
		}
	}
	return db.WithContext(ctx).Create(&revisions).Error
}

func seedFirstWorkflowRevisions(ctx context.Context, db *gorm.DB) error {
	var workflows []workflowRow
	err := db.WithContext(ctx).
		Where("NOT EXISTS (SELECT 1 FROM workflow_revision WHERE workflow_revision.workflow_id = workflow.workflow_id)").
		Find(&workflows).Error
	if err != nil {
		return err
	}
	if len(workflows) == 0 {
		return nil
	}
	revisions := make([]workflowRevisionRow, len(workflows))
	for i := range workflows {
		createdAt := workflows[i].UpdatedAt
		if createdAt == 0 {
			createdAt = workflows[i].CreatedAt
		}
		revisions[i] = workflowRevisionRow{
			WorkflowRevisionID: util.NewPrefixedID(util.PrefixWorkflowRevision),
			WorkflowID:         workflows[i].WorkflowID,
			Revision:           firstRevision(workflows[i].Revision),
			Name:               workflows[i].Name,
			Description:        workflows[i].Description,
			Definition:         workflows[i].Definition,
			Status:             workflows[i].Status,
			CreatedBy:          workflows[i].CreatedBy,
			CreatedAt:          createdAt,
		}
	}
	return db.WithContext(ctx).Create(&revisions).Error
}

// firstRevision returns the revision number a seeded row should carry. The
// column defaults to 1, so a row that reports less than that predates the
// column and is still its own first revision.
func firstRevision(current int) int {
	if current < 1 {
		return 1
	}
	return current
}
