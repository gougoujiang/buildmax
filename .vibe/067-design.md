# Design 067: Use k8s job to run worker instead of local process

## Goal

Make worker execution strategy configurable: **local_process** (current: spawn binary in same host) and **k8s_job** (create a Kubernetes Job per task). Persist worker metadata on the task record (worker_type, k8s_job_name, k8s_job_created_at). Add RBAC so the server can create Jobs in the cluster. Job name pattern: `buildmax-worker-<sanitized-task-id>-<timestamp>`.

## Modules

| Module | Responsibility |
|--------|----------------|
| **internal/config** | WorkerRunMode(), WorkerJobNamespace(), WorkerImage(); new env keys in env_spec.go. |
| **internal/executor** | WorkerRunner interface; LocalRunner and K8sJobRunner implementations; Scheduler takes runner and TaskStore, calls runner.Run then UpdateTaskWorkerInfo. |
| **internal/model** | Task: add WorkerType, K8sJobName, K8sJobCreatedAt. |
| **internal/storage/entity** | TaskStore: add UpdateTaskWorkerInfo; Store implements it; GORM migration for new columns. |
| **internal/servercmd** | RunServer: read WorkerRunMode(), construct LocalRunner or K8sJobRunner, pass to NewScheduler. |
| **internal/server** | (Optional) TaskResponse and taskToResponse: add worker_type, k8s_job_name, k8s_job_created_at. |
| **setup/buildmax-deploy.yaml** | ServiceAccount, Role, RoleBinding for Jobs/Pods; server Deployment serviceAccountName. |

## Structure

### internal/config

- **New env keys** (in env_spec.go): `BUILDMAX_WORKER_RUN_MODE` (default `local_process`), `BUILDMAX_WORKER_JOB_NAMESPACE` (default `buildmax`), `BUILDMAX_WORKER_IMAGE` (default `buildmax:local`).
- **Getters**:
  - `WorkerRunMode() string` — returns `local_process` or `k8s_job`; invalid value treated as `local_process`.
  - `WorkerJobNamespace() string` — namespace for Job creation; default `buildmax`.
  - `WorkerImage() string` — container image for the Job pod; default `buildmax:local`.
- **EnvVars**: Add three entries to the EnvVars slice for documentation.

### internal/executor

- **WorkerRunner interface**:
  - `Run(ctx context.Context, task entity.Task) (workerType string, k8sJobName *string, k8sJobCreatedAt *int64, err error)`
  - On success: returns worker type and optional k8s fields for the caller to persist. On failure: returns error; caller reverts task to PENDING.
- **LocalRunner** (new type): holds `workerPath string`. Implements WorkerRunner: exec CommandContext(workerPath, "--task-id", task.TaskID), inherit env, Run(); on success return `("local_process", nil, nil, nil)`; on error return `("", nil, nil, err)`.
- **K8sJobRunner** (new type): holds namespace, image, env map (or source for env), and a Kubernetes client capable of creating Jobs. Implements WorkerRunner: build Job name with `jobNameForTask(task.TaskID)`, build batch/v1 Job spec, create Job; on success return `("k8s_job", &jobName, &createdAtUnix, nil)`; on error return `("", nil, nil, err)`.
- **Job name**: `buildmax-worker-<sanitized-task-id>-<unix-timestamp>`. Sanitize task_id for DNS-1123 (lowercase, replace non-alphanumeric with `-`, take first N chars so total length ≤ 63). Helper `jobNameForTask(taskID string) string` (uses time.Now().Unix() at call time).
- **Scheduler**: Replace `workerPath` with `runner WorkerRunner`. Constructor `NewScheduler(taskStore entity.TaskStore, runner WorkerRunner) (*Scheduler, error)`. In loop: after UpdateTaskStatusIf(PENDING→SCHEDULED), call `workerType, k8sName, k8sAt, err := s.runner.Run(ctx, *task)`. If err != nil: revert to PENDING, continue. If err == nil: call `s.tasks.UpdateTaskWorkerInfo(ctx, task.TaskID, workerType, k8sName, k8sAt)` (ignore update error or log), then continue (local runner blocks until worker exits; k8s runner returns immediately after create).
- **K8s client**: New package or same package: use `k8s.io/client-go` (add to go.mod). Build rest.Config: in-cluster (InClusterConfig) when running in cluster; for local dev use env KUBECONFIG or default kubeconfig. Create batchv1 typed client for Jobs in the given namespace. K8sJobRunner accepts an interface for “create Job” so tests can inject a fake (e.g. `type JobCreator interface { Create(ctx, namespace, *batchv1.Job) error }`), or use a concrete type and test with a real cluster / skip when not in cluster.

### internal/model

- **Task struct**: Add fields (snake_case json, gorm column names):
  - `WorkerType` (string, gorm `type:varchar(32)`, json `worker_type,omitempty`) — `local_process` or `k8s_job`.
  - `K8sJobName` (*string, gorm `type:varchar(128)`, json `k8s_job_name,omitempty`).
  - `K8sJobCreatedAt` (*int64, gorm column `k8s_job_created_at`, json `k8s_job_created_at,omitempty`).
