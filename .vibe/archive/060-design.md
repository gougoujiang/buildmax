# Design 060: Cmd binary split part 2 — executor (worker)

## Goal

Introduce a dedicated worker binary (`buildmax-worker`) that runs a single task. The worker gets task details and updates status/results via HTTP API (task-id-only endpoints); it accesses MinIO or local storage directly for materialize and artifact blobs. The scheduler (Runner) remains in the server process and only polls and spawns the worker with `--task-id`. Session ID is generated inside the worker.

## Modules

| Module (package) | Responsibility | Owns |
|------------------|----------------|------|
| **cmd/buildmax** | CLI entry (unchanged). | main.go |
| **cmd/buildmax-server** | Server entry (unchanged). | main.go |
| **cmd/buildmax-worker** | Worker entry: parse `--task-id`, call worker API, build paths/storage, run RunTask. | main.go (new). |
| **internal/executor** | Runner (scheduler): poll loop, spawn worker with `--task-id`. RunTask: materialize, buildmax -p, update via API client, artifact to storage + API. | executor.go, paths.go; optional workerclient.go or API types. |
| **internal/server** | HTTP server + **worker routes**: GET/PATCH `/api/worker/tasks/{task_id}`, worker auth middleware. | server.go, worker_handlers.go (new), worker_auth.go (new). |
| **internal/servercmd** | RunServer: build Runner with NewRunner(st, workerPath) only; no paths/persist/artifact passed to executor. | run.go, paths.go (paths still used by server for file/conversation APIs). |
| **internal/config** | Add WorkerBinaryPath(), WorkerServerURL(), WorkerToken(); WorkspacesDir, storage builders (worker uses same). | config.go, env_spec.go. |

## Structure

**Directory / files**

- `cmd/buildmax-worker/` — Worker binary (new)
  - `main.go` — flag `--task-id` (required); load BUILDMAX_SERVER_URL, BUILDMAX_WORKER_TOKEN, storage env; GET /api/worker/tasks/{task_id}; if not PENDING exit 1; generate session_id; PATCH RUNNING; build paths + persist + artifact storage from config; call executor.RunTask(ctx, task, sessionID, paths, persist, artifactStorage, apiClient); exit 0/non-zero.
- `internal/executor/`
  - `executor.go` — **Runner** (scheduler): struct { tasks, workerPath, pollInterval, stopCh, doneCh }; NewRunner(taskStore, workerPath); loop: GetNextPendingTask, exec.Command(workerPath, "--task-id", task.TaskID), wait. **RunTask**: function RunTask(ctx, task, sessionID, paths, persist, artifactStorage, taskUpdater) — materialize, mkdirs, buildmax -p, write result file, call taskUpdater for status/artifact. **TaskUpdater** interface (or struct with methods) for status PATCH and artifact registration so worker can pass HTTP client impl.
  - `paths.go` — WorkspacePaths interface (unchanged).
  - Optionally `worker_api.go` — types for worker API (request/response DTOs, client that calls GET/PATCH).
- `internal/server/`
  - `server.go` — Register routes: GET /api/worker/tasks/{task_id}, PATCH /api/worker/tasks/{task_id}. Apply worker-auth middleware to `/api/worker/` (validate BUILDMAX_WORKER_TOKEN from config).
  - `worker_handlers.go` — getWorkerTaskHandler: TaskStore.GetTask(ctx, taskID); 401 if no worker auth; 404 if not found; return JSON (task_id, workspace_id, input, status, ...). patchWorkerTaskHandler: parse body (status, session_id?, started_at?, ended_at?, output?, error_message?, artifact?); TaskStore.UpdateTaskStatus; if artifact present call IncrementTaskSeq + CreateArtifactWithItem (ArtifactStore).
  - `worker_auth.go` — middleware or helper: extract Bearer token or X-Worker-Token; compare with cfg.WorkerToken; 401 if missing/invalid. Server Config needs WorkerToken string (from env).
- `internal/servercmd/`
  - `run.go` — workerPath := config.WorkerBinaryPath(); runner, err := executor.NewRunner(st, workerPath); no workspacePaths/persist/artifactStorage passed to executor. Rest unchanged (server still gets persist/artifact for API).
