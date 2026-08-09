# Session

> **Audience:** contributors · **Status:** current
>
> User-facing view: [guide/sessions-and-traces.md](../../guide/sessions-and-traces.md)

## Purpose

Local chat session state is split across two packages:

| Package | Owns |
|---|---|
| `internal/core/session` | The `Session` and `SessionItem` types, plus pure helpers. No file I/O beyond the types themselves. |
| `internal/agentapp` (`session_manager.go`) | All persistence and lifecycle: create, load, save, list, rename, delete, pin, title generation |

`SessionManager` is the entry point for anything that runs a session. The core
package holds the shape; `agentapp` holds the behavior.

## Types

```go
// internal/core/session — JSON tags match the on-disk format (snake_case).
type Session struct {
    ID                string
    Title             string
    CreatedAt         time.Time
    Messages          []llm.Message
    PromptTokens      int
    CompletionTokens  int
    CompactionIdx     int     // index into Messages where the latest compaction boundary falls
    CompactionSummary string  // summary of everything before CompactionIdx
}

type SessionItem struct {   // one row in the sessions.json index
    ID, Title, Workspace string
    CreatedAt            string  // RFC3339
    Pinned               bool
}
```

Fields are **exported with explicit `json:"snake_case"` tags** — `Session` is
its own serialization format, not a wrapper around a private one.

`Session` implements the agent loop's `MessageHistory` through
`HistoryMessages()` and `Append(msg)`, and its `CompactionHistory` extension
through `AddCompaction(summary, n)`. That is the whole reason the type exists in
`core`: the pure loop can drive it without importing anything above it.

Note the method is `HistoryMessages()`, not `Messages()` — it returns the
messages the LLM should see, which after a compaction is not the whole slice.

## Persistence

```text
<BUILDMAX_HOME>/sessions/
  <id>.json          one session
  sessions.json      the index: []SessionItem
```

The index exists so the picker and `--continue` can list sessions without
opening every file. Every write path updates both.

| Operation | Function |
|---|---|
| Create / load in a run | `SessionManager.Create`, `SessionManager.Load` |
| Save after a turn | `SessionManager.Save` |
| Finish a turn — title, usage, save | `SessionManager.Finalize` |
| List | `SessionManager.List`, or `agentapp.LoadSessionList(dir)` |
| Load one by id | `agentapp.LoadSession(dir, id)` |
| Rename / delete / pin | `agentapp.RenameSession`, `DeleteSession`, `SetSessionPinned` |
| Delete every session for a workspace | `agentapp.DeleteSessionsByWorkspace` |
| Update one index row | `agentapp.UpsertSessionItem` |

`Finalize` is what a surface calls at the end of a turn: it generates a title
when the session has none, folds `RunStats` token counts into the session, and
persists both the file and the index row. Title generation is an LLM call, so
its usage is added to the session too — that is how a session's token count
stays complete rather than counting only the visible turns.

`DeleteSessionsByWorkspace` matches through `workspaceAliases`, because the same
directory can be recorded under different spellings (symlinks, `~` expansion,
trailing slashes).

## Session ID In Context

`CtxWithSessionID(ctx, id)` and `SessionIDFromContext(ctx)` carry the session id
through call stacks that should not take it as a parameter — tools and the trace
recorder read it from the context rather than having it threaded down.

## Dependencies

- `internal/core/session` **uses**: `internal/core/llm` (the `Message` type),
  `github.com/google/uuid`
- **Used by**: `internal/agentapp`, `internal/interface/cli`,
  `internal/interface/desktop`, and worker task-run session restore

## Notes

- All JSON keys are `snake_case`, per the repository convention.
- The system prompt is never stored in a session; it is built per run by
  `agentapp.BuildEffectiveSystemPrompt`.
- See also: [Agent Loop](agent-loop.md), [CLI](cli.md), [TUI](tui.md).