- **Table**: GORM AutoMigrate will add columns; ensure singular table name unchanged.

### internal/storage/entity

- **TaskStore interface**: Add `UpdateTaskWorkerInfo(ctx context.Context, taskID, workerType string, k8sJobName *string, k8sJobCreatedAt *int64) error`.
- **Store (task.go)**: Implement UpdateTaskWorkerInfo: updates only worker_type, k8s_job_name, k8s_job_created_at for the given task_id. Use map updates and Where("task_id = ?").
- **Types**: Task is alias for model.Task; model already has new fields.

### internal/servercmd

- **RunServer**: After building st (entity store), determine runner:
  - `mode := config.WorkerRunMode()`
  - If mode != `k8s_job`: `runner := executor.NewLocalRunner(config.WorkerBinaryPath())`
  - If mode == `k8s_job`: build k8s rest.Config and Job client, env map from current process (BUILDMAX_SERVER_URL, BUILDMAX_WORKER_TOKEN, BUILDMAX_WORKSPACES_DIR, BUILDMAX_MINIO_*, BUILDMAX_PERSIST_STORAGE, BUILDMAX_ARTIFACT_STORAGE, etc.), `runner := executor.NewK8sJobRunner(config.WorkerJobNamespace(), config.WorkerImage(), envMap, jobClient)` (or equivalent). Then `scheduler, err := executor.NewScheduler(st, runner)`.
- No change to server or blob setup.

### internal/server (optional)

- **TaskResponse**: Add `WorkerType string`, `K8sJobName *string`, `K8sJobCreatedAt *int64` with json tags `worker_type`, `k8s_job_name`, `k8s_job_created_at`.
- **taskToResponse**: Set the new fields from model.Task.

### setup/buildmax-deploy.yaml

- **ServiceAccount**: `apiVersion: v1`, `kind: ServiceAccount`, `metadata: { name: buildmax-server, namespace: buildmax }`.
- **Role**: `apiVersion: rbac.authorization.k8s.io/v1`, `kind: Role`, `metadata: { name: buildmax-server, namespace: buildmax }`, rules: `apiGroups: [""] resources: [pods] verbs: [create, get, list, delete]`, `apiGroups: [batch] resources: [jobs] verbs: [create, get, list, delete]`.
- **RoleBinding**: `kind: RoleBinding`, roleRef to the Role, subjects: ServiceAccount buildmax-server in namespace buildmax.
- **Deployment buildmax-server**: In `spec.template.spec` add `serviceAccountName: buildmax-server`. Optionally add env for k8s_job mode: `BUILDMAX_WORKER_RUN_MODE: "k8s_job"`, `BUILDMAX_WORKER_JOB_NAMESPACE: "buildmax"`, `BUILDMAX_WORKER_IMAGE: "buildmax:local"` (or leave unset for default).

### Job spec (K8sJobRunner)

- **Name**: From `jobNameForTask(task.TaskID)`.
- **Namespace**: From config.
- **Pod template**: Single container; image from config; command `["buildmax-worker"]`; args `["--task-id", task.TaskID]`. Env: from env map (server URL, worker token, workspaces dir, MinIO/storage vars). RestartPolicy: OnFailure (or Never for Job). BackoffLimit: 3 (or 0 for no retries).
- **Job**: `batch/v1`, BackoffLimit set, no TTL (out of scope).

## Method design

### config

- **WorkerRunMode() string** — Read BUILDMAX_WORKER_RUN_MODE; if `k8s_job` return `k8s_job`, else return `local_process`.
- **WorkerJobNamespace() string** — BUILDMAX_WORKER_JOB_NAMESPACE or `buildmax`.
- **WorkerImage() string** — BUILDMAX_WORKER_IMAGE or `buildmax:local`.

### executor

- **WorkerRunner.Run(ctx, task) (workerType string, k8sJobName *string, k8sJobCreatedAt *int64, err error)** — Contract: on success (worker started or Job created), return non-empty workerType and optional k8s fields; err == nil. On failure, err != nil; return values for worker info are zero and not persisted.
- **NewLocalRunner(workerPath string) *LocalRunner** — Returns a LocalRunner that exec’s workerPath with --task-id. workerPath must be non-empty (caller validates).
- **NewK8sJobRunner(namespace, image string, env []corev1.EnvVar, jobClient JobCreator) *K8sJobRunner** — JobCreator is an interface: `CreateJob(ctx context.Context, namespace string, job *batchv1.Job) error`. Env is a list of EnvVar for the Job pod. Returns runner that creates Jobs with the given name pattern and spec.
- **NewScheduler(taskStore entity.TaskStore, runner WorkerRunner) (*Scheduler, error)** — taskStore and runner must be non-nil. Scheduler holds runner; loop calls runner.Run after claim and UpdateTaskWorkerInfo on success.
- **jobNameForTask(taskID string) string** — Sanitize taskID (lowercase, replace non-[a-z0-9-] with `-`, truncate so total length ≤ 63). Return `"buildmax-worker-" + sanitized + "-" + strconv.FormatInt(time.Now().Unix(), 10)`. Ensure no double hyphens; trim leading/trailing hyphens from sanitized segment if needed.

