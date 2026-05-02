# Backend Architecture Refactor Blueprint

## Goal

Refactor the backend under `cmd/` and `internal/` toward:

- clear module boundaries
- one orchestration path per product flow
- well-defined types instead of long stringly APIs
- application services that own business rules
- infrastructure packages that stop leaking upward

This is an incremental plan, not a rewrite.

---

## Current problems

### 1. Tier 1 conversation has two entry points

- Portal chat-run creation goes through `ConversationEngine.Process`.
- Portal conversation messaging calls `conversation.RunLoop` / `RunLoopStream` directly.

That means the project has both a contract-style conversation layer and a concrete loop-style conversation layer, but no single orchestration surface.

### 2. HTTP handlers own business rules

Portal handlers currently do more than transport concerns:

- agent input expansion
- title generation
- quota checks
- chat creation
- background task creation
- branching between new and legacy flows

This makes handlers large and duplicates logic across files.

### 3. Worker execution depends on the CLI binary contract

The worker shells out to `buildmax -p`, then reads files written by CLI-side logic. The CLI also imports `executor` to write usage metadata. That is a layering inversion.

### 4. Model/storage separation is half-finished

`internal/model` looks like a domain package, but it still embeds GORM tags and table names. `internal/infra/db` aliases those same types. This adds indirection without a true boundary.

### 5. Lifecycle/state transitions are stringly typed

Run and task status are updated through wide repository methods with many optional pointer arguments. Status transitions are spread across handlers, scheduler, worker, and repositories.

### 6. One concrete store is injected everywhere

`entity.Store` is convenient, but it weakens bounded contexts because every module can depend on the same broad surface area.

---

## Target architecture

The target shape is a simple layered backend:

1. `cmd/*`
Binary entrypoints only.

2. `internal/core/*`
Application services and use cases. This is where business workflows live.

3. `internal/domain/*`
Pure domain types, commands, statuses, and narrow repository interfaces.

4. `internal/infra/*`
Database, blob storage, HTTP transport, executor processes, and LLM clients.

5. `internal/ui/*`
CLI/TUI/HTTP transport adapters.

This is not strict clean architecture for its own sake. The main objective is to move rules out of transport and out of infrastructure.

---

## Proposed package layout

### Keep

- `cmd/buildmax`
- `cmd/buildmax-server`
- `cmd/buildmax-worker`
- `internal/config`
- `internal/infra/log`
- `internal/infra/llm`
- `internal/execution/agenttool`
- `internal/session`
- `internal/interface/tui`
- `internal/util`

### Introduce

```text
internal/
  app/
    agentrun/          # shared agent execution runtime used by CLI and worker
    chat/              # create chat, create run, start background task
    conversation/      # single HandleTurn orchestration for Tier 1
    workspace/         # file browsing/materialization use cases if needed
    auth/              # signup/login application logic if server grows
  domain/
    chat/
      types.go         # Chat, TaskRun, commands, events
      status.go        # typed run/task statuses
      repository.go    # ChatRepository, RunRepository
    conversation/
      types.go         # Conversation, Message, Turn, Result
      repository.go
    agent/
      types.go
    workspace/
      types.go
    usage/
      types.go
  infra/
    db/
      chatrepo/
      conversationrepo/
      userrepo/
    blob/
      persist/
      artifact/
    http/
      server/
      portal/
      worker/
    runner/
      local/
      k8s/
```

### Collapse or rename

- `internal/server` becomes transport-only HTTP wiring under `internal/infra/http`
- `internal/infra/db` becomes DB repository implementations under `internal/infra/db`
- `internal/model` is either removed, or converted into pure `internal/domain/*` types
- `internal/core/conversation/adapter` moves under HTTP transport or conversation app layer depending on responsibility

---

## Module responsibilities

### `app/agentrun`

Own the reusable agent execution runtime currently split across CLI setup and executor shelling.

Responsibilities:

- build tool set
- build agent types
- compose effective prompt
- execute one prompt against one session
- return output, usage, and updated session

Public shape:

```go
type RunInput struct {
    WorkspaceDir string
    SessionID    string
    Prompt       string
    Model        string
    Stream       llm.StreamSink
}

type RunOutput struct {
    Reply            string
    Session          *session.Session
    PromptTokens     int
    CompletionTokens int
    ToolCalls        int
}

type Runner interface {
    Run(ctx context.Context, in RunInput) (RunOutput, error)
}
```

