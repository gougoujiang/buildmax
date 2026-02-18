# Design 058: Runtime task dir re-org

## Goal

Reorganize the runtime task directory so each task run has two subdirs under the task path: `buildmax/` (agent runtime: sessions, logs, settings) and `ws/` (materialized workspace files). The executor runs `buildmax -p` with `BUILDMAX_HOME=<taskDir>/buildmax` and working directory `<taskDir>/ws`. Session file lookup for `GET /api/sessions/{session_id}` must resolve executor-created sessions from the task’s buildmax dir, with fallback to the global sessions dir.

## Modules and structure

| Package / file | Responsibility |
|----------------|----------------|
| `internal/config/config.go` | Add `RuntimeTaskBuildmaxDir(workspaceID, taskID)`, `RuntimeTaskWSDir(workspaceID, taskID)`; keep `RuntimeWorkspaceDir` unchanged (task root). |
| `internal/config/config_test.go` | Add tests for the two new functions; keep existing `TestRuntimeWorkspaceDir` (still asserts task root path). |
| `internal/executor/paths.go` | Extend `WorkspacePaths` with `RuntimeTaskBuildmaxDir`, `RuntimeTaskWSDir`. |
| `internal/executor/executor.go` | Create buildmax and ws subdirs; materialize into ws; set `cmd.Dir` and `BUILDMAX_HOME` in env; write result to task dir root. |
| `internal/executor/executor_test.go` | Implement new interface methods on `testWorkspacePaths`; update tests that depend on runtime layout or env (if any). |
| `internal/cmd/server.go` | Implement `RuntimeTaskBuildmaxDir` and `RuntimeTaskWSDir` on `defaultWorkspacePaths` by delegating to config. |
| `internal/server/session.go` | Resolve session file path: try task buildmax dir first, then fall back to global `SessionsDir`. |

No new packages. Existing types and callers unchanged except where listed.

## Method / function design

### config.RuntimeTaskBuildmaxDir

- **Signature**: `func RuntimeTaskBuildmaxDir(workspaceID, taskID string) string`
- **Behaviour**: Return `filepath.Join(RuntimeWorkspaceDir(workspaceID, taskID), "buildmax")`.
- **Comment**: "RuntimeTaskBuildmaxDir returns the buildmax subdir for a task run (agent data: sessions, logs, settings). It is RuntimeWorkspaceDir(workspaceID, taskID)/buildmax."

### config.RuntimeTaskWSDir

- **Signature**: `func RuntimeTaskWSDir(workspaceID, taskID string) string`
- **Behaviour**: Return `filepath.Join(RuntimeWorkspaceDir(workspaceID, taskID), "ws")`.
- **Comment**: "RuntimeTaskWSDir returns the workspace subdir for a task run (materialized persist files). It is RuntimeWorkspaceDir(workspaceID, taskID)/ws."

### WorkspacePaths (executor)

- **Add to interface** (in `paths.go`):
  - `RuntimeTaskBuildmaxDir(workspaceID, taskID string) string`
  - `RuntimeTaskWSDir(workspaceID, taskID string) string`

### defaultWorkspacePaths (cmd/server.go)

- **Add methods**:
  - `RuntimeTaskBuildmaxDir(workspaceID, taskID string) string` → `return config.RuntimeTaskBuildmaxDir(workspaceID, taskID)`
  - `RuntimeTaskWSDir(workspaceID, taskID string) string` → `return config.RuntimeTaskWSDir(workspaceID, taskID)`

### testWorkspacePaths (executor_test.go)

- **Add methods**: Same signatures; delegate to `config.RuntimeTaskBuildmaxDir` and `config.RuntimeTaskWSDir`.

### executor.executeTask

- **Current flow**: Create runtimeDir, materialize into runtimeDir, run buildmax with `cmd.Dir = runtimeDir`, write result to `runtimeDir/result-<taskID>.md`.
- **New flow**:
  1. `taskDir := r.paths.RuntimeWorkspaceDir(task.WorkspaceID, task.TaskID)` (unchanged).
  2. `buildmaxDir := r.paths.RuntimeTaskBuildmaxDir(task.WorkspaceID, task.TaskID)`; `wsDir := r.paths.RuntimeTaskWSDir(task.WorkspaceID, task.TaskID)`.
  3. `os.MkdirAll(taskDir, 0755)` then `os.MkdirAll(buildmaxDir, 0755)` and `os.MkdirAll(wsDir, 0755)` (or `MkdirAll(wsDir, 0755)` only; buildmaxDir is under taskDir so one `MkdirAll(wsDir, 0755)` creates taskDir and ws; then `MkdirAll(buildmaxDir, 0755)` creates buildmax — or create taskDir, then buildmax and ws explicitly for clarity).
  4. `r.persist.MaterializeToDir(ctx, task.WorkspaceID, wsDir)` (was runtimeDir).
  5. Build `cmd` with `cmd.Dir = wsDir`. Set `cmd.Env`: copy process env and set `BUILDMAX_HOME=<buildmaxDir>`. When `cmd.Env` is set, the child gets only that slice (no automatic inheritance). So: start from `os.Environ()`, remove any existing `BUILDMAX_HOME=` entry, append `BUILDMAX_HOME=`+buildmaxDir. Use `config.EnvKeyBuildmaxHome` for the var name.
  6. `resultPath := filepath.Join(taskDir, resultFilename)` (unchanged logically; result stays at task root).
  7. Artifact creation unchanged (still uses task.WorkspaceID, task.TaskID, output, resultFilename).

**Env building**: Add a small helper or inline: `env := os.Environ(); filtered := make([]string, 0, len(env)+1); prefix := config.EnvKeyBuildmaxHome + "="; for _, e := range env { if !strings.HasPrefix(e, prefix) { filtered = append(filtered, e) } }; cmd.Env = append(filtered, prefix + buildmaxDir)`. Executor must import `buildmax/internal/config` for `EnvKeyBuildmaxHome` and `strings` for `HasPrefix`.