### entity.TaskStore

- **UpdateTaskWorkerInfo(ctx, taskID, workerType string, k8sJobName *string, k8sJobCreatedAt *int64) error** — Update only these three columns for the row with task_id. Used after runner.Run succeeds.

### entity.Store

- **UpdateTaskWorkerInfo** — `updates := map[string]interface{}{"worker_type": workerType}`; if k8sJobName != nil set `k8s_job_name`; if k8sJobCreatedAt != nil set `k8s_job_created_at`. Where("task_id = ?", taskID).Updates(updates).Error.

## How they work together

1. **Startup**: RunServer loads config, builds DB and blob. It calls WorkerRunMode(). If local_process, it creates LocalRunner(WorkerBinaryPath()) and NewScheduler(st, runner). If k8s_job, it builds k8s config and Job client, builds env list from current process (or from a helper that returns EnvVars for worker), creates K8sJobRunner(namespace, image, env, client), then NewScheduler(st, runner). Scheduler.Start() runs the loop.
2. **Loop**: Scheduler ticks; GetNextPendingTask; UpdateTaskStatusIf(PENDING→SCHEDULED); runner.Run(ctx, task). If Run returns error: UpdateTaskStatus(taskID, PENDING) and continue. If Run returns success: UpdateTaskWorkerInfo(taskID, workerType, k8sJobName, k8sJobCreatedAt), then continue. For LocalRunner, Run blocks until the worker process exits (so next iteration only after that). For K8sJobRunner, Run returns immediately after Job create.
3. **Persistence**: Task row now has worker_type set for every scheduled run; for k8s_job, k8s_job_name and k8s_job_created_at are set. API (if extended) returns these in GET task / list tasks.
4. **RBAC**: Server pod runs as ServiceAccount buildmax-server, which can create/list/delete Jobs and Pods in namespace buildmax, so Create Job in that namespace succeeds.

## Dependencies

- **go.mod**: Add `k8s.io/client-go` and related (e.g. `k8s.io/api` batch/v1, core/v1). Use a recent stable version; typically pull in client-go and let it bring api and apimachinery.

## Changes for review

| Package / File | Change |
|----------------|--------|
| **internal/config/config.go** | Add WorkerRunMode(), WorkerJobNamespace(), WorkerImage(). |
| **internal/config/env_spec.go** | Add EnvKeyBuildmaxWorkerRunMode, EnvKeyBuildmaxWorkerJobNamespace, EnvKeyBuildmaxWorkerImage; add three EnvVars entries. |
| **.env.example** | Document BUILDMAX_WORKER_RUN_MODE, BUILDMAX_WORKER_JOB_NAMESPACE, BUILDMAX_WORKER_IMAGE. |
| **internal/model/models.go** | Task: add WorkerType, K8sJobName, K8sJobCreatedAt (gorm + json snake_case). |
| **internal/storage/entity/interfaces.go** | TaskStore: add UpdateTaskWorkerInfo(ctx, taskID, workerType string, k8sJobName *string, k8sJobCreatedAt *int64) error. |
| **internal/storage/entity/task.go** | Implement UpdateTaskWorkerInfo. Ensure AutoMigrate or migration adds new columns. |
| **internal/executor/runner.go** (new) | WorkerRunner interface; LocalRunner and K8sJobRunner types and Run implementations; jobNameForTask helper. |
| **internal/executor/scheduler.go** | Replace workerPath with runner WorkerRunner; NewScheduler(taskStore, runner); loop: after claim call runner.Run, on err revert to PENDING, on success call UpdateTaskWorkerInfo. |
| **internal/executor/k8s.go** (new, or in runner.go) | K8s client build: InClusterConfig / kubeconfig; NewJobClient; Job spec builder (name, container, env). |
| **internal/servercmd/run.go** | Branch on WorkerRunMode(); create LocalRunner or K8sJobRunner; NewScheduler(st, runner). |
| **internal/server/tasks.go** | (Optional) TaskResponse add WorkerType, K8sJobName, K8sJobCreatedAt; taskToResponse set from model. |
| **setup/buildmax-deploy.yaml** | Add ServiceAccount buildmax-server; Role (jobs, pods); RoleBinding; Deployment serviceAccountName: buildmax-server. |
| **go.mod / go.sum** | Add k8s.io/client-go, k8s.io/api (batch/v1, core/v1). |
| **internal/executor/executor_test.go** | NewScheduler(st, runner): use NewLocalRunner("buildmax-worker") in existing tests. Add test for LocalRunner Run success/failure; add test for K8sJobRunner with fake JobCreator or skip when no cluster. |

No change to RunTask, worker binary, or blob storage.
