# Design 054: Display artifacts

## Goal

Expose artifacts in the portal: backend APIs to list artifacts (with optional task_id/project_id filters) and to serve artifact file content; portal UI sections on Projects, Project, and Activity pages that list recent artifacts with key ids (workspace_id, task_id, project_id, artifact_id) and allow viewing artifact content (e.g. modal with markdown).

## Modules

| Module (package) | Responsibility | Owns |
|-----------------|----------------|------|
| **internal/store** | Artifact list + lookup | ArtifactWithTask DTO; ListArtifactsByWorkspace; GetArtifactByID; TaskStore.GetTask (by task_id) |
| **internal/server** | Artifacts HTTP API | Config.ArtifactStore; GET /artifacts list handler; GET /artifacts/{id}/content handler; ArtifactResponse type |
| **portal (React)** | Artifact API + UI | getArtifacts, getArtifactContent; ApiArtifact type; Artifact type; Artifacts section on Projects, Project, Activity; view content (modal or inline) |

No change to **internal/config** or **internal/executor**; artifact dir path and creation stay as in 053.

## Structure

### internal/store

- **ArtifactWithTask** (struct, not a table): DTO for list results. Fields: ArtifactID, TaskID, WorkspaceID, ProjectID (*string), CreatedAt, Seq, TaskInputSnippet (string, first 200 chars of task.input). JSON tags snake_case: `artifact_id`, `task_id`, `workspace_id`, `project_id`, `created_at`, `seq`, `task_input_snippet`.
- **ArtifactStore** interface: add two methods.
  - `ListArtifactsByWorkspace(ctx context.Context, workspaceID string, taskID, projectID *string) ([]ArtifactWithTask, error)`. Query: join `artifact` with `task` on `artifact.task_id = task.task_id`, where `task.workspace_id = workspaceID`, and optionally `task.task_id = *taskID` and `task.project_id = *projectID`. Order by `artifact.created_at DESC`. Select artifact.artifact_id, artifact.task_id, artifact.created_at, artifact.seq, task.workspace_id, task.project_id, and truncate task.input to 200 chars as task_input_snippet. Return slice of ArtifactWithTask.
  - `GetArtifactByID(ctx context.Context, artifactID string) (*Artifact, error)`. Return the artifact row by artifact_id, or (nil, nil) if not found. Used by content handler to get task_id.
- **TaskStore** interface: add `GetTask(ctx context.Context, taskID string) (*Task, error)`. Return the task by task_id, or (nil, nil) if not found. Used by content handler to get workspace_id and verify ownership.
- **Store**: implement ListArtifactsByWorkspace (join query, map to ArtifactWithTask); implement GetArtifactByID; implement GetTask. Store already implements ArtifactStore and TaskStore; no new interfaces in Config beyond ArtifactStore.

### internal/server

- **Config**: add `ArtifactStore store.ArtifactStore` (optional; required for artifact routes). Same `*store.Store` can be passed as TaskStore and ArtifactStore in cmd.
- **New (server)**: register `GET /api/workspaces/{workspace_id}/artifacts` → listWorkspaceArtifactsHandler; `GET /api/workspaces/{workspace_id}/artifacts/{artifact_id}/content` → artifactContentHandler.
- **Artifact response type**: ArtifactResponse (snake_case JSON): artifact_id, task_id, workspace_id, project_id, created_at, seq, task_input_snippet. Map from store.ArtifactWithTask.
- **listWorkspaceArtifactsHandler**: withWorkspaceAuth(w, r, "workspace_id"); require ArtifactStore (e.g. requireArtifactStore(w) returning bool). Parse query task_id, project_id (optional). Call cfg.ArtifactStore.ListArtifactsByWorkspace(ctx, workspaceID, taskIDPtr, projectIDPtr). Write JSON array of ArtifactResponse. Empty list on success.
- **artifactContentHandler**: withWorkspaceAuth(w, r, "workspace_id"); require ArtifactStore and TaskStore. Path param artifact_id. Get artifact by cfg.ArtifactStore.GetArtifactByID(ctx, artifactID). If nil, 404. Get task by cfg.TaskStore.GetTask(ctx, artifact.TaskID). If nil or task.WorkspaceID != workspaceID, 404. Path: config.ArtifactDir(workspaceID, task.TaskID, artifactID). Read file result.md (executor writes result.md in artifact dir per 053). If file missing, 404. Set Content-Type text/markdown; charset=utf-8 (or text/plain). Write file content.
- **Helper**: requireArtifactStore(w) similar to requireTaskStore; write 503 and return false if cfg.ArtifactStore == nil.
- **File**: new file `internal/server/artifacts.go` for handlers and response type; keep server.go for route registration only.

### portal