Consumers:

- CLI print mode
- TUI submit flow
- worker task execution

Result:

- worker no longer shells out to `buildmax`
- CLI no longer imports `executor` to write usage files

### `app/chat`

Own chat-related use cases that currently live in portal handlers.

Responsibilities:

- resolve chat input from optional agent
- generate title
- check quota
- create chat
- create run
- start background task

Suggested public methods:

```go
type Service interface {
    CreateChat(ctx context.Context, cmd CreateChatCmd) (*chat.Chat, error)
    CreateRun(ctx context.Context, cmd CreateRunCmd) (*chat.Run, error)
    StartBackgroundChat(ctx context.Context, cmd StartBackgroundChatCmd) (*StartBackgroundChatResult, error)
}
```

### `app/conversation`

Provide the single Tier 1 orchestration surface.

Responsibilities:

- normalize a turn
- load prior conversation state
- run LLM loop
- persist messages
- emit direct reply and/or background task creation

Suggested public method:

```go
type Service interface {
    HandleTurn(ctx context.Context, cmd HandleTurnCmd) (HandleTurnResult, error)
}
```

Important rule:

- HTTP handlers do not call `RunLoop` directly
- chat-run creation does not bypass the conversation service

### `domain/chat`

Hold pure types and transition logic.

Suggested core types:

```go
type RunStatus string

const (
    RunPending   RunStatus = "PENDING"
    RunScheduled RunStatus = "SCHEDULED"
    RunRunning   RunStatus = "RUNNING"
    RunSucceeded RunStatus = "SUCCEEDED"
    RunFailed    RunStatus = "FAILED"
)

type CreateChatCmd struct {
    WorkspaceID string
    Input       string
    Title       string
    CreatedBy   string
    AgentID     *string
    ConversationID *string
}

type CompleteRunCmd struct {
    RunID             string
    Output            string
    RelativePaths     []string
    EndedAt           int64
    PromptTokens      *int
    CompletionTokens  *int
}
```

Key change:

- repository methods become command-oriented
- no more wide `UpdateTaskRunStatus(... many pointers ...)`

### `infra/db/*`

Own GORM structs and repository implementations.

Rule:

- GORM tags stay here
- SQL-specific migration logic stays here
- app/domain packages do not import GORM

### `infra/http/*`

Own request/response DTOs and route registration.

Rule:

- HTTP handlers decode, authorize, call app services, encode
- business rules should not live here

---

## Recommended concrete refactors

## Refactor A: create a shared `app/agentrun` runtime

### Why

This removes the current worker-to-CLI binary dependency and fixes the layering inversion around usage files.

### Move code from

- `internal/interface/cli/setup.go`
- `internal/interface/cli/print.go`
- parts of `internal/core/agent`
- parts of `internal/execution`

### End state

- CLI becomes a thin presentation wrapper around `agentrun.Run`
- worker uses the same runtime directly
- usage is returned as data, not discovered via a sidecar file

### Migration notes

Do this before touching executor orchestration heavily.

---

## Refactor B: extract `app/chat.Service`

### Why

This removes duplicated chat business logic from portal handlers.

### Move code from

- `internal/server/portal/chats.go`
- `internal/server/portal/conversation_tools.go`

### Methods to extract first

- `ResolveInput`
- `GenerateTitle`
- `CreateChat`
- `CreateRun`
- `StartBackgroundChat`

### Handler end state

Current:

- decode body
- resolve input
- title generation
- quota check
- create chat
- write response

Target:

- decode body
- authorize
- call `chatService.CreateChat`
- write response

---

## Refactor C: unify Tier 1 behind `app/conversation.Service`

### Why

There should be one way to process a user turn.

### Replace

- direct calls to `conversation.RunLoop`
- direct calls to `conversation.RunLoopStream`
- separate `ConversationEngine.Process` path for portal task runs

### With

```go
type HandleTurnCmd struct {
    WorkspaceID     string
    ConversationID  string
    Channel         string
    UserID          string
    Message         string
    StreamSink      llm.StreamSink
}
```

### Result

- one orchestration path
- one place to add policies
- one place to decide direct reply vs background task vs tools

---

## Refactor D: replace wide repository methods with command methods

### Why

Today the store API leaks persistence concerns upward and makes transitions hard to reason about.

