# Design 068: Support rerun existing task (task_run, follow-up, run-scoped paths)

## Goal

Introduce **TaskRun** as the unit of execution. A task has one or more runs (first question + follow-ups). Task keeps denormalized "last run" state and one shared session_id. Each run has its own buildmax dir (`…/tasks/<taskID>/<runID>/buildmax/`) and artifacts under `artifacts/<taskID>/<runID>/<artifactID>`. Worker is invoked with `--task-run-id` only. Follow-up is POST `.../tasks/{id}/runs`; reject with 409 if a run is already in progress. Conversation is read from the latest run's buildmax. No migration; user truncates tables.

## Modules

| Module | Responsibility |
|--------|----------------|
| **internal/model** | TaskRun type; Task keeps denormalized last-run fields; Artifact adds TaskRunID. |
| **internal/storage/entity** | TaskRun table and TaskRunStore; Task CreateTask + first run; CreateTaskRun (reject if run in progress); Artifact by task_run_id; task denormalized update on run completion. |
| **internal/config** | RuntimeTaskBuildmaxDir(workspaceID, taskID, runID), ArtifactDir(..., runID); optional helpers. |
| **internal/storage/blob** | PersistStorage and ArtifactStorage add runID to task/artifact keys; keys.go TaskBuildmaxObjectKey(..., runID), ArtifactResultKey(..., runID). |
| **internal/executor** | WorkspacePaths adds runID to buildmax and artifact dirs; RunTask(task, run, ...) with session restore from previous run; TaskUpdater by run_id; upload buildmax per run. |
| **internal/executor (scheduler/runner)** | Scheduler polls GetNextPendingTaskRun, claims run, spawns worker with --task-run-id; LocalRunner/K8sJobRunner pass run_id. |
| **internal/executor (worker API client)** | GetWorkerTaskRun(ctx, baseURL, token, runID); WorkerHTTPUpdater by run_id (PATCH run, update task). |
| **internal/server** | POST tasks → task + first run; POST tasks/{id}/runs (409 if run in progress); GET/PATCH /api/worker/task-runs/{run_id}; conversation from last run's buildmax; list artifacts with task_run_id. |
| **internal/workercmd** | CLI flag --task-run-id; fetch run+task by run_id; call RunTask(task, run, ...). |
| **cmd/buildmax-worker** | Parse --task-run-id only. |
| **portal** | Task detail: follow-up input, POST runs, poll until run SUCCEEDED/FAILED then refresh. |

## Structure

### internal/model

- **TaskRun** (new struct, table `task_run`): RunID (PK), TaskID (FK), Input, Status, Output, ErrorMessage, StartedAt, EndedAt, SessionID (set when run starts), WorkerType, K8sJobName, K8sJobCreatedAt, CreatedAt. GORM + json snake_case. Status: PENDING → SCHEDULED → RUNNING → SUCCEEDED | FAILED.
- **Task**: Keep WorkspaceID, ProjectID, CreatedBy, CreatedAt, SessionID. Add (or keep) denormalized: Status, Output, StartedAt, EndedAt, ErrorMessage, WorkerType, K8sJobName, K8sJobCreatedAt, LastArtifactID, **LastRunID** (string, FK to task_run.run_id). LastRunID identifies which run's buildmax to use for conversation and which run's state is shown.
- **Artifact**: Add TaskRunID (required, FK). Keep TaskID for workspace scoping. Blob path uses run: `artifacts/<taskID>/<runID>/<artifactID>`.
- **ArtifactWithTask** (DTO): Add TaskRunID, optionally RunInputSnippet (from task_run.input); list still by workspace/task.

### internal/storage/entity