- **api.ts**: Add `ApiArtifact` interface: artifact_id, task_id, workspace_id, project_id, created_at (number), seq (number), task_input_snippet (string). Add `getArtifacts(workspaceId: string, token: string, options?: { projectId?: string; taskId?: string }): Promise<ApiArtifact[]>` (build URL with optional query params). Add `getArtifactContent(workspaceId: string, artifactId: string, token: string): Promise<string>` (GET artifact content endpoint, return response.text()). Add `apiArtifactToArtifact(api: ApiArtifact): Artifact` (or similar) to map to UI type with timeLabel, id = artifact_id, etc.
- **types.ts**: Add **Artifact** type: id (artifact_id), taskId, projectId (optional), workspaceId, timeLabel (from created_at), title or snippet (task_input_snippet). Used by list UIs.
- **useWorkspaceData or new hook**: Either extend useWorkspaceData with artifacts state and refetchArtifacts, or have each page (Projects, Project, Activity) call getArtifacts with appropriate filters (workspace; workspace+project; workspace). Prefer extending useWorkspaceData: add artifacts (Artifact[]), refetchArtifacts(projectId?: string, taskId?: string), and useEffect that fetches artifacts when workspaceId is set (and optionally when projectId changes for Project page). So: artifacts for workspace (no filter), and when on project route, refetch with projectId so Project page gets project-scoped list. Activity page uses workspace-wide artifacts (same as Projects). Projects page: workspace-wide artifacts. Project page: pass projectId to getArtifacts. Activity page: workspace-wide artifacts.
- **Projects.tsx**: Add section "Artifacts" below Projects list. Heading "Recent artifacts" or "Artifacts". Render list of artifacts (from useWorkspaceData.artifacts or props). Each row/card: link or button to view content; show artifact_id, task_id, project_id, workspace_id (e.g. in a table or card footer). Click "View" opens artifact content (modal or navigate). Pass refetchArtifacts and artifacts from parent.
- **Project.tsx**: Add "Artifacts" section (or subsection under Activity). List artifacts for this project (filter by project.id). Same columns/keys; view content action.
- **Activity.tsx**: Add "Artifacts" subsection below (or above) the task list. List workspace artifacts with key ids; view content action. Show tasks and artifacts in one place (two lists or one combined with type badge).
- **View content**: Add a small modal or inline viewer: when user clicks "View" on an artifact, call getArtifactContent(workspaceId, artifactId, token), render result in a modal with Markdown (reuse Markdown from TaskDetail). Component: ArtifactContentModal (props: workspaceId, artifactId, onClose). On open, fetch content and display; loading/error states.

### Optional: task list includes last_artifact_id

- **TaskResponse** (server): add field `LastArtifactID *string` (json "last_artifact_id,omitempty"). taskToResponse: set from t.LastArtifactID.
- **ApiTask** (portal): add `last_artifact_id: string | null`. UI can show an icon or "Has artifact" without calling artifacts API. Implementation: one-line addition in tasks.go and api.ts.

## Method design

| Package / layer | Component | Method / function | Signature / contract |
|-----------------|-----------|-------------------|----------------------|
| **store** | ArtifactStore | ListArtifactsByWorkspace | `ListArtifactsByWorkspace(ctx context.Context, workspaceID string, taskID, projectID *string) ([]ArtifactWithTask, error)`. Join artifact+task; filter by workspace_id and optional task_id, project_id; order created_at DESC; return DTOs with task_input_snippet (max 200 chars). |
| **store** | ArtifactStore | GetArtifactByID | `GetArtifactByID(ctx context.Context, artifactID string) (*Artifact, error)`. Select by artifact_id; (nil, nil) if not found. |
| **store** | TaskStore | GetTask | `GetTask(ctx context.Context, taskID string) (*Task, error)`. Select by task_id; (nil, nil) if not found. |
| **server** | Server | listWorkspaceArtifactsHandler | GET /api/workspaces/{id}/artifacts. Auth; query task_id, project_id; ListArtifactsByWorkspace; write JSON []ArtifactResponse. |
| **server** | Server | artifactContentHandler | GET /api/workspaces/{id}/artifacts/{artifact_id}/content. Auth; GetArtifactByID → GetTask; verify task.WorkspaceID == path workspace_id; read ArtifactDir(...)/result.md; 404 if missing; write body text/markdown. |
| **portal** | api | getArtifacts | `getArtifacts(workspaceId, token, options?: { projectId?, taskId? }): Promise<ApiArtifact[]>`. |
| **portal** | api | getArtifactContent | `getArtifactContent(workspaceId, artifactId, token): Promise<string>`. |
| **portal** | useWorkspaceData | (extend) | Add artifacts: Artifact[], refetchArtifacts(projectId?, taskId?). Fetch artifacts when workspaceId set; when projectId in route, refetch with projectId for Project page. |

## How they work together

**List artifacts (portal → server → store)**

