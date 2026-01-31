# Design 007 - Chat session (in-memory)

## Goal

Introduce an in-memory chat session (id, title, created_at, conversation history) and integrate it with the agent run loop so multi-turn chats use and update the same session, with messages appended each turn and LLM calls made over the full history.

## Modules

| Module (package) | Responsibility | Owns |
|------------------|----------------|------|
| **internal/session** (new) | In-memory chat session: id (UUID), title, created_at, and conversation history (user/assistant/tool messages). | `Session` struct, `NewSession`, `Append`, `Messages`, `ID`, `Title`, `CreatedAt`, `SetTitle`; `session.go`, `session_test.go`. |
| **internal/agent** | Core agent loop and session-aware processing. | `Agent`, `Process`, **`ProcessWithSession`** (new); `agent.go`, `agent_test.go`. |
| **internal/llm** | LLM message types; unchanged. | `Message`, `ToolDef`, `ToolCall`; `types.go`. |
| **cmd/buildmax** | CLI entry; wire prompt mode to session. | `main.go` — in runPromptMode, create session and call ProcessWithSession. |

## Structure

**Directory / files**

- `internal/session/` — in-memory chat session (new package)
  - `session.go` — `Session` struct, `NewSession`, `Append`, `Messages`, `ID`, `Title`, `CreatedAt`, `SetTitle`
  - `session_test.go` — unit tests for session (NewSession UUID/CreatedAt, Append, Messages copy/order)
- `internal/agent/` — agent loop; add session-aware processing
  - `agent.go` — add `ProcessWithSession(ctx, session, userMessage)`; `Process` unchanged
  - `agent_test.go` — add `TestProcessWithSession` (two turns, full history on second LLM call)
- `cmd/buildmax/` — CLI; wire prompt mode to session
  - `main.go` — in runPromptMode: create one session with `session.NewSession("")`, call `ProcessWithSession(ctx, session, prompt)` instead of `Process`; each run gets a new session

**Main types and interfaces**

- **Session** (internal/session): In-memory container. Fields (unexported or exported as needed): `id` (string, UUID), `title` (string), `createdAt` (time.Time), `messages` ([]llm.Message). No dependency on agent.
- **Agent** (internal/agent): Unchanged shape; add method `ProcessWithSession`. Depends on `internal/session` for `*session.Session` and on `internal/llm` for `Message`, `ToolDef`, `ToolCall`.
- **llm.Message** (internal/llm): Unchanged; used by session for conversation history and by agent for LLM calls.

## Method design

| Receiver | Method | Signature | Responsibility |
|----------|--------|-----------|-----------------|
| (package) | NewSession | `(title string) *Session` | Create session with new UUID (e.g. uuid.New().String()), given title, created_at = time.Now(), empty messages. |
| **Session** | Append | `(msg llm.Message)` | Append one message to internal history. Caller ensures role is user/assistant/tool. |
| **Session** | Messages | `() []llm.Message` | Return a **copy** of the current conversation history (new slice, same length and elements). |
| **Session** | ID | `() string` | Return session id (UUID string). |
| **Session** | Title | `() string` | Return session title. |
| **Session** | CreatedAt | `() time.Time` | Return creation timestamp. |
| **Session** | SetTitle | `(title string)` | Set session title (optional; for future use). |
| **Agent** | Process | `(ctx, userMessage) (reply string, err error)` | Unchanged: stateless single-turn loop. |
| **Agent** | ProcessWithSession | `(ctx context.Context, session *session.Session, userMessage string) (reply string, err error)` | Append user message to session; build messages = [system] + session.Messages(); run same loop as Process but append each assistant/tool message to session and refresh messages from session each iteration; return final assistant content. |

## How they work together

**Data/control flow (prompt mode: one session per run)**

1. User runs `buildmax -p PROMPT`. In runPromptMode: create one session with `session.NewSession("")`, then call `reply, err := agent.ProcessWithSession(ctx, session, PROMPT)`. Print reply and exit. Each process run creates a new session (no persistence).

**Data/control flow (multi-turn with session, e.g. future TUI)**

1. Caller creates a session: `s := session.NewSession("My chat")` (id and created_at set inside).
2. For each user turn, caller invokes `reply, err := agent.ProcessWithSession(ctx, s, userInput)`.
3. ProcessWithSession: append `{Role: "user", Content: userInput}` to `s`; set `messages = append([]llm.Message{{Role: "system", Content: DefaultSystemPrompt}}, s.Messages()...)`.
4. Loop: call `caller.ChatWithTools(ctx, messages, a.toolDefs)`. If no tool calls, return content. Otherwise append assistant message (and tool results) to `s`, then set `messages = system + s.Messages()` and repeat.
5. Session `s` now holds full history (user, assistant, tool) for all turns; next turn’s ProcessWithSession will send that full history to the LLM (with system prepended).

**Dependencies**

- **internal/session** depends on **internal/llm** (for `llm.Message`) and **github.com/google/uuid** (for id generation). No dependency on internal/agent.
- **internal/agent** depends on **internal/llm** (unchanged) and **internal/session** (for `*session.Session` only in `ProcessWithSession`).

**Key data structures**

- **Session**: Created by `session.NewSession(title)`; mutated by `Session.Append` (and by agent via same session reference). Consumed by `Agent.ProcessWithSession` for read (Messages) and write (Append).
- **[]llm.Message**: Built in ProcessWithSession as system + session.Messages(); passed to ChatWithTools. Session stores only user/assistant/tool; system is prepended at call time.

## Changes for review

- **New**: `internal/session/` — new package for in-memory chat session.
- **New**: `internal/session/session.go` — `Session` struct (id, title, createdAt, messages), `NewSession(title)`, `Append(msg)`, `Messages()` (copy), `ID()`, `Title()`, `CreatedAt()`, `SetTitle(title)`.
- **New**: `internal/session/session_test.go` — tests for NewSession (UUID format, CreatedAt, Title, empty Messages), Append + Messages (copy semantics, order), ID/Title/CreatedAt.
- **New**: `Agent.ProcessWithSession(ctx, session, userMessage) (reply, err)` in `internal/agent/agent.go` — append user message to session; run agent loop using system + session.Messages(), appending assistant/tool messages to session each iteration; return final reply.
- **New**: `TestProcessWithSession` in `internal/agent/agent_test.go` — two turns with same session; assert session history contains both user messages and assistant reply(ies); assert second LLM call receives full history (system + first user + first assistant + second user) via recording mock.
- **Modified**: `go.mod` — add `github.com/google/uuid` for session id generation.
- **Modified**: `cmd/buildmax/main.go` — in runPromptMode, create one session with `session.NewSession("")` and call `ProcessWithSession(ctx, session, prompt)` instead of `Process(ctx, prompt)`; each run gets a new session.
- **Unchanged**: `Process`, `internal/llm`, and all other existing packages and types. TUI wiring is left for later tasks.
