# Design 013 - Persist session list file for easy lookup

## Goal

Persist a session index file (`sessions.json`) under the sessions directory so sessions can be listed and the "last" session resolved without scanning the filesystem, and add a `--continue` flag to resume the most recent session by created_at.

## Modules

| Module (package) | Responsibility | Owns |
|------------------|----------------|------|
| **internal/session** | Session lifecycle, per-session JSON, and session list file (index). | `Session`, `sessionFile`, list types; `SaveToDir`, `LoadFromDir`; new: list load/upsert, title helper, last-by-created_at. |
| **cmd/buildmax** | CLI flags, prompt mode, TUI bootstrap; orchestration of save + list update and `--continue` resolution. | Root command, `runPromptMode`, `runTUI`; uses session package only (no new packages). |
| **internal/tui** | TUI model and save-after-reply; calls session save and (after this task) list upsert with workspace from opts. | `Model`, `TUIOpts`; single save call site in `agentDoneMsg` handler. |

## Structure

**Directory / files**

- `internal/session/`
  - `session.go` — existing: `Session`, `SaveToDir`, `LoadFromDir`, etc.
  - `list.go` — **new**: list file path constant, `ListEntry`, `LoadList`, `UpsertListEntry`, `LastByCreatedAt`; `EnsureTitleFromFirstUserMessage` can live here or in `session.go` (single place for "session + list" API).
- `cmd/buildmax/`
  - `root.go` — add `--continue` flag; resolve resume id from list when `--continue` and no `--resume`; in prompt mode and TUI path: ensure title, save session, upsert list entry.
- `internal/tui/`
  - `model.go` — in `agentDoneMsg` success path: ensure title, then save session, then upsert list entry using `opts.Workspace` and `opts.SessionsDir`.

**Main types and interfaces**

- **ListEntry** (internal/session): Struct for session list JSON. Fields: `ID`, `Title`, `Workspace`, `CreatedAt` (time.Time or string for RFC3339); tags `json:"id"`, `json:"title"`, `json:"workspace"`, `json:"created_at"`. Exported so cmd can construct it.
- **Session** (internal/session): Unchanged; callers use `ID()`, `Title()`, `CreatedAt()` to build `ListEntry`.
- **session list file**: `dir/sessions.json` (same `dir` as `dir/<id>.json`). JSON array of list entries; snake_case keys.

## Method design

| Receiver / Package | Method | Signature | Responsibility |
|--------------------|--------|-----------|-----------------|
| **session** | LoadList | `LoadList(dir string) ([]ListEntry, error)` | Read `dir/sessions.json`; if missing return `([]ListEntry, nil)`; if invalid JSON return error. |
| **session** | UpsertListEntry | `UpsertListEntry(dir string, entry ListEntry) error` | Load list, upsert by `entry.ID` (update title/workspace if exists, else append), write back; create dir if needed. |
| **session** | LastByCreatedAt | `LastByCreatedAt(entries []ListEntry) *ListEntry` | Return entry with latest `created_at`; nil if empty. |
| **session** | EnsureTitleFromFirstUserMessage | `EnsureTitleFromFirstUserMessage(s *Session, maxLen int)` | If `s.Title() == ""` and there is at least one user message (role `"user"`), set title to first such message's content truncated to `maxLen` runes. |
| **session** | (existing) SaveToDir | `SaveToDir(s *Session, dir string) error` | Unchanged. |
| **cmd** | runRoot | (existing + flag handling) | If `--continue` set and `--resume` empty: `LoadList(sessionsDir)`, `LastByCreatedAt(list)`; if nil, stderr + exit 1; else set resumeID = that entry's ID. Then existing resume/prompt/TUI logic. |
| **cmd** | runPromptMode | (modified) | Before save: `EnsureTitleFromFirstUserMessage(sess, 100)`. After `SaveToDir`: build `ListEntry{sess.ID(), sess.Title(), sess.CreatedAt(), cwd}`, `UpsertListEntry(sessionsDir, entry)`. |
| **tui** | Update (agentDoneMsg) | (modified) | On success: `EnsureTitleFromFirstUserMessage(m.opts.Session, 100)`; `SaveToDir(...)`; build ListEntry from session + `m.opts.Workspace`, `UpsertListEntry(m.opts.SessionsDir, entry)`. |