- `internal/config/`
  - Add env keys: EnvKeyBuildmaxWorkerBinary, EnvKeyBuildmaxServerURL, EnvKeyBuildmaxWorkerToken.
  - WorkerBinaryPath() string — default "buildmax-worker".
  - WorkerServerURL() string — required for worker (e.g. http://localhost:5678).
  - WorkerToken() string — required for worker auth.
  - Server Config (or LoadServerEnv) extended to load WorkerToken so server can validate worker requests.

## Main types and interfaces

- **Runner** (internal/executor): Scheduler only. Fields: tasks entity.TaskStore, workerPath string, pollInterval, stopCh, doneCh. NewRunner(taskStore, workerPath). Start/Stop/loop unchanged; executeTask replaced by: exec.Command(workerPath, "--task-id", task.TaskID), inherit env, Wait.
- **RunTask** (internal/executor): `func RunTask(ctx context.Context, task *entity.Task, sessionID string, paths WorkspacePaths, persist blob.PersistStorage, artifactStorage blob.ArtifactStorage, updater TaskUpdater) error`. Implements current executeTask logic (mkdirs, materialize, buildmax -p, result file, updater.UpdateStatus SUCCEEDED/FAILED, updater.RegisterArtifact if success). Used by worker main.
- **TaskUpdater** (internal/executor): Interface with UpdateTaskStatus(ctx, taskID, status, startedAt, endedAt, output, errMsg, sessionID) and RegisterArtifact(ctx, taskID, artifactID, seq, relativePath). Worker provides an implementation that calls PATCH and (if needed) a separate artifact-registration call over HTTP.
- **Worker API** (internal/server): GET response: JSON with task_id, workspace_id, input, status, created_at, etc. PATCH body: JSON with status (required), optional session_id, started_at, ended_at, output, error_message, and optional artifact: { artifact_id, relative_path } (server computes seq via IncrementTaskSeq).

## Method design

| Receiver / Scope | Method | Signature | Responsibility |
|------------------|--------|-----------|----------------|
| executor | **NewRunner** | `(taskStore entity.TaskStore, workerPath string) (*Runner, error)` | Validate inputs; return Runner with no storage/paths. |
| **Runner** | **Start** | `()` | Launch loop goroutine (unchanged). |
| **Runner** | **Stop** | `()` | Signal stop, wait done (unchanged). |
| **Runner** | **loop** | (internal) | Ticker; GetNextPendingTask; exec worker with --task-id; wait. |
| **Runner** | **executeTask** | (internal) `(ctx, task)` | exec.Command(workerPath, "--task-id", task.TaskID); cmd.Run. |
| executor | **RunTask** | `(ctx, task, sessionID, paths, persist, artifactStorage, updater) error` | Mkdirs; materialize; buildmax -p; write result file; updater.UpdateStatus; on success updater.RegisterArtifact + artifactStorage.PutResult. |
| executor | **TaskUpdater** | interface: UpdateTaskStatus(...), RegisterArtifact(...) | Abstraction for worker to use HTTP client. |
| server | **getWorkerTaskHandler** | `(w, r)` | Worker auth; path task_id; GetTask; 404 if nil; write JSON. |
| server | **patchWorkerTaskHandler** | `(w, r)` | Worker auth; path task_id; parse body; UpdateTaskStatus; if artifact: IncrementTaskSeq, CreateArtifactWithItem. |
| server | **workerAuthMiddleware** | wrap handler | Validate Authorization Bearer or X-Worker-Token == cfg.WorkerToken; 401 else. |
| config | **WorkerBinaryPath** | `() string` | Env BUILDMAX_WORKER_BINARY or "buildmax-worker". |
| config | **WorkerServerURL** | `() string` | Env BUILDMAX_SERVER_URL (no default). |
| config | **WorkerToken** | `() string` | Env BUILDMAX_WORKER_TOKEN (no default). |

## How they work together

**Scheduler (server process)**

1. RunServer creates Runner with NewRunner(st, config.WorkerBinaryPath()). Runner.Start() runs loop.
2. Loop: GetNextPendingTask(ctx). If task == nil, continue. Otherwise exec.Command(workerPath, "--task-id", task.TaskID), inherit os.Environ(), Run (block until worker exits). No DB write in scheduler for RUNNING; worker claims via PATCH.

**Worker process**

1. main: parse --task-id. Load WorkerServerURL(), WorkerToken(), WorkspacesDir(), storage config (same as server). Build HTTP client with worker token.
2. GET /api/worker/tasks/{task_id}. If 404 or status != "PENDING", os.Exit(1). Generate session_id (e.g. uuid).
3. PATCH /api/worker/tasks/{task_id} with { "status": "RUNNING", "session_id": sessionID, "started_at": now }. If 4xx/5xx, exit 1.
4. Build workspace paths from WorkspacesDir() (same layout as serverWorkspacePaths — can use a small struct in worker or in a shared package). Build persist and artifact storage via config (MinIO or local_fs).
5. Build TaskUpdater impl that PATCHes to server. Call executor.RunTask(ctx, task, sessionID, paths, persist, artifactStorage, updater).
6. RunTask: mkdirs (RuntimeTaskBuildmaxDir, RuntimeTaskWSDir); persist.MaterializeToDir(workspaceID, wsDir); exec buildmax -p with session_id; on success: artifactStorage.PutResult; updater.RegisterArtifact (PATCH or dedicated POST); updater.UpdateStatus(SUCCEEDED, ...). On failure: updater.UpdateStatus(FAILED, ...).
7. main exits 0 or 1 based on RunTask error.

**Worker API**

- GET /api/worker/tasks/{task_id}: Requires worker auth. TaskStore.GetTask(ctx, taskID). Response JSON: task_id, workspace_id, input, status, project_id, created_at, etc. (snake_case).
- PATCH /api/worker/tasks/{task_id}: Body: status, optional session_id, started_at, ended_at, output, error_message, optional artifact { artifact_id, relative_path }. UpdateTaskStatus with provided fields. If artifact present: IncrementTaskSeq(taskID), then CreateArtifactWithItem(taskID, artifact_id, seq, relative_path). Worker uploads blob to MinIO directly before calling PATCH with artifact.

**Config**

- Server needs WorkerToken in Config to validate /api/worker/ requests (LoadServerEnv or new LoadWorkerEnv can load it).
- Worker needs WorkerServerURL, WorkerToken, WorkspacesDir, BUILDMAX_PERSIST_STORAGE, BUILDMAX_ARTIFACT_STORAGE, MinIO vars (same as server) so it can materialize and write artifacts.

## Dependencies

- cmd/buildmax-worker → internal/executor, internal/config; executor.RunTask and Runner not used by worker main (only RunTask).
- internal/executor (Runner) → entity.TaskStore only; no blob, no paths. executor.RunTask → config (for BUILDMAX_HOME in buildmax env), blob, paths, TaskUpdater.
- internal/server → entity (TaskStore, ArtifactStore for worker handlers); config for WorkerToken. Worker handlers use TaskStore.GetTask, UpdateTaskStatus, IncrementTaskSeq; ArtifactStore.CreateArtifactWithItem.
- internal/servercmd → executor.NewRunner(st, workerPath); no paths/persist/artifact for executor.

## Make and AGENTS.md

- **make build**: Build all three: buildmax, buildmax-server, buildmax-worker. Clean removes all three.
- **make run server**: Build and run buildmax-server; document that worker must be on PATH and BUILDMAX_SERVER_URL + BUILDMAX_WORKER_TOKEN (and storage env) set when worker is spawned.
- **AGENTS.md**: Add cmd/buildmax-worker; document worker API (/api/worker/tasks/{task_id}), worker auth, direct MinIO access; three-binaries layout.

## Tests

- **internal/executor**: Test that Runner with mock TaskStore and fake worker path runs command with args ["--task-id", "<id>"] and no --session-id. Test RunTask with mock paths, mock persist/artifact storage, mock TaskUpdater (verify UpdateStatus and RegisterArtifact called).
- **internal/server**: Test GET /api/worker/tasks/{task_id} returns 401 without worker token, 404 when task missing, 200 with task body when auth ok. Test PATCH returns 401 without token; 200 and status/artifact updated when body valid.
- **cmd/buildmax-worker**: Test that missing --task-id exits non-zero; optional: test with mock server that GET/PATCH are called.

## Changes for review

- **New**: `cmd/buildmax-worker/main.go` — parse --task-id; load config; GET task; if not PENDING exit; PATCH RUNNING; build paths/storage; executor.RunTask with HTTP TaskUpdater; exit code.
- **New**: `internal/server/worker_handlers.go` — getWorkerTaskHandler, patchWorkerTaskHandler.
- **New**: `internal/server/worker_auth.go` — middleware or helper for worker token validation; register under /api/worker/.
- **Modified**: `internal/server/server.go` — add WorkerToken to Config; register GET/PATCH /api/worker/tasks/{task_id} with worker auth.
- **Modified**: `internal/executor/executor.go` — Runner: remove artifacts, paths, persist, artifactStorage; add workerPath. NewRunner(taskStore, workerPath). executeTask: only exec worker with --task-id. Add RunTask(...) and TaskUpdater interface; move current executeTask body into RunTask (using updater for DB/API).
- **New** (optional): `internal/executor/worker_api.go` or in worker main — HTTP client types and TaskUpdater implementation that PATCHes server.
- **Modified**: `internal/servercmd/run.go` — runner, err := executor.NewRunner(st, config.WorkerBinaryPath()); remove workspacePaths, persist, artifactStorage from executor.New.
- **Modified**: `internal/config/config.go` — WorkerBinaryPath(), WorkerServerURL(), WorkerToken(). LoadServerEnv or server startup loads WorkerToken into server.Config.
- **Modified**: `internal/config/env_spec.go` — add EnvKeyBuildmaxWorkerBinary, EnvKeyBuildmaxServerURL, EnvKeyBuildmaxWorkerToken and entries in EnvVars.
- **Modified**: `make` — build and clean include buildmax-worker; usage text.
- **Modified**: `.env.example` — add BUILDMAX_WORKER_BINARY, BUILDMAX_SERVER_URL, BUILDMAX_WORKER_TOKEN.
- **Modified**: AGENTS.md — third binary, worker API, worker auth, direct storage.
