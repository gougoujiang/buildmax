# Design 069 — Generate task title from user input

## Goal

Store a short, LLM-generated task title (3–5 words) when creating a task, and expose it via the API so the Portal can display it instead of truncated input.

## Modules

| Module | Responsibility | Owns |
|--------|----------------|------|
| **internal/model** | Domain types for API and DB. | Task (add Title). |
| **internal/storage/entity** | Task persistence; implements TaskStore. | CreateTask(ctx, ..., title, ...); Store, interfaces. |
| **internal/session** | Session + title generation (conversation and single-input). | GenerateTitleFromInput(ctx, client, input); reuses cleanTitle. |
| **internal/server** | HTTP API; create-task handler computes title, passes to store, returns in response. | Config.TaskTitleGenerator; createWorkspaceTaskHandler; TaskResponse.title; taskToResponse. |
| **internal/servercmd** | Server startup; builds optional LLM client and TaskTitleGenerator from env. | RunServer: LoadLLM, NewClient, wire TitleGenerator into server.Config. |
| **portal** | Types and mappers for API task; use title when present. | ApiTask.title (optional); apiTaskToTask uses api.title or fallback. |

## Structure

**Directories / files**

- `internal/model/` — Add `Title` to Task in models.go.
- `internal/session/title.go` — Add task title prompt constant and `GenerateTitleFromInput(ctx, client TitleChatClient, input string) (string, error)`.
- `internal/storage/entity/` — task.go: CreateTask accepts `title string`; interfaces.go: TaskStore.CreateTask signature updated; types alias model.Task so no duplicate struct.
- `internal/server/` — server.go: Config gains `TaskTitleGenerator` interface; tasks.go: createWorkspaceTaskHandler computes title (generator or truncate), TaskResponse.Title, taskToResponse(t).Title.
- `internal/servercmd/run.go` — After building cfg, optionally set TaskTitleGenerator from LoadLLM + llm.NewClient + session adapter.
- `portal/src/lib/api/types.ts` — ApiTask: add optional `title?: string`.
- `portal/src/lib/api/mappers.ts` — apiTaskToTask: use `api.title` when present and non-empty, else current truncated input.

**Main types and interfaces**

- **Task** (model): Add field `Title string` with gorm tag `type:varchar(256)` and json `"title,omitempty"`. Optional; empty for existing rows.
- **TaskTitleGenerator** (server): Interface `GenerateTaskTitle(ctx context.Context, input string) (string, error)`. Implemented by a small adapter that calls session.GenerateTitleFromInput with server-held TitleChatClient.
- **TitleChatClient** (session): Existing; unchanged. Used by GenerateTitleFromInput.

## Method design

| Receiver / Package | Method / Function | Signature | Responsibility |
|--------------------|-------------------|-----------|----------------|
| **session** | GenerateTitleFromInput | `(ctx context.Context, client TitleChatClient, input string) (string, error)` | Build [system, user] messages with task prompt (3–5 words); call client.Chat; return cleanTitle(result). |
| **session** | cleanTitle | (existing, package-private) | Unchanged; reused for task title. |
| **entity.Store** | CreateTask | `(ctx, workspaceID string, projectID *string, input, title, createdBy string) (*Task, error)` | Create task and first run in transaction; set task.Title = title (may be empty). |
| **server** | createWorkspaceTaskHandler | (existing handler) | Before CreateTask: if cfg.TaskTitleGenerator != nil, title = GenerateTaskTitle(ctx, req.Input); on error or empty use truncateTaskTitle(req.Input, 50). Pass title to CreateTask. Respond with taskToResponse including Title. |
| **server** | truncateTaskTitle | `(input string, maxRunes int) string` | Return first maxRunes runes of input; if len(runes) > maxRunes append "…". |
| **server** | taskToResponse | (existing) | Add assignment `Title: t.Title`. |
| **servercmd** | RunServer | (existing) | After building cfg: if llmCfg := config.LoadLLM(); llmCfg.APIKey != "" { client := llm.NewClient(llmCfg); cfg.TaskTitleGenerator = taskTitleGenAdapter{client} }. Else leave nil. |

**Task title adapter (server or servercmd)**

