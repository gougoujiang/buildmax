package db

import (
	"context"

	"gorm.io/gorm"
)

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