- **TaskRunStore** (new interface): CreateTaskRun(ctx, taskID, input, createdBy) (*TaskRun, error) — returns 409 or ErrRunInProgress if task has any run in PENDING/SCHEDULED/RUNNING; GetNextPendingTaskRun(ctx) (*TaskRun, error) — oldest PENDING by created_at; GetTaskRun(ctx, runID) (*TaskRun, error); GetTaskRunWithTask(ctx, runID) (*TaskRun, *Task, error) for worker; UpdateTaskRunStatus(ctx, runID, status, startedAt, endedAt, output, errMsg, sessionID) error; UpdateTaskRunWorkerInfo(ctx, runID, workerType, k8sJobName, k8sJobCreatedAt) error; OnRunComplete(ctx, runID, artifactID, relativePath) error — create artifact with item, update task denormalized fields (status, output, started_at, ended_at, error_message, worker_type, k8s_*, last_artifact_id, last_run_id) and task.session_id if run set it.
- **TaskStore**: CreateTask(ctx, workspaceID, projectID, input, createdBy) — in one transaction create Task (status PENDING, last_run_id nil initially) and first TaskRun (input, PENDING), then set task.last_run_id to that run (or set denormalized from run in same tx). GetTask, ListTasksByWorkspace unchanged; task rows already have denormalized fields. Remove GetNextPendingTask; scheduler uses TaskRunStore.GetNextPendingTaskRun.
- **ArtifactStore**: CreateArtifactWithItem(ctx, taskID, taskRunID, artifactID, seq, relativePath) — artifact row has task_run_id; update task.last_artifact_id. ListArtifactsByWorkspace: JOIN task_run for run context; filter by task_id/task_run_id as needed; ArtifactWithTask includes task_run_id and optionally run input snippet.
- **Store**: Implements TaskRunStore; AutoMigrate adds TaskRun, adds Artifact.TaskRunID and Task.LastRunID (and denormalized columns if not already present).

### internal/config

- **RuntimeTaskBuildmaxDir(workspaceID, taskID, runID string) string** — return `WorkspacesDir()/workspaceID/tasks/taskID/runID/buildmax`. Keep existing RuntimeTaskBuildmaxDir(workspaceID, taskID) for compatibility only if needed, or remove and replace all call sites with three-arg version.
- **RuntimeWorkspaceDir(workspaceID, taskID)** — unchanged (still task-level; worker materializes once per task dir; run-level buildmax is under taskDir/runID/buildmax). So runtime layout: `…/tasks/<taskID>/ws`, `…/tasks/<taskID>/<runID>/buildmax`. Optional: **RuntimeTaskRunDir(workspaceID, taskID, runID)** = `…/tasks/<taskID>/<runID>`.
- **ArtifactDir(workspaceID, taskID, runID, artifactID string) string** — return `…/artifacts/<taskID>/<runID>/<artifactID>`.

### internal/storage/blob

- **keys.go**: TaskBuildmaxObjectKey(prefix, workspaceID, taskID, runID, relPath) — path `prefix/workspaceID/tasks/taskID/runID/buildmax/relPath`. ArtifactResultKey(prefix, workspaceID, taskID, runID, artifactID) — `prefix/workspaceID/artifacts/taskID/runID/artifactID/result.md`.
- **PersistStorage interface**: PutTaskBuildmax(ctx, workspaceID, taskID, runID, relPath, r); GetTaskBuildmax(ctx, workspaceID, taskID, runID, relPath). Existing two-arg (taskID only) signatures removed or deprecated; all call sites pass runID.
- **ArtifactStorage interface**: PutResult(ctx, workspaceID, taskID, runID, artifactID, data); GetResult(ctx, workspaceID, taskID, runID, artifactID).
- **S3 and LocalFS** implementations updated to use runID in paths.

### internal/executor