- Type `taskTitleGenAdapter struct { client *llm.Client }` implementing `GenerateTaskTitle(ctx, input string) (string, error)` by building `session.TitleChatFunc(func(ctx, msgs) { return c.client.ChatWithTools(ctx, msgs, nil) })` and calling `session.GenerateTitleFromInput(ctx, adapter, input)`. Adapter can live in server (e.g. server/tasktitle.go) to avoid servercmd importing session; then servercmd only needs to construct the adapter with an interface that server defines (e.g. a factory or the same TaskTitleGenerator with a constructor that takes *llm.Client). Simplest: put adapter in servercmd, so servercmd imports session and llm; server defines only the interface and receives the implementation. So: **server** defines `type TaskTitleGenerator interface { GenerateTaskTitle(ctx, input) (string, error) }` and **servercmd** builds a struct that holds *llm.Client and implements it via session.GenerateTitleFromInput.

## How they work together

**Create-task flow**

1. Client POSTs `{ "input": "...", "project_id": "?" }` to create task.
2. createWorkspaceTaskHandler parses body, resolves project, then computes title: if TaskTitleGenerator != nil, call GenerateTaskTitle(ctx, req.Input); on error or empty result use truncateTaskTitle(req.Input, 50). Otherwise use truncateTaskTitle(req.Input, 50) only.
3. Handler calls TaskStore.CreateTask(ctx, workspaceID, projectID, req.Input, title, userID).
4. Store creates Task (with Title) and first TaskRun in one transaction.
5. Handler returns taskToResponse(task) with Title set; client sees `title` in JSON.
6. Portal apiTaskToTask uses api.title when present and non-empty; else truncates input as today.

**Dependencies**

- **server** depends on **entity** (TaskStore), **model** (Task type in response). Server does not depend on session or llm.
- **servercmd** depends on **server**, **config**, **llm**, **session**. servercmd builds TaskTitleGenerator implementation that uses session.GenerateTitleFromInput and llm.Client.
- **session** depends on **llm** (Message, TitleChatClient usage). No new deps.
- **entity** depends on **model** (Task type). CreateTask gets one extra argument.
- **portal** depends only on API contract (response shape); add optional title field.

**Key data**

- **Task.Title**: Set at create time; persisted; returned in list/get task. Empty for old tasks.
- **TaskTitleGenerator**: Injected into server.Config; nil when LLM not configured; implementation in servercmd calls session.GenerateTitleFromInput with llm.Client wrapped in TitleChatFunc.

## DB migration

- Add column `title` to table `task`: `VARCHAR(256)` or `TEXT`, default `''`, nullable or not (empty string). GORM AutoMigrate in entity.New will add the column when the Task struct gains `Title`.

## Task title prompt (session)

- New constant in session/title.go: `taskTitleSystemPrompt = "Generate a short task title (3-5 words) from this user request. Return ONLY the title, no quotes or punctuation."`
- GenerateTitleFromInput: messages = [system with taskTitleSystemPrompt, user with input]; title, err := client.Chat(ctx, messages); return cleanTitle(title), err.

## Changes for review

- **Modified** `internal/model/models.go` — Add `Title string` to Task with gorm and json tags.
- **New** `internal/session/title.go` — Add taskTitleSystemPrompt and `GenerateTitleFromInput(ctx, client TitleChatClient, input string) (string, error)`.
- **Modified** `internal/storage/entity/task.go` — CreateTask(ctx, workspaceID, projectID *string, input, title, createdBy string); set t.Title = title.
- **Modified** `internal/storage/entity/interfaces.go` — TaskStore.CreateTask signature add `title string` parameter.
- **Modified** `internal/server/server.go` — Config add `TaskTitleGenerator TaskTitleGenerator` (interface type).
- **New** `internal/server/tasktitle.go` (optional) — Define `TaskTitleGenerator` interface. Alternatively define interface in server.go next to Config.
- **Modified** `internal/server/tasks.go` — Add truncateTaskTitle(input, maxRunes); TaskResponse add Title; taskToResponse set Title; createWorkspaceTaskHandler compute title (generator or truncate), pass to CreateTask.
- **Modified** `internal/servercmd/run.go` — If LoadLLM().APIKey != "", build llm.Client and a struct implementing TaskTitleGenerator via session.GenerateTitleFromInput; set cfg.TaskTitleGenerator.
- **Modified** `portal/src/lib/api/types.ts` — ApiTask add `title?: string`.
- **Modified** `portal/src/lib/api/mappers.ts` — apiTaskToTask: title = (api.title && api.title.trim() !== "") ? api.title : (input truncation as now).
