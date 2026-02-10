# Session

## Purpose

The `internal/session` package manages chat session state: in-memory conversation history, JSON persistence to disk, and a session list index for quick lookup.

## Key Types and Interfaces

| Name | Kind | Role |
|------|------|------|
| **Session** | struct | In-memory session: id (UUID), title, created_at, message history |
| **ListEntry** | struct | Session metadata in the list index: id, title, workspace, created_at |
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

`sessions.json` is a flat JSON array of `ListEntry` objects for quick session lookup without reading each individual file:

- `LoadList(dir)` — reads the list; returns empty slice if file missing.
- `UpsertListEntry(dir, entry)` — loads list, updates or appends entry by ID, writes back.
- `LastByCreatedAt(entries)` — returns the most recent entry (for `--continue` flag).

### Context Helpers

- `CtxWithSessionID(ctx, id)` — stores session ID in context.
- `SessionIDFromContext(ctx)` — retrieves session ID from context. Used by tools that need the session ID.

### Loading a Session

- `LoadFromDir(dir, sessionID)` — reads `dir/<sessionID>.json`, parses JSON, returns a `Session` via `NewSessionFromData()`.
- `NewSessionFromData(id, title, createdAt, messages)` — constructs a Session from persisted data without exposing internal fields.

## Dependencies

- **Uses**: `internal/llm` (Message type for history), `github.com/google/uuid` (ID generation)
- **Used by**: `internal/agent` (reads/writes messages), `internal/tui` (passes to agent, persists after reply), `cmd/buildmax` (loads/creates sessions)

## Notes

- All JSON keys use `snake_case` per project convention.
- Session fields are unexported — access only through methods and constructors.
- `Messages()` returns a copy to prevent external mutation during the agent loop.
- See also: [Agent Loop](agent-loop.md), [CLI](cli.md), [TUI](tui.md).
