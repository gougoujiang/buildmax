# Design 008 - Persist sessions to file system

## Goal

Save chat sessions to disk under the application data directory so they can be loaded and resumed across runs via a `--resume <session-id>` flag.

## Modules

| Module (package) | Responsibility | Owns |
|------------------|----------------|------|
| **internal/session** | In-memory session plus persistence: save/load to JSON files under a given directory. | `Session`, `NewSession`, `NewSessionFromData`; `sessionFile` (JSON DTO); `SaveToDir`, `LoadFromDir`; `session.go`, `session_test.go`. |
| **internal/config** | Application data directory; unchanged. | `DataDir()` — used by cmd to build sessions path. |
| **internal/llm** | Message types; unchanged. | `Message` (JSON-serializable). |
| **cmd/buildmax** | CLI: prompt mode creates or resumes session, runs agent, then saves session to disk. | `main.go` — `-resume` flag, sessions dir, runPromptMode(new \| resume) → save. |

## Structure

**Directory / files**

- `internal/session/` — session + persistence
  - `session.go` — add `sessionFile` (internal JSON type), `NewSessionFromData`, `SaveToDir`, `LoadFromDir`
  - `session_test.go` — tests for SaveToDir (file exists, valid JSON), LoadFromDir (after save, missing file, invalid JSON), round-trip
- `cmd/buildmax/` — CLI
  - `main.go` — add `-resume` flag; runPromptMode(prompt, resumeID): resolve sessionsDir, load or new session, Process, SaveToDir; update usage

**Main types and interfaces**

- **sessionFile** (internal/session): Package-internal struct for JSON only. Fields: `ID`, `Title`, `CreatedAt` (string RFC3339), `Messages` ([]llm.Message). Not exported; used only in SaveToDir/LoadFromDir.
- **Session** (internal/session): Unchanged shape; add constructor **NewSessionFromData** for reconstituting from persisted data.
- **config** (internal/config): Unchanged; cmd uses `config.DataDir()` to build sessions path.

## Method design

| Receiver | Method | Signature | Responsibility |
|----------|--------|-----------|-----------------|
| (package) | NewSessionFromData | `(id, title string, createdAt time.Time, messages []llm.Message) *Session` | Build a Session from persisted data (same id, title, created_at, copy of messages). Used by LoadFromDir. |
| (package) | SaveToDir | `(s *Session, dir string) error` | MkdirAll(dir); serialize s to sessionFile (CreatedAt as RFC3339); write `dir/<s.ID()>.json`. |
| (package) | LoadFromDir | `(dir string, sessionID string) (*Session, error)` | Read `dir/<sessionID>.json`; if missing return clear error (e.g. "session not found: <id>"); if invalid JSON return error; parse CreatedAt (RFC3339); return NewSessionFromData(...). |

## How they work together

**Data/control flow (new session: `buildmax -p PROMPT`)**

1. main: parse flags; no `-resume` → runPromptMode(prompt, "").
2. runPromptMode: sessionsDir = filepath.Join(config.DataDir(), "sessions"); sess = session.NewSession(""); reply, err = agent.Process(ctx, sess, prompt); on success session.SaveToDir(sess, sessionsDir); print reply.

**Data/control flow (resume: `buildmax --resume <id> -p PROMPT`)**

1. main: parse flags; if `-resume` set without `-p` → error and exit. Otherwise runPromptMode(prompt, resumeID).
2. runPromptMode: sessionsDir = filepath.Join(config.DataDir(), "sessions"); sess, err = session.LoadFromDir(sessionsDir, resumeID) — on error print to stderr and exit; reply, err = agent.Process(ctx, sess, prompt); on success session.SaveToDir(sess, sessionsDir); print reply.

**Dependencies**

- **internal/session** depends on **internal/llm** (Message). Does **not** depend on internal/config or cmd; callers pass `dir` so session stays agnostic to where sessions live.
- **cmd/buildmax** depends on **internal/config** (DataDir), **internal/session** (NewSession, LoadFromDir, SaveToDir), **internal/agent** (Process).

**Key data structures**

- **sessionFile**: Internal to session package; used only for JSON encode/decode. Created in SaveToDir from Session getters; consumed in LoadFromDir to build Session via NewSessionFromData.
- **Sessions directory**: Single root = config.DataDir() + "/sessions"; one file per session = `<session-id>.json`. Created on first save (SaveToDir does MkdirAll).

## Changes for review

- **New**: `sessionFile` (internal/session) — internal struct for JSON; fields ID, Title, CreatedAt (string), Messages.
- **New**: `NewSessionFromData(id, title string, createdAt time.Time, messages []llm.Message) *Session` (internal/session).
- **New**: `SaveToDir(s *Session, dir string) error` (internal/session) — MkdirAll(dir), write dir/<id>.json.
- **New**: `LoadFromDir(dir string, sessionID string) (*Session, error)` (internal/session) — read dir/<id>.json; missing or invalid → error.
- **New**: session tests — SaveToDir creates valid JSON file; LoadFromDir after save returns same data; LoadFromDir missing/invalid JSON returns error; round-trip Save → Load, Messages equal.
- **Modified**: `cmd/buildmax/main.go` — add `-resume` string flag; runPromptMode(prompt, resumeID): sessionsDir = config.DataDir()+"/sessions"; if resumeID load else NewSession; Process; SaveToDir; error if resume without -p or load/save fails; update usage for --resume and session storage.
- **Unchanged**: internal/agent, internal/config, internal/llm. No new external Go modules.