1. User is on Projects, Project, or Activity page. useWorkspaceData (or page) calls getArtifacts(workspaceId, token, { projectId } for Project page).
2. Browser: GET /api/workspaces/{id}/artifacts?project_id=... (optional). Server: withWorkspaceAuth, requireArtifactStore, ListArtifactsByWorkspace(workspaceID, taskID, projectID). Store: join artifact and task, filter, return []ArtifactWithTask. Server maps to []ArtifactResponse, writes JSON.
3. Portal maps ApiArtifact → Artifact (timeLabel, etc.) and renders list with key ids and "View" button.

**View artifact content (portal → server → store + config)**

1. User clicks "View" on an artifact. Modal opens; getArtifactContent(workspaceId, artifactId, token) → GET /api/workspaces/{id}/artifacts/{artifact_id}/content.
2. Server: withWorkspaceAuth, GetArtifactByID(artifactID) → artifact (get task_id). GetTask(artifact.TaskID) → task; if task == nil or task.WorkspaceID != path workspace_id → 404. artifactDir := config.ArtifactDir(workspaceID, task.TaskID, artifactID). Read artifactDir/result.md; if not exist → 404. Write body with Content-Type text/markdown.
3. Portal receives text; modal renders with <Markdown>{content}</Markdown>.

**Content handler 404 cases**

- Artifact not found (GetArtifactByID returns nil).
- Task not found (GetTask returns nil).
- Task.WorkspaceID != request workspace_id (forbidden / not in workspace).
- result.md missing on disk (e.g. deleted or not yet written).

## Errors and edge cases

- **ArtifactStore nil**: list and content handlers call requireArtifactStore; 503 if not configured.
- **Empty list**: ListArtifactsByWorkspace returns empty slice; 200 with [].
- **Task_id / project_id filter**: If project_id given, resolve same as tasks handler (validate project belongs to workspace); then pass to store. If task_id given, pass through; store filters by artifact.task_id.
- **Truncation**: Task input longer than 200 chars: store or server truncates to 200 for task_input_snippet (store preferred in one place).

## Tests

- **store**: ListArtifactsByWorkspace: create workspace, project, task, artifact (CreateArtifactWithItem); call ListArtifactsByWorkspace(workspaceID, nil, nil) → one row with correct workspace_id, project_id, snippet. Filter by project_id → same row; filter by other project_id → empty. GetArtifactByID: existing artifact_id → *Artifact; non-existing → (nil, nil). GetTask: existing task_id → *Task; non-existing → (nil, nil).
- **server**: List handler: 200 with body array; optional query task_id/project_id applied; 401 without auth; 503 without ArtifactStore. Content handler: 200 with result.md body when artifact and file exist; 404 when artifact_id unknown; 404 when task not in workspace; 404 when result.md missing.
- **portal**: Optional unit tests for apiArtifactToArtifact (timeLabel format); integration or manual for Artifacts section and modal.

## Backward compatibility

- New API routes only; no change to existing task or workspace APIs except optional TaskResponse.last_artifact_id.
- cmd/server: add cfg.ArtifactStore = st (same store).

---

## Changes for review

| Area | Change |
|------|--------|
| **internal/store** | ArtifactWithTask struct (DTO). ArtifactStore: ListArtifactsByWorkspace(ctx, workspaceID, taskID?, projectID?) ([]ArtifactWithTask, error); GetArtifactByID(ctx, artifactID) (*Artifact, error). TaskStore: GetTask(ctx, taskID) (*Task, error). Store implements all three. |
| **internal/server** | Config: add ArtifactStore. requireArtifactStore(w). New file artifacts.go: ArtifactResponse type; listWorkspaceArtifactsHandler; artifactContentHandler (read result.md via config.ArtifactDir). server.go: register GET /api/workspaces/{workspace_id}/artifacts and GET /api/workspaces/{workspace_id}/artifacts/{artifact_id}/content. |
| **internal/cmd** | server.Config: set ArtifactStore: st. |
| **portal/src/lib/api.ts** | ApiArtifact type; getArtifacts(workspaceId, token, options?); getArtifactContent(workspaceId, artifactId, token); apiArtifactToArtifact(api). |
| **portal/src/lib/types.ts** | Artifact type (id, taskId, projectId?, workspaceId, timeLabel, title/snippet). |
| **portal/src/hooks/useWorkspaceData.ts** | artifacts state; refetchArtifacts(projectId?, taskId?); fetch artifacts when workspaceId (and optionally projectId) set; return artifacts, refetchArtifacts. |
| **portal/src/pages/Projects.tsx** | "Artifacts" section; list artifacts with key ids; "View" opens content modal. |
| **portal/src/pages/Project.tsx** | "Artifacts" section filtered by project; list + view content. |
| **portal/src/pages/Activity.tsx** | "Artifacts" subsection; list + view content. |
| **portal** | ArtifactContentModal component (fetch and render markdown); used from Projects, Project, Activity. |
| **Optional** | TaskResponse and ApiTask: add last_artifact_id. taskToResponse set from task.LastArtifactID. |