- **WorkspacePaths interface**: RuntimeTaskBuildmaxDir(workspaceID, taskID, runID string) string; ArtifactDir(workspaceID, taskID, runID, artifactID string) string. RuntimeWorkspaceDir and RuntimeTaskWSDir stay (workspaceID, taskID) — one ws dir per task, shared across runs.
- **RunTask(ctx, task *entity.Task, run *entity.TaskRun, sessionID string, paths WorkspacePaths, persist blob.PersistStorage, artifactStorage blob.ArtifactStorage, updater TaskRunUpdater) error**: Buildmax dir = paths.RuntimeTaskBuildmaxDir(task.WorkspaceID, task.TaskID, run.RunID). If task.SessionID != nil, restore session: GetTaskBuildmax(ctx, task.WorkspaceID, task.TaskID, previousRunID, "sessions/"+*task.SessionID+".json") — previousRunID = run from same task with latest ended_at (or store "session_run_id" on task); write to current run's buildmax dir. Then run buildmax -p run.Input --session-id (task.SessionID or new). On success: artifact path uses run.RunID; updater.OnRunComplete(runID, artifact...). Upload buildmax with PutTaskBuildmax(..., taskID, run.RunID, relPath).
- **TaskRunUpdater interface** (replaces TaskUpdater): UpdateRunStatus(ctx, runID, status, startedAt, endedAt, output, errMsg, sessionID) error; OnRunComplete(ctx, runID, artifactID, relativePath) error (server implements: create artifact, update task denormalized + session_id if needed).
- **uploadTaskBuildmax**: Accept (ctx, buildmaxDir, workspaceID, taskID, runID, persist); call PutTaskBuildmax(..., taskID, runID, relPath).
- **Scheduler**: Holds TaskRunStore and WorkerRunner. Loop: run, err := taskRunStore.GetNextPendingTaskRun(ctx); claim with UpdateTaskRunStatusIf(ctx, run.RunID, PENDING, SCHEDULED); runner.Run(ctx, run) with run only (runner passes --task-run-id run.RunID). On success: UpdateTaskRunWorkerInfo(ctx, run.RunID, ...). On failure: revert run to PENDING.
- **WorkerRunner interface**: Run(ctx context.Context, run entity.TaskRun) (workerType string, k8sJobName *string, k8sJobCreatedAt *int64, err error). LocalRunner: exec worker with "--task-run-id", run.RunID. K8sJobRunner: Job args ["--task-run-id", run.RunID]; job name can use run.RunID for uniqueness.
- **worker_api.go**: GetWorkerTaskRun(ctx, baseURL, token, runID) (*TaskRun, *Task, error). WorkerHTTPUpdater: UpdateRunStatus(ctx, runID, ...); OnRunComplete(ctx, runID, artifactID, relativePath). PATCH /api/worker/task-runs/{run_id} body includes status, session_id, started_at, ended_at, output, error_message, artifact; server updates run and on terminal status calls store.OnRunComplete.

### Session restore (executor)

- Task stores SessionID (set by first run). To restore for a follow-up run we need the session file from a previous run. Option: store on task the **run_id that last wrote the session** (e.g. LastRunID is that run). So when building "previous run" buildmax path we use task.LastRunID (if this run is not the first, LastRunID points to the previous run). But wait: before current run completes, task.LastRunID might already be the previous run. So for run N (N>1), restore from task.LastRunID's buildmax (the run that last completed). If task.LastRunID is nil or equals current run.RunID (shouldn't happen), no restore. So: if task.SessionID != nil && task.LastRunID != nil && task.LastRunID != run.RunID, GetTaskBuildmax(ctx, workspaceID, taskID, task.LastRunID, "sessions/"+*task.SessionID+".json") and write to current run's buildmax dir.
- When run completes (SUCCEEDED), server sets task.LastRunID = run.RunID and task.SessionID = run.SessionID if run set it. So next run will restore from this run's buildmax.

### internal/server

- **Routes**: POST `/api/workspaces/{id}/tasks` — create task + first run (unchanged body). POST `/api/workspaces/{id}/tasks/{task_id}/runs` — body `{"input":"..."}`; create run (409 if run in progress). GET/PATCH `/api/worker/task-runs/{run_id}` — worker auth; GET returns run + task; PATCH updates run and on completion updates task.
- **createWorkspaceTaskHandler**: Call TaskStore.CreateTask (which creates task + first run). Response: task with denormalized status (PENDING from first run).
- **createTaskRunHandler** (new): Parse task_id from path, body input. TaskRunStore.CreateTaskRun(ctx, taskID, input, userID). If err == ErrRunInProgress return 409. Return 201 with run or task+run id.
- **getWorkerTaskRunHandler**: By run_id; return run + task (workspace_id, project_id, session_id, run input, etc.) for worker.
- **patchWorkerTaskRunHandler**: Update run status/fields; if status SUCCEEDED/FAILED call OnRunComplete (artifact if SUCCEEDED); OnRunComplete updates task denormalized and session_id.
- **getTaskConversationHandler**: Get task; if task.SessionID == nil 404. If task.LastRunID == nil 404 (no run yet). Read session from GetTaskBuildmax(ctx, workspaceID, taskID, task.LastRunID, "sessions/"+sessionID+".json"). Fallback to local path using config.RuntimeTaskBuildmaxDir(workspaceID, taskID, task.LastRunID).
- **List/Get task**: Task response already has denormalized fields (status, output, ...) from task row. No change to response shape except ensure last_run_id in response if needed for UI.
- **Artifact list/content**: List: ArtifactWithTask includes task_run_id. Content: GetResult(..., taskID, runID, artifactID) — runID from artifact row.

