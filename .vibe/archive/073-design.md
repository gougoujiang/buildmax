# Design 073: Align concepts frontend and backend (task → chat, task_run → chat_run)

## Goal

Rename backend and frontend terminology from **task** / **task_run** to **chat** / **chat_run** so API, storage, types, and code use a single vocabulary. No backward compatibility.

## Modules

| Module | Responsibility | Changes |
|--------|----------------|---------|
| **internal/model** | Domain types | Task→Chat, TaskRun→ChatRun; table names chat, chat_run; JSON/DB fields chat_id, chat_run_id |
| **internal/storage/entity** | MySQL persistence, store interfaces | TaskStore→ChatStore, TaskRunStore→ChatRunStore; task.go→chat.go, task_run.go→chat_run.go; artifact/store use chat_id/chat_run_id |
| **internal/storage/blob** | Blob key layout and interfaces | TaskBuildmaxObjectKey→ChatBuildmaxObjectKey; path segment tasks→chats; method params taskID→chatID |
| **internal/config** | Path helpers for runtime dirs | RuntimeTask*→RuntimeChat*, param taskID→chatID; path segment tasks→chats |
| **internal/server** | HTTP API, handlers, OpenAPI | Routes /tasks→/chats, task_id→chat_id; handlers and types renamed; worker path task-runs→chat-runs |
| **internal/workerapi** | Worker HTTP contract | Types and JSON: task→chat, task_run→chat_run, task_id→chat_id, run_id→chat_run_id |
| **internal/executor** | Scheduler and RunTask | Use Chat, ChatRun, ChatStore, ChatRunStore; paths and worker API chat-runs, --chat-run-id |
| **internal/servercmd** | Server bootstrap | Wire ChatStore, ChatRunStore (no API change) |
| **internal/workercmd** | Worker entry | Accept --chat-run-id, call GetWorkerChatRun |
| **cmd/buildmax-worker** | Worker binary | Flag --task-run-id→--chat-run-id |
| **portal/** | Frontend types, API, pages, hooks | Task→Chat, taskId→chatId; API names and URLs /chats, chat_id; all usages updated |

## Structure and naming decisions

**ID naming**

- **chat_id**: The chat’s public id (replaces task_id). DB column and JSON field `chat_id`.
- **chat_run_id**: The run’s public id (replaces run_id). DB column and JSON field `chat_run_id`. URL path for worker remains `.../chat-runs/{chat_run_id}` (one path param).

**DB**

- Table `task` → `chat`; primary key column `task_id` → `chat_id`. All other columns on `task` renamed where they reference “task” (e.g. no task_run_id on chat; chat has last_run_id or last_chat_run_id—keep last_run_id for minimal change, or rename to last_chat_run_id; design choice: keep last_run_id in DB for “last run’s id” to avoid renames in many places; alternatively last_chat_run_id. Prefer last_run_id to limit scope; only table/type renames and task_id→chat_id.)
- Table `task_run` → `chat_run`; column `run_id` → `chat_run_id`, `task_id` → `chat_id`.
- Table `artifact`: columns `task_id` → `chat_id`, `task_run_id` → `chat_run_id`.

**Blob storage (S3/FS)**

- Object key path: `.../tasks/<id>/<runId>/...` → `.../chats/<chatID>/<chatRunID>/...`. Function params: workspaceID, chatID, chatRunID (or runID kept internally; design: use chatID, chatRunID for clarity).
- Artifact key: `.../artifacts/<taskID>/<runID>/...` → `.../artifacts/<chatID>/<chatRunID>/...`.

**Config paths (runtime dirs on disk)**

- `.../tasks/<taskID>` → `.../chats/<chatID>`; `.../tasks/<taskID>/<runID>/buildmax` → `.../chats/<chatID>/<chatRunID>/buildmax`. Function names: RuntimeWorkspaceDir(workspaceID, chatID), RuntimeChatBuildmaxDir, RuntimeChatRunBuildmaxDir, RuntimeChatRunDir, RuntimeChatWSDir, ArtifactDir(..., chatID, chatRunID, ...).

## Backend: file-level design

**internal/model/models.go**

- Rename `Task` → `Chat`; struct fields `TaskID` → `ChatID`, `LastRunID` → keep or `LastChatRunID` (keep `LastRunID` to avoid broad renames). TableName `"chat"`. JSON tags `task_id` → `chat_id`, `last_run_id` stays.
- Rename `TaskRun` → `ChatRun`; `RunID` → `ChatRunID`, `TaskID` → `ChatID`; TableName `"chat_run"`; JSON `run_id` → `chat_run_id`, `task_id` → `chat_id`.
- `Artifact`: `TaskID` → `ChatID`, `TaskRunID` → `ChatRunID`; JSON and gorm tags updated.
- `ArtifactWithTask` → `ArtifactWithChat`; fields `TaskID` → `ChatID`, `TaskRunID` → `ChatRunID`, `TaskInputSnippet` → `ChatInputSnippet` (or keep TaskInputSnippet as “snippet of chat input”; prefer `ChatInputSnippet`).

**internal/storage/entity**

- **interfaces.go**: `TaskStore` → `ChatStore` (ListChatsByWorkspace, ListChatsByWorkspacePaginated, GetChat, GetChatBySessionID, CreateChat, UpdateChatStatus, UpdateChatStatusIf, IncrementChatSeq). `TaskRunStore` → `ChatRunStore` (CreateChatRun, GetNextPendingChatRun, GetChatRun, GetChatRunWithChat, UpdateChatRunStatusIf, UpdateChatRunStatus, UpdateChatRunWorkerInfo, OnRunComplete, SyncChatFromRun). ErrRunInProgress comment “chat has a run already in progress”.
- **types.go**: If it re-exports model types, `Task` → `Chat`, `TaskRun` → `ChatRun`.
- **task.go** → **chat.go**: All methods updated to Chat, chat_id, ChatStore contract; table/column references to chat, chat_id, last_run_id (or last_chat_run_id).
- **task_run.go** → **chat_run.go**: All methods updated to ChatRun, chat_run_id, chat_id; ChatRunStore contract.
- **artifact.go**: CreateArtifactWithItem(ctx, chatID, chatRunID, ...); ListArtifactsByWorkspace(ctx, workspaceID, chatID *string); SELECT/join use chat_id, chat_run_id; ArtifactWithChat.
- **store.go**: AutoMigrate(&Chat{}, &ChatRun{}, ...); comment “implements … ChatStore, ChatRunStore”.
- **store_test.go**: All task/task_run references → chat/chat_run; mock stores and calls updated.

**internal/storage/blob**

- **keys.go**: `TaskBuildmaxObjectKey` → `ChatBuildmaxObjectKey`; path `"tasks"` → `"chats"`; params (prefix, workspaceID, chatID, chatRunID, relPath). `ArtifactResultKey`: params include chatID, chatRunID; path still `artifacts/<chatID>/<chatRunID>/<artifactID>/result.md`.
- **interfaces.go**: PersistStorage `PutTaskBuildmax` → `PutChatBuildmax`, `GetTaskBuildmax` → `GetChatBuildmax` (params workspaceID, chatID, chatRunID, relPath). ArtifactStorage PutResult/GetResult params (..., chatID, chatRunID, artifactID).
- **s3_persist.go**, **localfs_persist.go**: Implement PutChatBuildmax, GetChatBuildmax with ChatBuildmaxObjectKey.
- **s3_artifact.go**, **localfs_artifact.go**: PutResult/GetResult use chatID, chatRunID; artifactDir func(workspaceID, chatID, chatRunID, artifactID string).
- **keys_test.go**: Update expected paths to use "chats" and param names.

**internal/config/config.go**

- `RuntimeWorkspaceDir(workspaceID, taskID)` → `RuntimeWorkspaceDir(workspaceID, chatID)`; path `"tasks"` → `"chats"`.
- `RuntimeTaskBuildmaxDir` → `RuntimeChatBuildmaxDir(workspaceID, chatID)`.
- `RuntimeTaskRunBuildmaxDir` → `RuntimeChatRunBuildmaxDir(workspaceID, chatID, chatRunID)`.
- `RuntimeTaskRunDir` → `RuntimeChatRunDir(workspaceID, chatID, chatRunID)`.
- `RuntimeTaskWSDir` → `RuntimeChatWSDir(workspaceID, chatID)`.
- `ArtifactDir(workspaceID, taskID, runID, artifactID)` → `ArtifactDir(workspaceID, chatID, chatRunID, artifactID)`.
- **config_test.go**: Update calls and expected paths.

**internal/server**

- **server.go**: Config holds ChatStore, ChatRunStore (replacing TaskStore, TaskRunStore). Routes: `GET/POST /api/workspaces/{workspace_id}/chats`, `POST .../chats/{chat_id}/runs`, `GET .../chats/{chat_id}/conversation`. Worker: `GET/PATCH /api/worker/chat-runs/{chat_run_id}`.
- **tasks.go** → **chats.go**: listWorkspaceChatsHandler, createWorkspaceChatHandler, createChatRunHandler, getChatConversationHandler; path value `chat_id`; request/response types use chat_id, chat_run_id; call ChatStore/ChatRunStore.
- **artifacts.go**: Query param `task_id` → `chat_id`; response and internal types chat_id, chat_run_id; ListArtifactsByWorkspace(ctx, workspaceID, chatIDPtr).
- **worker_handlers.go**: Path prefix `/api/worker/chat-runs/`, path value chat_run_id; build GetChatRunResponse (run with chat_run_id, chat_id; chat with chat_id, ...); handlers getChatRunHandler, patchChatRunHandler.
- **helpers_test.go**: Mock ChatStore, ChatRunStore; all handler tests use chat_id, chat_run_id.
- **tasks_test.go** → **chats_test.go**: Same cases with /chats and chat_id.
- **static/openapi.json**: Paths `/tasks` → `/chats`, `task_id` → `chat_id`; schemas and examples; worker path `task-runs` → `chat-runs`, `run_id` → `chat_run_id`.

**internal/workerapi/types.go**

- GetTaskRunResponse → GetChatRunResponse; TaskRunRun → ChatRunRun (Run → ChatRunRun with json chat_run_id, chat_id); TaskRunTask → ChatRunChat (Chat portion: chat_id, workspace_id, session_id, last_run_id or last_chat_run_id). PatchTaskRunRequest → PatchChatRunRequest. All JSON snake_case: chat_id, chat_run_id.

**internal/executor**

- **worker_api.go**: URL `/api/worker/chat-runs/` + chatRunID; parse GetChatRunResponse; return (*entity.ChatRun, *entity.Chat, error). ErrTaskAlreadyClaimed → ErrChatRunAlreadyClaimed.
- **executor.go**: RunTask(ctx, chat *entity.Chat, run *entity.ChatRun, ...); TaskRunUpdater → ChatRunUpdater; paths use config.RuntimeChatRunBuildmaxDir etc.; internal vars chatID, chatRunID.
- **paths.go**: WorkspacePaths interface: RuntimeWorkspaceDir(workspaceID, chatID), RuntimeChatBuildmaxDir, RuntimeChatRunBuildmaxDir, RuntimeChatWSDir, ArtifactDir(..., chatID, chatRunID, ...).
- **runner.go**: WorkerRunner.Run(ctx, run entity.ChatRun); LocalRunner passes --chat-run-id.
- **k8s.go**: Run(ctx, run entity.ChatRun); job args --chat-run-id.
- **scheduler.go**: taskRuns → chatRuns (field); entity.ChatRunStore; GetNextPendingChatRun, UpdateChatRunStatusIf, etc.
- **executor_test.go**: Fake storage and paths use chatID, chatRunID; mockChatRunStore; TestRunTask etc. with Chat, ChatRun.

**internal/servercmd/run.go**

- Pass ChatStore, ChatRunStore from Store (Store implements them); no new deps.

**internal/workercmd/run.go**

- RunWorker(ctx, chatRunID string); flag/param from --chat-run-id. GetWorkerChatRun(ctx, baseURL, token, chatRunID, nil). UpdateRunStatus(ctx, run.ChatRunID, ...). ErrAlreadyClaimed message “chat run already claimed”. All log keys run_id → chat_run_id where appropriate.

**cmd/buildmax-worker/main.go**

- Flag `task-run-id` → `chat-run-id`; variable runID → chatRunID; pass to workercmd.RunWorker(ctx, *chatRunID).

## Frontend: file-level design

**portal/src/lib/types.ts**

- `Task` → `Chat`; fields id, projectId? (remove if 072 done), sessionId?, title, status, timeLabel, summary. Prefer `chatId` only (id). Remove Project if 072 done.
- `WorkspaceScope`: `taskId?` → `chatId?`.
- Route types: already "chat"/"chats"; ensure `taskId` in route payload → `chatId` (e.g. `{ name: "chat"; workspaceId: string; chatId: string }`).
- `ViewArtifactParams`: keep workspaceId, artifactId; no change unless artifact type gains chatId (already has taskId→chatId in Artifact type).
- Artifact: `taskId` → `chatId`; remove projectId if 072 done.

**portal/src/lib/api/types.ts**

- `ApiTask` → `ApiChat`; fields id (or chat_id from server), workspace_id, project_id removed if 072 done, session_id, status, input, title, output, created_by, created_at, started_at, ended_at, error_message. JSON names: chat_id (server sends chat_id; frontend can keep id as alias for chat_id in ApiChat).
- `ApiTasksListResponse` → `ApiChatsListResponse`; tasks → chats.
- `CreateTaskRunResponse` → `CreateChatRunResponse`; run_id → chat_run_id, task_id → chat_id.
- `ApiArtifact`: task_id → chat_id, task_run_id → chat_run_id; remove project_id if 072 done.
- Remove ApiProject if 072 done.

**portal/src/lib/api/index.ts**

- getTasks → getChats; URL .../chats; no projectId param (or drop if 072 done).
- getTasksPaginated → getChatsPaginated; URL .../chats; return ApiChatsListResponse.
- createTask → createChat; URL POST .../chats; body { input } (no project_id).
- createTaskRun → createChatRun; URL POST .../chats/{chatId}/runs; body { input }; parse CreateChatRunResponse (chat_run_id, chat_id).
- getTaskConversation → getChatConversation; URL .../chats/{chatId}/conversation.
- getArtifacts: option taskId → chatId; query param task_id → chat_id.
- Export types: ApiChat, ApiChatsListResponse, CreateChatRunResponse; export getChats, getChatsPaginated, createChat, createChatRun, getChatConversation.

**portal/src/lib/api/mappers.ts**

- apiTaskToTask → apiChatToChat(api: ApiChat): Chat; map chat_id to id, project_id drop; taskStatusToUI unchanged (status values same).
- apiArtifactToArtifact: map chat_id to chatId (or taskId→chatId on Artifact type).

**portal/src (components, pages, hooks)**

- **router.ts**: Route definitions already use "chat"/"chats"; params taskId → chatId where a route param holds the chat id.
- **pages/Chats.tsx**, **NewChat.tsx**, **WorkspaceHome.tsx**, **TaskDetail.tsx**: Use chatId, getChats, createChat, createChatRun, getChatConversation; TaskDetail may be renamed ChatDetail or stay as page name with Chat type inside.
- **hooks/useWorkspaceTasks.ts** → **useWorkspaceChats.ts** (or keep filename and export useWorkspaceChats); return chats, loading, createChat, etc.; use getChats/getChatsPaginated and createChat.
- **hooks/useArtifacts.ts**: Options taskId → chatId; getArtifacts(..., { chatId }).
- **contexts/WorkspaceContext.tsx**, **layout/LeftSidebar.tsx**, **WorkspaceRouter.tsx**: Any task/taskId refs → chat/chatId; links to chat use chatId.
- **components/ArtifactContentModal.tsx**: artifact.taskId → artifact.chatId if displayed or passed.
- **lib/taskStatus.ts**: Can stay (status values same) or rename to chatStatus.ts; only type name Task might become Chat in signature if used.
- **lib/workspace.ts**: Any task references → chat.
- **App.tsx**, **Layout.tsx**, **index.css**: Text/labels "task"→"chat" where user-facing; class names if any (e.g. .task-detail → .chat-detail) optional.

## Method design (key renames)

| Layer | Old | New |
|------|-----|-----|
| **model** | Task, TaskRun, TaskID, TaskRunID, task_id, task_run_id | Chat, ChatRun, ChatID, ChatRunID, chat_id, chat_run_id |
| **entity** | TaskStore, TaskRunStore, ListTasksByWorkspace, CreateTask, GetTaskRunWithTask, CreateTaskRun, ... | ChatStore, ChatRunStore, ListChatsByWorkspace, CreateChat, GetChatRunWithChat, CreateChatRun, ... |
| **blob** | PutTaskBuildmax, GetTaskBuildmax, TaskBuildmaxObjectKey, taskID, runID | PutChatBuildmax, GetChatBuildmax, ChatBuildmaxObjectKey, chatID, chatRunID |
| **config** | RuntimeTaskBuildmaxDir, RuntimeTaskRunBuildmaxDir, RuntimeTaskWSDir, RuntimeTaskRunDir, taskID, runID | RuntimeChatBuildmaxDir, RuntimeChatRunBuildmaxDir, RuntimeChatWSDir, RuntimeChatRunDir, chatID, chatRunID |
| **server** | listWorkspaceTasksHandler, createWorkspaceTaskHandler, createTaskRunHandler, getTaskConversationHandler; path task_id | listWorkspaceChatsHandler, createWorkspaceChatHandler, createChatRunHandler, getChatConversationHandler; path chat_id |
| **worker** | GET/PATCH /api/worker/task-runs/{run_id} | GET/PATCH /api/worker/chat-runs/{chat_run_id} |
| **workerapi** | GetTaskRunResponse, TaskRunRun, TaskRunTask, PatchTaskRunRequest | GetChatRunResponse, ChatRunRun, ChatRunChat, PatchChatRunRequest |
| **executor** | GetWorkerTaskRun, TaskRunUpdater, RunTask(ctx, task, run, ...) | GetWorkerChatRun, ChatRunUpdater, RunTask(ctx, chat, run, ...) |
| **worker binary** | --task-run-id | --chat-run-id |
| **portal types** | Task, taskId, ApiTask, getTasks, createTask, createTaskRun, getTaskConversation | Chat, chatId, ApiChat, getChats, createChat, createChatRun, getChatConversation |

## How they work together

**Request flow (unchanged behavior)**

1. User opens chats in workspace → GET /api/workspaces/{id}/chats → ChatStore.ListChatsByWorkspace → response with chat_id, etc.
2. User creates chat → POST .../chats { input } → ChatStore.CreateChat + first ChatRun → 201 with chat_id.
3. User triggers run → POST .../chats/{chat_id}/runs → ChatRunStore.CreateChatRun → 201 with chat_run_id, chat_id.
4. Scheduler polls ChatRunStore.GetNextPendingChatRun → spawns worker with --chat-run-id {chat_run_id}.
5. Worker GET /api/worker/chat-runs/{chat_run_id} → server returns run + chat; worker runs executor.RunTask(chat, run, ...); PATCH to update status/artifact.
6. Frontend GET .../chats/{chat_id}/conversation and GET .../artifacts?chat_id=... use chat_id.

**Database migration**

- No backward compatibility: either (A) fresh install with new schema (chat, chat_run, artifact with chat_id/chat_run_id), or (B) one-time migration: rename table task→chat, task_run→chat_run; rename columns (task_id→chat_id in chat and artifact; in task_run run_id→chat_run_id, task_id→chat_id; in artifact task_id→chat_id, task_run_id→chat_run_id). GORM AutoMigrate with new struct names will create new tables; existing data requires explicit migration script (ALTER TABLE rename, then rename columns). Design recommends: for clean slate, drop old tables and rely on AutoMigrate; for preserving data, write a small migration that renames tables and columns to match new names.

## Changes for review

**Backend (Go)**

- **internal/model/models.go**: Task→Chat, TaskRun→ChatRun; Artifact task_id→chat_id, task_run_id→chat_run_id; ArtifactWithTask→ArtifactWithChat, TaskInputSnippet→ChatInputSnippet; table names chat, chat_run; all JSON/gorm tags.
- **internal/storage/entity**: interfaces.go (ChatStore, ChatRunStore, method renames); types.go (Chat, ChatRun); task.go→chat.go; task_run.go→chat_run.go; artifact.go (chat_id, chat_run_id, ArtifactWithChat); store.go (AutoMigrate Chat, ChatRun); store_test.go.
- **internal/storage/blob**: keys.go (ChatBuildmaxObjectKey, path "chats", params chatID/chatRunID); interfaces.go (PutChatBuildmax, GetChatBuildmax; ArtifactStorage params); s3_persist.go, localfs_persist.go, s3_artifact.go, localfs_artifact.go; keys_test.go.
- **internal/config/config.go**: RuntimeWorkspaceDir path "chats"; RuntimeChatBuildmaxDir, RuntimeChatRunBuildmaxDir, RuntimeChatRunDir, RuntimeChatWSDir, ArtifactDir; param names chatID, chatRunID. config_test.go.
- **internal/server**: server.go (routes /chats, chat_id; worker /chat-runs/{chat_run_id}; Config ChatStore, ChatRunStore); tasks.go→chats.go (handlers and types); artifacts.go (chat_id query/response); worker_handlers.go (chat-runs, GetChatRunResponse); helpers_test.go, tasks_test.go→chats_test.go; static/openapi.json.
- **internal/workerapi/types.go**: GetChatRunResponse, ChatRunRun, ChatRunChat, PatchChatRunRequest; JSON chat_id, chat_run_id.
- **internal/executor**: worker_api.go (URL, GetWorkerChatRun, entity.ChatRun/Chat); executor.go (Chat, ChatRun, ChatRunUpdater, path helpers); paths.go (WorkspacePaths); runner.go, k8s.go (ChatRun, --chat-run-id); scheduler.go (ChatRunStore, GetNextPendingChatRun, etc.); executor_test.go.
- **internal/servercmd/run.go**: Config ChatStore, ChatRunStore.
- **internal/workercmd/run.go**: RunWorker(chatRunID), GetWorkerChatRun, run.ChatRunID, ErrAlreadyClaimed message.
- **cmd/buildmax-worker/main.go**: Flag chat-run-id, chatRunID.

**Frontend (Portal)**

- **portal/src/lib/types.ts**: Task→Chat, taskId→chatId; WorkspaceScope.taskId→chatId; Route chat payload chatId; Artifact taskId→chatId.
- **portal/src/lib/api/types.ts**: ApiTask→ApiChat, ApiTasksListResponse→ApiChatsListResponse, CreateTaskRunResponse→CreateChatRunResponse; fields chat_id, chat_run_id; remove ApiProject/project_id if 072 done.
- **portal/src/lib/api/index.ts**: getChats, getChatsPaginated, createChat, createChatRun, getChatConversation; URLs /chats, chat_id; getArtifacts option chatId.
- **portal/src/lib/api/mappers.ts**: apiChatToChat, artifact chatId.
- **portal/src/router.ts**: Param chatId where applicable.
- **portal/src/pages**: Chats, NewChat, WorkspaceHome, TaskDetail (or ChatDetail): use chatId, getChats, createChat, createChatRun, getChatConversation.
- **portal/src/hooks**: useWorkspaceTasks→useWorkspaceChats (or same file, export useWorkspaceChats); useArtifacts chatId.
- **portal/src/contexts, layout, components**: task→chat, taskId→chatId, artifact.chatId.
- **portal/src/lib/taskStatus.ts** (optional rename to chatStatus) and **workspace.ts**: Chat type if needed.