### Replace patterns like

```go
UpdateTaskRunStatus(ctx, id, status, startedAt, endedAt, output, errorMessage, sessionID, promptTokens, completionTokens)
```

### With

```go
ClaimRun(ctx, runID string) (bool, error)
MarkRunRunning(ctx, cmd MarkRunRunningCmd) error
CompleteRun(ctx, cmd CompleteRunCmd) error
FailRun(ctx, cmd FailRunCmd) error
```

### Result

- transition semantics become explicit
- call sites become readable
- fewer invalid combinations of nils and statuses

---

## Refactor E: finish the model boundary

### Preferred direction

Create pure domain packages and move GORM models into DB repositories.

### Alternative

If that is too much for now, remove `internal/model` and let `internal/infra/db` be the storage model package until the real split is funded.

### Recommendation

Do not keep the current alias-based middle state long term.

---

## Suggested phased migration

## Phase 1: stabilize orchestration seams

Scope:

- introduce `app/agentrun`
- update CLI to use it
- update worker to use it
- keep current HTTP APIs

Benefits:

- immediate layering improvement
- least product risk

Non-goals:

- no DB model redesign yet

## Phase 2: move chat rules out of handlers

Scope:

- add `app/chat.Service`
- refactor portal chat handlers to use it
- refactor background task creation tool path to use it

Benefits:

- remove duplication
- shrink handlers

## Phase 3: unify conversation flow

Scope:

- add `app/conversation.Service`
- migrate portal conversation endpoints
- migrate portal chat-run creation path
- reduce `internal/core/conversation` to either domain contracts or runtime internals, not both

Benefits:

- one Tier 1 path
- easier future channel support

## Phase 4: typed lifecycle and repository cleanup

Scope:

- add typed statuses
- replace wide status update methods with command methods
- move transition logic behind repositories/services

Benefits:

- better correctness
- cleaner executor and worker code

## Phase 5: finish domain/infrastructure split

Scope:

- create pure domain types
- move GORM models into `infra/db`
- stop aliasing storage structs as domain

Benefits:

- the cleanest abstraction boundary
- easier future persistence changes

---

## Recommended file-by-file first moves

### First move set

- create `internal/execution/agentrun`
- migrate logic from `internal/interface/cli/setup.go`
- make `internal/interface/cli/print.go` call `agentrun`
- make `internal/execution` call `agentrun` instead of `exec.Command("buildmax", ...)`

### Second move set

- create `internal/core/chat/service.go`
- move `buildChatInputFromAgent`
- move title generation helper
- move quota + create-chat logic
- move background task creation logic

### Third move set

- create `internal/core/conversation/service.go`
- move `RunLoop` and `RunLoopStream` internals behind service methods
- make both portal conversation endpoints and chat-run creation call the same service

### Fourth move set

- add `domain/chat/status.go`
- add lifecycle command structs
- adapt executor/worker/server to use them

### Fifth move set

- replace `internal/model` with `internal/domain/*`
- move GORM models under `infra/db`

---

## Package ownership rules

Use these rules during refactors to avoid drifting back:

- `cmd/*` may wire modules, but should not own business rules.
- `infra/http/*` may validate transport input, but should not generate titles, expand agents, or decide run transitions.
- `app/*` may depend on domain interfaces and infrastructure adapters.
- `domain/*` must not import GORM, `net/http`, or Cobra/Bubble Tea.
- `infra/db/*` may map between DB rows and domain types.
- `executor` should orchestrate execution mechanics, not define business semantics for run completion.

---

## What not to do

- Do not move everything at once.
- Do not create a generic “service” package with unrelated logic.
- Do not keep both old and new orchestration paths alive longer than necessary.
- Do not introduce domain packages that still contain GORM tags.
- Do not make handlers smarter while refactoring services out of them.

---

## Success criteria

The refactor is working when:

- worker no longer shells out to `buildmax`
- CLI no longer imports `executor`
- portal handlers mostly decode/auth/call service/respond
- all Tier 1 user turns flow through one service entry point
- run status changes happen through typed commands or transition methods
- domain packages are free of persistence tags

---

## Short recommendation

If only one refactor gets funded soon, do this order:

1. `app/agentrun`
2. `app/chat.Service`
3. `app/conversation.Service`

That sequence gives the highest architecture payoff with the lowest migration risk.
