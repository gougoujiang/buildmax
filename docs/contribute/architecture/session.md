# Session

> **Audience:** contributors · **Status:** current
>
> User-facing view: [guide/sessions-and-traces.md](../../guide/sessions-and-traces.md)
>
> Rationale and the full record contract:
> [design/local-session-storage.md](../../design/local-session-storage.md)

## Purpose

Local chat session state is split across three packages:

| Package | Owns |
|---|---|
| `internal/core/session` | Record types, validation, the linked-history reducer, recovery analysis, and the `Store` seam. No file I/O. |
| `internal/infra/sessionstore` | Physical durability: JSONL codec, atomic metadata, the single-writer lock, tail repair, salvage |
| `internal/agentapp` | `SessionManager` and `SessionContext`: when state commits, and lifecycle |

`SessionManager` is the entry point for anything that runs a session. Core holds
what the records mean, infra holds how they survive, `agentapp` holds when they
are written.

## On-disk shape

```text
<BUILDMAX_HOME>/sessions/
  index.json                    the picker projection, rebuildable
  <session_id>/
    meta.json                   current selections and running totals
    history.jsonl               the append-only conversation journal
    traces/<run_id>.jsonl       one file per run
    writer.lock                 who holds this session
```

Two records, two kinds of authority, and neither is a projection of the other:

- **`history.jsonl`** is authoritative for anything that reconstructs the
  conversation — messages, tool outcomes, compaction, durable state. It is
  append-only and lossless.
- **`meta.json`** is authoritative for current selections and running
  aggregates — title, pin, workspace, selected model, tokens, cost. Replay does
  not recover these, and no history record needs to. It also carries
  `project_id`, the local Project this session belongs to; unlike the fields
  beside it that is immutable, and `MetaUpdate` has no way to change it.
- **`index.json`** is the only pure projection, and the only file rebuilt by
  scanning.

Nothing lives in both. The current head is *derived*: it is the last record in
the journal, because `head_selected` chains to the item a rewind returns to, so
the parent links already express the branch.

## Records

Every journal line carries `seq` (physical order), `id`/`parent_id` (logical
order), a `type`, and `required` — whether a reader that cannot interpret the
type must refuse the session or may skip the record. That one bit is what lets
the format grow without older readers either mis-reducing a conversation or
refusing every session containing something new.

The type vocabulary and each payload are listed in
[design §6.3](../../design/local-session-storage.md); the two that matter most
here are `tool_execution_started` and `tool_result`. The first is written — and
synced — *before* a tool runs, which is the only way an interrupted run can tell
a call that never started from one that may already have changed the world.

## Commit path

`SessionContext` is the committing context. It has no exported fields: every
change a resumed turn would have to see goes through a method that reaches the
journal before returning, so a caller cannot change resumable state without
committing it.

| Change | Method | Lands in |
|---|---|---|
| A message | `Append` | history |
| A tool about to run | `ToolExecutionStarted` | history |
| A tool's outcome | `AppendToolResult` | history |
| Compaction boundary | `AddCompaction` | history |
| Notes / todos / additional prompt | `SetNotes`, `SetTodos`, `SetAdditionalPrompt` | history |
| Turn open / close | `BeginTurn`, `FinishTurn` | history |
| Title, workspace, model, usage | `SetTitle`, `SetWorkspace`, `SetModel`, `AddUsage` | metadata |

The agent loop reaches the first four through `MessageHistory` and its
optional extensions (`CompactionHistory`, `NotesHistory`,
`ToolBoundaryHistory`), so the loop itself stays free of storage.

## Lifecycle

| Operation | Function |
|---|---|
| Create, or create under an assigned id | `SessionManager.Create`, `CreateWithID` |
| Create a subagent's hidden bundle | `SessionManager.CreateSubagent` |
| Open for writing (takes the lock) | `SessionManager.Open` |
| Read without the lock | `SessionManager.Load` |
| List (picker projection) | `SessionManager.List` |
| Rename / pin | `SessionManager.Rename`, `SetPinned` |
| Delete one, or every session for a workspace | `SessionManager.Delete`, `DeleteByWorkspace` |
| Finish a turn — title, usage, metadata | `SessionManager.Finalize` |

`Finalize` writes metadata only. The conversation is already durable by the time
it runs, so a failure there loses reporting rather than the turn.

`DeleteByWorkspace` matches through `workspaceAliases`, because the same
directory can be recorded under different spellings (symlinks, `~` expansion,
trailing slashes). A session with no recorded workspace never matches.

## The writer lock

One writer per session, held for as long as the session is open rather than per
append — which is what stops two turns interleaving into one span. It is an OS
advisory lock (`flock` on unix, `LockFileEx` on Windows) on `writer.lock`, not
on the journal, so a reader can still inspect a stable prefix while a writer
holds the session. `Load` never takes it.

The file's contents are diagnostics. Ownership is the kernel's answer, because a
recorded PID cannot say whether that process is still alive, and a lock the
kernel holds is released when its owner exits however it exits.

Opening a session that another process holds returns `sessionstore.ErrLocked`.

## Recovery

`Open` repairs a torn final line, then — only if the branch still has calls left
uncertain by an interruption — appends one `turn_recovered` record and one
`unknown` tool result per uncertain call. The gate is "is anything still
uncertain", not "was the turn left open", so a session repaired once is not
repaired again.

`Writer.Loaded().Recovery` reports what *was* repaired, for a caller that wants
to tell the user. `Load` computes the same classification without writing
anything.

## Subagents

Every subagent run gets its own bundle with `kind: subagent`, hidden from the
picker and from `--continue`, recording which session, run, and tool call
delegated to it. Its traces are filed under the *parent's* session, because a
hidden bundle is not somewhere a person navigates to.

`internal/tool` declares `SubAgentSession` and `agentapp` supplies it, since
`tool` sits below `agentapp`. A runner with no factory falls back to an
in-memory session, so the run path has no branch in it.

## Session ID In Context

`CtxWithSessionID(ctx, id)` and `SessionIDFromContext(ctx)` carry the session id
through call stacks that should not take it as a parameter — tools and the trace
recorder read it from the context rather than having it threaded down.

## Dependencies

- `internal/core/session` **uses**: `internal/core/llm`, `internal/core/agent`
  (note and todo types, tool outcome statuses), `github.com/google/uuid`
- `internal/infra/sessionstore` **uses**: `internal/core/session`,
  `internal/util` (atomic file replacement), `golang.org/x/sys` (the lock)
- **Used by**: `internal/agentapp`, `internal/interface/cli`,
  `internal/interface/desktop`, and worker task-run session restore

## Notes

- All JSON keys are `snake_case`, per the repository convention.
- The system prompt is never stored in a session; it is built per run by
  `agentapp.BuildEffectiveSystemPrompt`. The *additional* system prompt is
  stored, because a resumed session would otherwise lose the identity it ran
  under.
- Session directories and files use private permissions (`0700`/`0600`).
- See also: [Agent Loop](agent-loop.md), [CLI](cli.md), [TUI](tui.md).