**ListEntry CreatedAt representation**: Use `time.Time` in Go; marshal/unmarshal as RFC3339 string in JSON (same as `sessionFile.CreatedAt`). So `ListEntry.CreatedAt` is `time.Time` with custom JSON or a string field; implementation can use a small helper or `json` tag with type that marshals to RFC3339. Simplest: store `CreatedAt string` in struct with `json:"created_at"` and have callers set `s.CreatedAt().Format(time.RFC3339)`; `LastByCreatedAt` parses each with `time.Parse(time.RFC3339, ...)` to compare. Alternatively one internal struct for JSON (created_at string) and an exported ListEntry with time.Time that converts at load/save boundary.

## How they work together

**Data/control flow**

1. **Resolve --continue**: Root reads `--continue` and `--resume`. If `--continue` and no `--resume`: `sessionsDir := filepath.Join(config.DataDir(), "sessions")`, `list, _ := session.LoadList(sessionsDir)`, `last := session.LastByCreatedAt(list)`; if last == nil, print to stderr and exit 1; else set resumeID = last.ID. Then pass resumeID into existing prompt or TUI path.
2. **Prompt mode save**: After `agent.Process` and before/after existing `SaveToDir`: call `session.EnsureTitleFromFirstUserMessage(sess, 100)`; then `SaveToDir(sess, sessionsDir)`; then `session.UpsertListEntry(sessionsDir, session.ListEntry{ID: sess.ID(), Title: sess.Title(), Workspace: cwd, CreatedAt: ...})` (CreatedAt from sess.CreatedAt() in the format chosen for ListEntry).
3. **TUI save**: On `agentDoneMsg` with no error: `EnsureTitleFromFirstUserMessage(m.opts.Session, 100)`; `SaveToDir(m.opts.Session, m.opts.SessionsDir)`; build ListEntry from session + `m.opts.Workspace`; `UpsertListEntry(m.opts.SessionsDir, entry)`.
4. **UpsertListEntry**: Load `dir/sessions.json` (or treat missing as []), find index of entry with same ID; if found replace that element's title and workspace (keep created_at); else append. Write back with `os.MkdirAll(dir, 0755)` and `os.WriteFile(dir/sessions.json, ...)`.

**Dependencies**

- `cmd/buildmax` and `internal/tui` depend on `internal/session` for list and title helpers; session has no new dependencies (still `encoding/json`, `os`, `path/filepath`, `time`, `internal/llm` only for Message in Session).
- Session package does not call SaveToDir from list logic; callers orchestrate save then upsert.

**Key data structures**

- **ListEntry**: Built by cmd/tui from `Session` (id, title, created_at) plus workspace string; passed to `UpsertListEntry`. Session package returns it from `LoadList` and `LastByCreatedAt`.
- **sessions.json**: Array of list entries; read in full and written in full on each upsert (no append-only stream).

## Changes for review

- **New**: `internal/session/list.go` — `ListEntry` struct; `listFileName` or constant `"sessions.json"`; `LoadList(dir) ([]ListEntry, error)`; `UpsertListEntry(dir, entry ListEntry) error`; `LastByCreatedAt(entries []ListEntry) *ListEntry`.
- **New**: `internal/session` — `EnsureTitleFromFirstUserMessage(s *Session, maxLen int)` (in `list.go` or `session.go`); scans `s.Messages()` for first `Role == "user"`, sets `s.SetTitle(truncate(content, maxLen))` using runes.
- **Modified**: `cmd/buildmax/root.go` — add `--continue` / `-c` flag; in RunE, if continue set and resume empty, resolve id from list and set resumeID, else exit with message; in `runPromptMode`: ensure title, save, then upsert list with `ListEntry{..., Workspace: cwd}`; update help text.
- **Modified**: `internal/tui/model.go` — in `agentDoneMsg` success branch: ensure title, save session, upsert list entry using `m.opts.Workspace` and `m.opts.SessionsDir`.
- **New**: `internal/session/list_test.go` (or extend `session_test.go`) — tests for LoadList (missing, invalid, round-trip), UpsertListEntry (create, update by id), LastByCreatedAt (empty, two entries), EnsureTitleFromFirstUserMessage (set from first user, truncate, no-op when title set or no user messages).
