# Design 041: Add --session-id flag for conversation

## Goal

Add a CLI flag `--session-id <uuid>` so the user can specify which session ID to use. The value must be a valid UUID; invalid values produce a clear error. When set, that ID is used for the run: load the session if it exists on disk, otherwise create a new session with that ID. When not set, behavior is unchanged (resume via `-r`/`-c`, new session with random UUID).

## Modules

| Module (package) | Responsibility | Changes |
|------------------|----------------|--------|
| **internal/cmd** | Root command, flags, session resolution, run dispatch | Add `--session-id` flag; resolve effective session ID (session-id takes precedence over -r/-c); validate UUID; pass effective ID to setupAgentAndSession. Update setup to support "load or create" when ID is provided. |
| **internal/session** | Session load/save, identity | Export a sentinel error for "session not found" so callers can distinguish "missing file" from other load failures. |

## Structure

**internal/cmd/root.go**

- Add flag: `root.Flags().String("session-id", "", "use a specific session ID (load if exists, else create); must be a valid UUID")`.
- In **runRoot**: read `sessionID, _ := cmd.Flags().GetString("session-id")`. If `sessionID != ""`, validate with `uuid.Parse(sessionID)` (from `github.com/google/uuid`); on parse error, print to stderr e.g. `invalid session-id: not a valid UUID` and return a non-nil error (e.g. `fmt.Errorf("invalid session-id: %w", err)`).
- Compute **effective session ID**: if `sessionID != ""` then effective = sessionID (already validated); else effective = result of `resolveResumeID(resumeID, cont)` (unchanged).
- Pass effective to `runPrintMode(prompt, effective, model)` or `runTUI(effective, model)`.
- Update **rootLong**: under Sessions, add one line: "Use --session-id <uuid> to use a specific session ID (load if exists, else create); value must be a valid UUID."

**internal/cmd/setup.go**

- **setupAgentAndSession(sessionID string, modelSelector string)** — rename parameter from `resumeID` to `sessionID` for clarity (call sites pass "effective session ID").
- Session load/create logic:
  - If `sessionID == ""`: create new session with `session.NewSession("")` (unchanged).
  - If `sessionID != ""`: call `session.LoadFromDir(sessionsDir, sessionID)`. If the error is **session not found** (see session package below), create a new session with that ID: `session.NewSessionFromData(sessionID, "", time.Now(), nil)` and use it. For any other error (e.g. invalid JSON, bad created_at), return the error as today.
- Logging: when loading, keep "resumed session"; when creating after not-found, log e.g. "created session with id" for the new-session path.

**internal/session/session.go**

- Define sentinel: `var ErrSessionNotFound = errors.New("session not found")` (exported).
- In **LoadFromDir**, when the file is missing (`os.IsNotExist(err)`), return an error that wraps the sentinel so callers can use `errors.Is(err, session.ErrSessionNotFound)`. For example: `return nil, fmt.Errorf("%w: %s", ErrSessionNotFound, sessionID)`.
- Keep the rest of LoadFromDir unchanged (other errors remain as today).

## Method / function design

| Location | Name | Signature / change | Responsibility |
|----------|------|--------------------|----------------|
| **cmd (root)** | runRoot | (unchanged signature) | Read --session-id; if set, validate UUID and use as effective session ID; else resolve via resolveResumeID; pass effective to runPrintMode/runTUI. |
| **cmd (root)** | NewRootCommand | (unchanged) | Register new flag `--session-id` and add line to rootLong. |
| **cmd (setup)** | setupAgentAndSession | `(sessionID string, modelSelector string)` | When sessionID != "": try LoadFromDir; if errors.Is(err, session.ErrSessionNotFound) then NewSessionFromData(sessionID, "", time.Now(), nil); else return err. When sessionID == "": NewSession("") as today. |
| **session** | LoadFromDir | (unchanged signature) | On file missing, return error wrapping ErrSessionNotFound. Other behavior unchanged. |
| **session** | (new export) | `var ErrSessionNotFound = errors.New("session not found")` | Sentinel for "session file does not exist"; enables errors.Is in callers. |

## How they work together

**Flow when --session-id is set**

1. User runs `buildmax --session-id <uuid>` or `buildmax --session-id <uuid> -p "query"`.
2. runRoot reads --session-id; validates with uuid.Parse; on invalid, prints error to stderr and returns error (exit non-zero).
3. effectiveSessionID = sessionID (--session-id wins; -r/-c ignored). runRoot calls runPrintMode(..., effectiveSessionID, ...) or runTUI(effectiveSessionID, ...).
4. setupAgentAndSession(sessionID, modelSelector): sessionID non-empty → LoadFromDir(sessionsDir, sessionID). If err != nil and errors.Is(err, session.ErrSessionNotFound) → sess = session.NewSessionFromData(sessionID, "", time.Now(), nil). If err != nil and not ErrSessionNotFound → return err. If err == nil → use loaded sess. Then TUI or print mode runs with that session.

**Flow when --session-id is not set**

1. effectiveSessionID = resolveResumeID(resumeID, cont) (unchanged).
2. setupAgentAndSession(effectiveSessionID, ...): if empty → NewSession(""); if non-empty → LoadFromDir only (current behavior: fail if not found). No change to this branch.

## Data flow (effective session ID)

- **Inputs**: --session-id (optional), -r/--resume (optional), -c/--continue (optional).
- **Rule**: If --session-id is non-empty (after trim/parse), effective = validated UUID; else effective = resolveResumeID(resumeID, cont).
- **Output**: effective is passed to setupAgentAndSession. In setup: empty → new random session; non-empty → load or create (only when LoadFromDir returns ErrSessionNotFound).

## Testing

- **internal/cmd (root)**  
  - Test that when --session-id is an invalid string (e.g. "not-a-uuid", "x"), the root command returns an error and the error message indicates invalid session-id (e.g. contains "invalid session-id" or "not a valid UUID"). Use Execute() or equivalent and assert stderr/error. No need to start TUI or agent.
- **internal/session**  
  - Existing test that expects "session not found: nonexistent-id" should be updated: assert that `errors.Is(err, ErrSessionNotFound)` is true when LoadFromDir is called for a missing session ID. Keep or adjust message check as needed.
- **internal/cmd (setup)**  
  - Optional: test that setupAgentAndSession with a non-empty sessionID that does not exist on disk returns a session with that ID (NewSessionFromData path). Can be done via a temp sessions dir and asserting sess.ID() == sessionID.

## Changes for review

- **Modified** `internal/cmd/root.go` — Add --session-id flag; in runRoot read and validate UUID; compute effective session ID (session-id overrides -r/-c); pass effective to runPrintMode/runTUI; add one line to rootLong for --session-id.
- **Modified** `internal/cmd/setup.go` — Parameter name resumeID → sessionID; when sessionID != "" try LoadFromDir and on session.ErrSessionNotFound create NewSessionFromData(sessionID, "", time.Now(), nil); otherwise keep current load/new behavior.
- **Modified** `internal/session/session.go` — Add `var ErrSessionNotFound = errors.New("session not found")`; in LoadFromDir when file is missing return `fmt.Errorf("%w: %s", ErrSessionNotFound, sessionID)`.
- **Modified** `internal/session/session_test.go` — When testing LoadFromDir for missing session, assert `errors.Is(err, ErrSessionNotFound)` (and optionally still check message).
- **New or modified** `internal/cmd` test — Add test for root command with --session-id invalid (e.g. "not-a-uuid") that expects non-nil error and message containing "invalid session-id" or similar.