### server.getSessionHandler

- **Current**: After ownership check, `path := filepath.Join(s.cfg.SessionsDir, sessionID+".json")` and `os.ReadFile(path)`.
- **New**: After ownership check, compute task-based path: `taskSessionPath := filepath.Join(config.RuntimeTaskBuildmaxDir(task.WorkspaceID, task.TaskID), "sessions", sessionID+".json")`. If `os.Stat(taskSessionPath)` or equivalent indicates the file exists, read from `taskSessionPath`. Otherwise fall back to `path := filepath.Join(s.cfg.SessionsDir, sessionID+".json")` and read from that. Use a single read path: e.g. `data, err := os.ReadFile(taskSessionPath); if err != nil && os.IsNotExist(err) { data, err = os.ReadFile(filepath.Join(s.cfg.SessionsDir, sessionID+".json")) }; if err != nil { ... }`. So: try task path first; on not-exist try global; on any other error (or both missing) return error/404 as today. Handler must import `buildmax/internal/config`.
- **SessionsDir empty**: Keep existing check; if `SessionsDir == ""`, we can still try task path (task buildmax dir is independent of global SessionsDir). So only return "sessions not configured" when SessionsDir is empty and we would need it for fallback — i.e. try task path first; if not found there and SessionsDir is empty, then return 404 or "session file not found". Simplest: try task path first; if not found, try global path (and if SessionsDir is "" that join is still valid, just will not exist); then if both fail return 404. So no change to the "SessionsDir == ''" check at the start — we still require SessionsDir for backward compatibility for now; if we want to allow session read when only task path exists, we could relax the check. Task spec says "fall back to existing global SessionsDir path". So: if task path exists use it; else use global path. If both missing → 404. The early check `if s.cfg.SessionsDir == ""` could prevent fallback; for executor-only sessions we don't need global. So: try task path first; if not exist, try global (only if SessionsDir != "" to avoid weird path); if still not exist or error, return 404 / error. Optionally: remove the early "SessionsDir not configured" check and only use it when falling back (if SessionsDir is "" and task path doesn't exist, 404). Keeping the early check is safer: if SessionsDir is not set we might have no sessions at all (pre-reorg). So leave the early check: when SessionsDir is "" return 503. Then in the read logic: try task path first; if os.ReadFile(taskSessionPath) succeeds, use it; else if os.IsNotExist(err), try global path and use that result; else return the error from task path.

## How they work together

1. **Executor**: On each task run, creates `<taskDir>/buildmax` and `<taskDir>/ws`, materializes persist into `ws`, runs `buildmax -p` with `BUILDMAX_HOME=<taskDir>/buildmax` and `cmd.Dir=<taskDir>/ws`. The agent process therefore writes sessions to `<taskDir>/buildmax/sessions/<session_id>.json` and uses `ws` as CWD and tool root. Result file stays at `<taskDir>/result-<taskID>.md`; artifact storage and DB unchanged.
2. **Session API**: When a client requests `GET /api/sessions/{session_id}`, the server finds the task by session_id and checks ownership. It then tries to read the session file from `<taskDir>/buildmax/sessions/<session_id>.json`; if that file does not exist (e.g. pre-reorg or non-executor session), it reads from the global `SessionsDir` instead. So executor-created sessions are found under the task’s buildmax dir; others still work from the global dir.
3. **Config and paths**: All path construction goes through config functions; WorkspacePaths implementations delegate to config so that executor and server stay testable and consistent.

## Changes for review

| Change | File | Detail |
|--------|------|--------|
| Edit | `internal/config/config.go` | Add `RuntimeTaskBuildmaxDir(workspaceID, taskID string) string` and `RuntimeTaskWSDir(workspaceID, taskID string) string` (each joins RuntimeWorkspaceDir with "buildmax" or "ws"). Add doc comments. |
| Edit | `internal/config/config_test.go` | Add `TestRuntimeTaskBuildmaxDir` and `TestRuntimeTaskWSDir` (set WorkspacesDir env, assert joined path ends with `/buildmax` or `/ws`). |
| Edit | `internal/executor/paths.go` | Add `RuntimeTaskBuildmaxDir(workspaceID, taskID string) string` and `RuntimeTaskWSDir(workspaceID, taskID string) string` to WorkspacePaths interface. |
| Edit | `internal/executor/executor.go` | In executeTask: obtain buildmaxDir and wsDir from paths; create both subdirs; MaterializeToDir into wsDir; set cmd.Dir = wsDir; build cmd.Env from os.Environ() with BUILDMAX_HOME=buildmaxDir (filter existing BUILDMAX_HOME); write result to taskDir. Import config (EnvKeyBuildmaxHome) and strings. |
| Edit | `internal/executor/executor_test.go` | Implement RuntimeTaskBuildmaxDir and RuntimeTaskWSDir on testWorkspacePaths (delegate to config). Add or adjust tests that verify subdir creation and env if needed. |
| Edit | `internal/cmd/server.go` | Add RuntimeTaskBuildmaxDir and RuntimeTaskWSDir methods to defaultWorkspacePaths delegating to config. |
| Edit | `internal/server/session.go` | Import config. After ownership check, set taskSessionPath := filepath.Join(config.RuntimeTaskBuildmaxDir(task.WorkspaceID, task.TaskID), "sessions", sessionID+".json"). Try os.ReadFile(taskSessionPath); if err is nil use it; else if os.IsNotExist(err) try filepath.Join(s.cfg.SessionsDir, sessionID+".json"). On any remaining error or both missing return 404/500 as appropriate. |
