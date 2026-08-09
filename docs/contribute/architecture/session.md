# Session

## Purpose

The `internal/core/session` package models local chat session state. Runtime
session loading and saving is assembled by `internal/agentapp`, which stores
session files under `config.SessionsDir()`.

## Key Types and Interfaces

| Name | Kind | Role |
|------|------|------|
| **Session** | struct | In-memory session: id (UUID), title, created_at, message history |
| **SessionItem** | struct | Session metadata in the list index: id, title, workspace, created_at |
| **sessionFile** | struct (private) | JSON serialization format for individual session files |

## How It Works

### Session Lifecycle

1. **Create**: `NewSession(title)` generates a UUID, sets `created_at` to now, initializes empty history.
2. **Append messages**: `session.Append(msg)` adds user, assistant, or tool messages to history. System messages are not stored (prepended at call time by the agent).
3. **Read messages**: `session.Messages()` returns a defensive copy of the history.
4. **Persist**: `PersistAfterReply()` runs the full save flow after each assistant reply.

### Persistence

Sessions are stored as JSON files under `DataDir()/sessions/`:

```
~/.buildmax/sessions/
  <uuid>.json          # individual session file
  sessions.json        # session list index
```

**Individual session file** (`<uuid>.json`):
```json
{
  "id": "uuid-string",
  "title": "first user message...",
  "created_at": "2026-01-15T10:30:00Z",
  "messages": [
    {"role": "user", "content": "..."},
    {"role": "assistant", "content": "...", "tool_calls": [...]},
    {"role": "tool", "content": "...", "tool_call_id": "..."}
  ]
}
```

**`PersistAfterReply` flow**:
1. `EnsureTitleFromFirstUserMessage()` — sets title from first user message if empty (truncated to maxLen runes).
2. `SaveToDir()` — serializes session to JSON, writes to `dir/<id>.json`.
3. `UpsertListEntry()` — updates the session list index.

### Session List Index

`sessions.json` is a flat JSON array of `SessionItem` objects for quick session lookup without reading each individual file:

- `agentapp.LoadSessionList(dir)` — reads the list; returns empty slice if file missing.
- `agentapp` session helpers update or append entries when sessions are saved.
- CLI `--continue` resolves the most recent entry by `created_at`.

### Context Helpers

- `CtxWithSessionID(ctx, id)` — stores session ID in context.
- `SessionIDFromContext(ctx)` — retrieves session ID from context. Used by tools that need the session ID.

### Loading a Session

- `LoadFromDir(dir, sessionID)` — reads `dir/<sessionID>.json`, parses JSON, returns a `Session` via `NewSessionFromData()`.
- `NewSessionFromData(id, title, createdAt, messages)` — constructs a Session from persisted data without exposing internal fields.

## Dependencies

- **Uses**: `internal/core/llm` (Message type for history), `github.com/google/uuid` (ID generation)
- **Used by**: `internal/agentapp`, `internal/interface/cli`, `internal/interface/desktop`, and worker task-run session restore

## Notes

- All JSON keys use `snake_case` per project convention.
- Session fields are unexported — access only through methods and constructors.
- `Messages()` returns a copy to prevent external mutation during the agent loop.
- See also: [Agent Loop](agent-loop.md), [CLI](cli.md), [TUI](tui.md).
