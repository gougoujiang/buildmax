-- BuildMax migration: rename background execution tables from chat* to task*
-- Safe to run with the mysql CLI. Existing IDs are preserved; only table/column/index names move.

DROP PROCEDURE IF EXISTS buildmax_exec_if;

DELIMITER //
CREATE PROCEDURE buildmax_exec_if(IN stmt TEXT, IN should_run BOOLEAN)
BEGIN
  IF should_run THEN
    SET @sql = stmt;
    PREPARE s FROM @sql;
    EXECUTE s;
    DEALLOCATE PREPARE s;
  END IF;
END//
DELIMITER ;

SET FOREIGN_KEY_CHECKS = 0;

CALL buildmax_exec_if(
  'RENAME TABLE `chat` TO `task`',
  EXISTS(
    SELECT 1
    FROM information_schema.tables
    WHERE table_schema = DATABASE() AND table_name = 'chat'
  ) AND NOT EXISTS(
    SELECT 1
    FROM information_schema.tables
    WHERE table_schema = DATABASE() AND table_name = 'task'
  )
);

CALL buildmax_exec_if(
  'RENAME TABLE `chat_run` TO `task_run`',
  EXISTS(
    SELECT 1
    FROM information_schema.tables
    WHERE table_schema = DATABASE() AND table_name = 'chat_run'
  ) AND NOT EXISTS(
    SELECT 1
    FROM information_schema.tables
    WHERE table_schema = DATABASE() AND table_name = 'task_run'
  )
);

CALL buildmax_exec_if(
  'RENAME TABLE `chat_run_artifact` TO `task_run_artifact`',
  EXISTS(
    SELECT 1
    FROM information_schema.tables
    WHERE table_schema = DATABASE() AND table_name = 'chat_run_artifact'
  ) AND NOT EXISTS(
    SELECT 1
    FROM information_schema.tables
    WHERE table_schema = DATABASE() AND table_name = 'task_run_artifact'
  )
);

CALL buildmax_exec_if(
  'ALTER TABLE `task` CHANGE COLUMN `chat_id` `task_id` VARCHAR(64) NOT NULL',
  EXISTS(
    SELECT 1
    FROM information_schema.columns
    WHERE table_schema = DATABASE() AND table_name = 'task' AND column_name = 'chat_id'
  ) AND NOT EXISTS(
    SELECT 1
    FROM information_schema.columns
    WHERE table_schema = DATABASE() AND table_name = 'task' AND column_name = 'task_id'
  )
);

CALL buildmax_exec_if(
  'ALTER TABLE `task_run` CHANGE COLUMN `chat_run_id` `task_run_id` VARCHAR(64) NOT NULL',
  EXISTS(
    SELECT 1
    FROM information_schema.columns
    WHERE table_schema = DATABASE() AND table_name = 'task_run' AND column_name = 'chat_run_id'
  ) AND NOT EXISTS(
    SELECT 1
    FROM information_schema.columns
    WHERE table_schema = DATABASE() AND table_name = 'task_run' AND column_name = 'task_run_id'
  )
);

CALL buildmax_exec_if(
  'ALTER TABLE `task_run` CHANGE COLUMN `chat_id` `task_id` VARCHAR(64) NOT NULL',
  EXISTS(
    SELECT 1
    FROM information_schema.columns
    WHERE table_schema = DATABASE() AND table_name = 'task_run' AND column_name = 'chat_id'
  ) AND NOT EXISTS(
    SELECT 1
    FROM information_schema.columns
    WHERE table_schema = DATABASE() AND table_name = 'task_run' AND column_name = 'task_id'
  )
);

CALL buildmax_exec_if(
  'ALTER TABLE `task_run_artifact` CHANGE COLUMN `chat_run_id` `task_run_id` VARCHAR(64) NOT NULL',
  EXISTS(
    SELECT 1
    FROM information_schema.columns
    WHERE table_schema = DATABASE() AND table_name = 'task_run_artifact' AND column_name = 'chat_run_id'
  ) AND NOT EXISTS(
    SELECT 1
    FROM information_schema.columns
    WHERE table_schema = DATABASE() AND table_name = 'task_run_artifact' AND column_name = 'task_run_id'
  )
);

CALL buildmax_exec_if(
  'ALTER TABLE `task_run_artifact` RENAME INDEX `uq_chat_run_artifact_run_path` TO `uq_task_run_artifact_run_path`',
  EXISTS(
    SELECT 1
    FROM information_schema.statistics
    WHERE table_schema = DATABASE()
      AND table_name = 'task_run_artifact'
      AND index_name = 'uq_chat_run_artifact_run_path'
  ) AND NOT EXISTS(
    SELECT 1
    FROM information_schema.statistics
    WHERE table_schema = DATABASE()
      AND table_name = 'task_run_artifact'
      AND index_name = 'uq_task_run_artifact_run_path'
  )
);

SET FOREIGN_KEY_CHECKS = 1;

DROP PROCEDURE IF EXISTS buildmax_exec_if;