### internal/workercmd

- **RunWorker(ctx, runID string)**: Get run+task via GetWorkerTaskRun(ctx, baseURL, token, runID). Validate run status SCHEDULED. Mark RUNNING with session_id (new or task's). Build paths with run.RunID; persist and artifact storage use run-scoped paths. executor.RunTask(ctx, task, run, sessionID, paths, persist, artifactStorage, updater). Updater is WorkerHTTPUpdater by run_id.
- **main**: Flag --task-run-id (required). Call RunWorker(ctx, runID).

### cmd/buildmax-worker

- main.go: Parse --task-run-id; pass to workercmd.RunWorker.

### portal

- **TaskDetail**: Add "Follow-up" section: textarea + "Send". On submit: POST `/api/workspaces/{workspaceId}/tasks/{taskId}/runs` with `{ input }`. Response includes run_id. Then poll GET task (or GET run by id) until run status is SUCCEEDED or FAILED; then refetch task and conversation (GET task, GET conversation).
- **API client**: add createTaskRun(workspaceId, taskId, body, token). Optional getTaskRun(workspaceId, taskId, runId) if polling by run.

## Method design

### entity.TaskRunStore

- **CreateTaskRun(ctx, taskID, input, createdBy) (*TaskRun, error)** — Insert run (PENDING). If any run for task has status in (PENDING, SCHEDULED, RUNNING), return ErrRunInProgress (caller returns 409). Return new run.
- **GetNextPendingTaskRun(ctx) (*TaskRun, error)** — SELECT * FROM task_run WHERE status = 'PENDING' ORDER BY created_at ASC LIMIT 1.
- **GetTaskRun(ctx, runID) (*TaskRun, error)** — By run_id.
- **GetTaskRunWithTask(ctx, runID) (*TaskRun, *Task, error)** — Get run; get task by run.TaskID; return both.
- **UpdateTaskRunStatusIf(ctx, runID, expectedStatus, newStatus, startedAt, endedAt, output, errMsg, sessionID) (bool, error)** — Atomic claim (PENDING→SCHEDULED) or worker (SCHEDULED→RUNNING).
- **UpdateTaskRunStatus(ctx, runID, status, ...)** — Unconditional update.
- **UpdateTaskRunWorkerInfo(ctx, runID, workerType, k8sJobName, k8sJobCreatedAt) error**
- **OnRunComplete(ctx, runID, artifactID, relativePath) error** — In transaction: create artifact row with task_run_id, artifact_item; update task set last_run_id=runID, status=run.status, output, started_at, ended_at, error_message, worker_type, k8s_job_name, k8s_job_created_at, last_artifact_id; if run has session_id set task.session_id.

### entity.TaskStore

- **CreateTask(ctx, workspaceID, projectID, input, createdBy) (*Task, error)** — Insert task (status PENDING, last_run_id nil); insert first task_run (input, PENDING); set task.last_run_id = run.RunID and task denormalized from run (status PENDING). All in one transaction.

### blob.PersistStorage / ArtifactStorage

- **PutTaskBuildmax(ctx, workspaceID, taskID, runID, relPath string, r io.Reader) error**
- **GetTaskBuildmax(ctx, workspaceID, taskID, runID, relPath string) ([]byte, error)**
- **PutResult(ctx, workspaceID, taskID, runID, artifactID string, data []byte) error**
- **GetResult(ctx, workspaceID, taskID, runID, artifactID string) ([]byte, error)**

### executor.WorkspacePaths

- **RuntimeTaskBuildmaxDir(workspaceID, taskID, runID string) string**
- **ArtifactDir(workspaceID, taskID, runID, artifactID string) string**

### executor.TaskRunUpdater

- **UpdateRunStatus(ctx, runID, status string, startedAt, endedAt *int64, output, errMsg, sessionID *string) error**
- **OnRunComplete(ctx, runID, artifactID, relativePath string) error**

### executor.RunTask

- **RunTask(ctx, task *entity.Task, run *entity.TaskRun, sessionID string, paths WorkspacePaths, persist blob.PersistStorage, artifactStorage blob.ArtifactStorage, updater TaskRunUpdater) error** — Create run buildmax dir; optionally restore session from task.LastRunID buildmax; materialize ws; run buildmax -p run.Input --session-id; upload run buildmax; on success PutResult(..., run.RunID, ...), updater.OnRunComplete(run.RunID, ...).

### executor.WorkerRunner

- **Run(ctx, run entity.TaskRun) (workerType string, k8sJobName *string, k8sJobCreatedAt *int64, err error)** — Local: exec worker "--task-run-id", run.RunID. K8s: Job args ["--task-run-id", run.RunID].

### executor (scheduler loop)

- GetNextPendingTaskRun; UpdateTaskRunStatusIf(runID, PENDING, SCHEDULED); runner.Run(ctx, run); on err revert run to PENDING; on success UpdateTaskRunWorkerInfo(runID, ...).

## How they work together

1. **Create task**: User POSTs /api/workspaces/{id}/tasks with input. Server creates Task + first TaskRun (PENDING), sets task.last_run_id. Scheduler later picks run via GetNextPendingTaskRun, claims it (PENDING→SCHEDULED), spawns worker with --task-run-id. Worker GETs run+task, runs RunTask (no session restore), uploads buildmax to tasks/<taskID>/<runID>/buildmax, PATCHes run SUCCEEDED and OnRunComplete (artifact, task denormalized, task.session_id). Conversation is then read from GetTaskBuildmax(..., taskID, task.LastRunID, "sessions/"+sessionID+".json").

2. **Follow-up**: User POSTs /api/workspaces/{id}/tasks/{id}/runs with input. Server CreateTaskRun (409 if run in progress). Scheduler picks run, spawns worker. Worker GETs run+task; task.SessionID and task.LastRunID are set; restore session from GetTaskBuildmax(..., taskID, task.LastRunID, "sessions/"+*task.SessionID+".json") into current run's buildmax dir; RunTask runs buildmax with that session_id; uploads run buildmax; OnRunComplete updates task.LastRunID to this run so next conversation read uses this run's buildmax.

3. **Conversation**: GET .../tasks/{id}/conversation loads task, uses task.LastRunID and task.SessionID to read session file from persist or local path tasks/<taskID>/<lastRunID>/buildmax/sessions/<session_id>.json.

4. **Artifacts**: Each run produces one artifact; stored under artifacts/<taskID>/<runID>/<artifactID>. List artifacts returns task_run_id; content GET uses runID from artifact.

## Dependencies

- None new. k8s Job already uses args; we only change args to --task-run-id.

## Changes for review

| Package / File | Change |
|----------------|--------|
| **internal/model/models.go** | Add TaskRun struct (table task_run). Task: add LastRunID; keep denormalized status, output, started_at, ended_at, error_message, worker_type, k8s_*, last_artifact_id. Artifact: add TaskRunID (required). ArtifactWithTask: add task_run_id, optionally run_input_snippet. |
| **internal/storage/entity/types.go** | Export TaskRun (alias model.TaskRun). |
| **internal/storage/entity/interfaces.go** | Add TaskRunStore (CreateTaskRun, GetNextPendingTaskRun, GetTaskRun, GetTaskRunWithTask, UpdateTaskRunStatusIf, UpdateTaskRunStatus, UpdateTaskRunWorkerInfo, OnRunComplete). TaskStore: CreateTask creates task+first run; remove GetNextPendingTask. ArtifactStore: CreateArtifactWithItem(ctx, taskID, taskRunID, artifactID, seq, relativePath). |
| **internal/storage/entity/task_run.go** (new) | Implement TaskRunStore. |
| **internal/storage/entity/task.go** | CreateTask: create task row + first task_run row, set task.last_run_id and denormalized from run. Remove GetNextPendingTask. |
| **internal/storage/entity/artifact.go** | CreateArtifactWithItem(taskID, taskRunID, ...); artifact has task_run_id; list JOIN task_run for snippet. |
| **internal/storage/entity/store.go** | Store implements TaskRunStore; AutoMigrate TaskRun; Artifact.TaskRunID, Task.LastRunID. |
| **internal/config/config.go** | RuntimeTaskBuildmaxDir(workspaceID, taskID, runID); RuntimeTaskRunDir(workspaceID, taskID, runID) optional; ArtifactDir(workspaceID, taskID, runID, artifactID). |
| **internal/storage/blob/keys.go** | TaskBuildmaxObjectKey(prefix, workspaceID, taskID, runID, relPath); ArtifactResultKey(..., runID, artifactID). |
| **internal/storage/blob/interfaces.go** | PersistStorage: PutTaskBuildmax/GetTaskBuildmax add runID. ArtifactStorage: PutResult/GetResult add runID. |
| **internal/storage/blob/s3_persist.go, localfs_persist.go** | Implement new signatures with runID. |
| **internal/storage/blob/s3_artifact.go, localfs_artifact.go** | PutResult/GetResult add runID in path. |
| **internal/executor/paths.go** | WorkspacePaths: RuntimeTaskBuildmaxDir(workspaceID, taskID, runID), ArtifactDir(..., runID, artifactID). |
| **internal/executor/executor.go** | RunTask(task, run, sessionID, paths, persist, artifactStorage, updater TaskRunUpdater); restore session from task.LastRunID; buildmax dir and upload by runID; artifact by runID; updater.OnRunComplete. |
| **internal/executor/worker_api.go** | GetWorkerTaskRun(ctx, baseURL, token, runID); WorkerHTTPUpdater by run_id (UpdateRunStatus(runID, ...), OnRunComplete(runID, ...)). PATCH URL /api/worker/task-runs/{run_id}. |
| **internal/executor/scheduler.go** | Depend on TaskRunStore; GetNextPendingTaskRun; claim run; runner.Run(ctx, run); UpdateTaskRunWorkerInfo(runID, ...). |
| **internal/executor/runner.go** | WorkerRunner.Run(ctx, run entity.TaskRun); LocalRunner exec "--task-run-id", run.RunID. |
| **internal/executor/k8s.go** | K8sJobRunner.Run(ctx, run); Job args ["--task-run-id", run.RunID]. jobNameForTask(run.RunID) or keep task_id in name for debugging (run.RunID preferred for uniqueness). |
| **internal/server/server.go** | Register POST tasks/{task_id}/runs; GET/PATCH /api/worker/task-runs/{run_id}. Remove GET/PATCH /api/worker/tasks/{task_id}. |
| **internal/server/tasks.go** | createWorkspaceTaskHandler: TaskStore.CreateTask (task+first run). createTaskRunHandler (new). getTaskConversationHandler: read from task.LastRunID buildmax. taskToResponse: from task denormalized. |
| **internal/server/worker_handlers.go** | Replace getWorkerTaskHandler/patchWorkerTaskHandler with getWorkerTaskRunHandler (by run_id), patchWorkerTaskRunHandler (update run, OnRunComplete on terminal). |
| **internal/server/artifacts.go** | List: include task_run_id. Content: GetResult(..., task.TaskID, artifact.RunID, artifactID) — Artifact row has TaskRunID. |
| **internal/servercmd/run.go** | NewScheduler(taskRunStore, runner); pass TaskRunStore. Config may need to provide both stores (Store implements both). |
| **internal/workercmd/run.go** | RunWorker(ctx, runID string); GetWorkerTaskRun; RunTask(task, run, ...). |
| **cmd/buildmax-worker/main.go** | Flag --task-run-id; RunWorker(ctx, runID). |
| **portal/src** | TaskDetail: follow-up textarea + submit; createTaskRun API; poll task or run until SUCCEEDED/FAILED; refresh. |
| **config.BuildArtifactStorage / BuildPersistStorage** | ArtifactDir and persist task-buildmax paths now take runID; call sites in workercmd and servercmd pass runID where applicable (artifact: run from artifact row; persist: run from run context). |
