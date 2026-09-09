- A TaskRun's durable trace now records who or what started it and why
  (created_by, created_by_type, trigger_source, retry_of_task_run_id) on its
  run_start line, so a downloaded trace explains its own origin.
