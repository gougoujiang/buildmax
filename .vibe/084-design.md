# Design 084: Metering (LLM token usage per chat run)

## Goal

Record LLM token usage (prompt and completion tokens) per agent run and persist it on chat runs: capture from API (non-stream and stream when available), expose via agent RunStats, store on `chat_run`, and have the worker send usage on success PATCH. CLI writes usage to run global when running under the worker so the worker can include it in the PATCH.

## Modules


| Module                      | Responsibility                   | Owns                                                                                                                                               |
| --------------------------- | -------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------- |
| **internal/llm**            | LLM client and usage extraction  | `Usage` type; `ChatWithTools` / `ChatWithToolsStream` return usage; map from go-openai response and stream chunks.                                 |
| **internal/agent**          | Agent loop and run stats         | `RunStats` with token counts; accumulate usage per iteration; `LLMCaller` interface extended for usage.                                            |
| **internal/model**          | Domain models                    | `ChatRun` optional `PromptTokens`, `CompletionTokens` (snake_case JSON/DB).                                                                        |
| **internal/storage/entity** | Chat run persistence             | `UpdateChatRunStatus` accepts optional usage; write to `chat_run` columns.                                                                         |
| **internal/workerapi**      | Worker HTTP contract             | `PatchChatRunRequest` optional `prompt_tokens`, `completion_tokens`.                                                                               |
| **internal/server**         | Worker PATCH handler             | Pass `req.PromptTokens`, `req.CompletionTokens` into `UpdateChatRunStatus`.                                                                        |
| **internal/executor**       | Run execution and success report | Read usage from run global `usage.json`; add usage to `RunResult`; include in success PATCH. Usage file helpers: `ReadRunUsage`, `WriteUsageFile`. |
| **internal/session**        | Session and persist format      | Session holds accumulated usage; `AddUsage(prompt, completion int)`; session JSON includes optional `prompt_tokens`, `completion_tokens`; SaveToDir/LoadFromDir handle them (old files → 0,0). |
| **internal/cmd**            | CLI print mode                   | After `Process`, add stats to session via `AddUsage`, then persist; if stats have token counts, call `executor.WriteUsageFile` for worker path. |
| **internal/tui**            | TUI after assistant reply        | After agent reply, call `session.AddUsage(stats.PromptTokens, stats.CompletionTokens)` before `PersistAfterReply` so session file has cumulative usage. |


## Structure

**LLM**

- `internal/llm/types.go` — Add `Usage` struct: `PromptTokens`, `CompletionTokens`, `TotalTokens int` (JSON snake_case if ever serialized).
- `internal/llm/client.go` — `ChatWithTools`: read `resp.Usage` from go-openai `ChatCompletionResponse`, map to `llm.Usage`; change signature to return `(content string, toolCalls []ToolCall, usage Usage, err error)`. `ChatWithToolsStream`: in the `for { resp, err := stream.Recv(); ... }` loop, after processing choices, check for usage—if go-openai `ChatCompletionStreamResponse` has a `Usage` field (or we introduce a custom stream decoder that parses chunks into a struct that includes `Usage`), set/accumulate it; return same signature `(content, toolCalls, usage, err)`. If the SDK does not expose usage on stream chunks, return zero usage for streaming (document in code).

**Agent**

- `internal/agent/agent.go` — `RunStats`: add `PromptTokens`, `CompletionTokens int`. `LLMCaller` interface: both methods return `(content string, toolCalls []llm.ToolCall, usage llm.Usage, err error)`. In `processLoop`: declare `var totalPrompt, totalCompletion int`; each iteration after LLM call, add `usage.PromptTokens` and `usage.CompletionTokens` to totals; when returning, set `RunStats{PromptTokens: totalPrompt, CompletionTokens: totalCompletion, ToolCalls: totalToolCalls}`. All call sites of `Process` / `ProcessAfterUserAppended` already receive `RunStats`; no change except stats now include token counts.

**Model**

- `internal/model/models.go` — On `ChatRun`, add `PromptTokens *int` and `CompletionTokens *int` with `gorm` and `json:"prompt_tokens"` / `json:"completion_tokens"`. Nullable so existing rows and runs without usage remain valid.

**Entity (store)**

- `internal/storage/entity/interfaces.go` — Extend `UpdateChatRunStatus` to accept two more optional params: `promptTokens, completionTokens *int`. Signature: `UpdateChatRunStatus(ctx, chatRunID, status string, startedAt, endedAt *int64, output, errorMessage, sessionID *string, promptTokens, completionTokens *int) error`. `UpdateChatRunStatusIf` unchanged (used only for RUNNING transition; no usage).
- `internal/storage/entity/chat_run.go` — In `UpdateChatRunStatus`, add to `updates` map: if `promptTokens != nil` then `updates["prompt_tokens"] = *promptTokens`; if `completion_tokens != nil` then `updates["completion_tokens"] = *completionTokens`. DB columns: `prompt_tokens`, `completion_tokens` (int, nullable). Migration: add columns (GORM AutoMigrate or explicit migration per project practice).

**Worker API**

- `internal/workerapi/types.go` — Add to `PatchChatRunRequest`: `PromptTokens *int` and `CompletionTokens *int` with `json:"prompt_tokens,omitempty"` and `json:"completion_tokens,omitempty"`.

**Server**

- `internal/server/worker_handlers.go` — In `patchWorkerChatRunHandler`, when calling `UpdateChatRunStatus` for non-RUNNING status, pass `req.PromptTokens` and `req.CompletionTokens` as the new arguments. No change for RUNNING branch (still uses `UpdateChatRunStatusIf` without usage).

**Executor**

- **Usage file contract**: Well-known file `usage.json` in the run global dir. Content: `{"prompt_tokens": N, "completion_tokens": N}` (snake_case). Written by CLI (when run by worker, `BUILDMAX_HOME` is run global). Read by executor before success PATCH.
- `internal/executor/usage.go` (new) — Constants: `UsageFilename = "usage.json"`. `ReadRunUsage(globalDir string) (promptTokens, completionTokens *int, err error)`: read `filepath.Join(globalDir, UsageFilename)`, decode JSON into a struct with `PromptTokens`, `CompletionTokens int` (json tags); return pointers to values; if file missing or invalid, return `(nil, nil, nil)` (no error). `WriteUsageFile(dir string, promptTokens, completionTokens int) error`: write the same JSON to `dir/UsageFilename`; only call when both values are known (caller ensures).
- `internal/executor/executor.go` — `RunResult`: add `PromptTokens`, `CompletionTokens *int`. After `runBuildmaxCmd` and before `reportRunSuccess`, call `ReadRunUsage(runGlobal)`; if both pointers non-nil and non-negative, set `result.PromptTokens`, `result.CompletionTokens`. In `reportRunSuccess`, when building `PatchChatRunRequest`, set `PromptTokens: result.PromptTokens`, `CompletionTokens: result.CompletionTokens` if present.
- `internal/executor/executor_test.go` — Update `mockChatRunStore.UpdateChatRunStatus` signature to accept `promptTokens, completionTokens *int`. Any test that calls `UpdateRunStatus` with success payload can assert usage when provided.

**Session**

- **Persisted format**: Session JSON (`sessionFile`) gains optional `prompt_tokens` and `completion_tokens` (int, snake_case). When both are 0 we can omit for backward compatibility, or always write them (design choice: always write so format is consistent; on load, missing fields → 0).
- `internal/session/session.go` — **sessionFile**: add `PromptTokens int \`json:"prompt_tokens,omitempty"\`` and `CompletionTokens int \`json:"completion_tokens,omitempty"\``. **Session** (internal state): add `promptTokens`, `completionTokens int`. **NewSession**: set both to 0. **NewSessionFromData**: add params `promptTokens, completionTokens int`; store in Session. **AddUsage(prompt, completion int)**: add to session’s running totals. **PromptTokens()**, **CompletionTokens()**: getters so SaveToDir can read. **SaveToDir**: set sessionFile.PromptTokens, sessionFile.CompletionTokens from session. **LoadFromDir**: decode into sessionFile; if fields are missing (old file), use 0; call NewSessionFromData(..., f.PromptTokens, f.CompletionTokens).
- **Call sites**: (1) `internal/cmd/print.go` — after `Process`, call `res.Session.AddUsage(stats.PromptTokens, stats.CompletionTokens)` then `session.PersistAfterReply(...)`; then, if stats have tokens, `executor.WriteUsageFile(config.DataDir(), ...)`. (2) `internal/tui/model.go` — wherever we have just received `RunStats` and are about to call `PersistAfterReply`, call `session.AddUsage(stats.PromptTokens, stats.CompletionTokens)` first (both places that call PersistAfterReply after an assistant reply must receive stats and add them).

**CLI (cmd)**

- `internal/cmd/print.go` — After `Process` returns: call `res.Session.AddUsage(stats.PromptTokens, stats.CompletionTokens)`; then persist (e.g. `session.PersistAfterReply`). If `stats.PromptTokens > 0 || stats.CompletionTokens > 0`, also call `executor.WriteUsageFile(config.DataDir(), stats.PromptTokens, stats.CompletionTokens)` for the worker path. Ignore write errors for the usage file (log and continue).

**TUI**

- `internal/tui/model.go` — After the agent returns a reply (both code paths that call `PersistAfterReply`), we have access to the run’s `RunStats`. Call `session.AddUsage(stats.PromptTokens, stats.CompletionTokens)` before `session.PersistAfterReply(...)` so the session file gets the updated cumulative usage.

**Tests**

- `internal/llm/client_test.go` (or new test file) — Test that when a mocked completion response includes `Usage`, `ChatWithTools` returns it; when `Usage` is zero/empty, return zero usage. Mock the HTTP client or use an interface for the create-completion call so we don't call the real API.
- `internal/agent/agent_test.go` — Test that when the mock LLMCaller returns usage in multiple iterations, `RunStats` accumulates prompt and completion tokens.
- `internal/executor/usage_test.go` — Test `ReadRunUsage`: missing file → (nil, nil, nil); invalid JSON → (nil, nil, nil) or error per implementation; valid JSON → correct pointers. Test `WriteUsageFile`: writes valid JSON; `ReadRunUsage` after write returns same values.
- `internal/session/session_test.go` — SaveToDir/LoadFromDir round-trip with usage (non-zero prompt_tokens, completion_tokens); LoadFromDir on file without usage fields returns session with 0,0; AddUsage then SaveToDir then LoadFromDir yields session with accumulated totals.
- `internal/server/worker_handlers_test.go` (or storage test) — Optional: PATCH with `prompt_tokens` and `completion_tokens` results in run row having those values (requires DB or store mock that records the call).

## Method and signature design


| Location                | Method / Type             | Signature / Shape                                                                                                                                | Responsibility                                                                                                                     |
| ----------------------- | ------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------ | ---------------------------------------------------------------------------------------------------------------------------------- |
| **llm**                 | Usage                     | `PromptTokens, CompletionTokens, TotalTokens int`                                                                                                | Hold token counts from API.                                                                                                        |
| **llm.Client**          | ChatWithTools             | `(ctx, messages, tools) (content string, toolCalls []ToolCall, usage Usage, err error)`                                                          | Return content, toolCalls, and usage from `resp.Usage`.                                                                            |
| **llm.Client**          | ChatWithToolsStream       | `(ctx, messages, tools, onDelta) (content string, toolCalls []ToolCall, usage Usage, err error)`                                                 | Same; usage from last non-zero chunk when SDK provides it, else zero.                                                              |
| **agent**               | LLMCaller (interface)     | Both methods add `usage Usage` as third return value.                                                                                            | Callers (agent) get usage per call.                                                                                                |
| **agent**               | RunStats                  | Add `PromptTokens int`, `CompletionTokens int`                                                                                                   | Accumulated token counts for the run.                                                                                              |
| **agent**               | processLoop               | —                                                                                                                                                | After each LLM call, add returned usage to running totals; set stats on return.                                                    |
| **model**               | ChatRun                   | Add `PromptTokens *int`, `CompletionTokens *int` (gorm + json snake_case)                                                                        | Optional DB columns.                                                                                                               |
| **entity.ChatRunStore** | UpdateChatRunStatus       | `(ctx, chatRunID, status string, startedAt, endedAt *int64, output, errorMessage, sessionID *string, promptTokens, completionTokens *int) error` | Persist optional usage on the run row.                                                                                             |
| **entity.Store**        | UpdateChatRunStatus       | Same                                                                                                                                             | Build updates map including prompt_tokens, completion_tokens when non-nil.                                                         |
| **workerapi**           | PatchChatRunRequest       | Add `PromptTokens *int`, `CompletionTokens *int` (json prompt_tokens, completion_tokens)                                                         | Worker can send usage on success.                                                                                                  |
| **server**              | patchWorkerChatRunHandler | —                                                                                                                                                | Pass `req.PromptTokens`, `req.CompletionTokens` into `UpdateChatRunStatus`.                                                        |
| **executor**            | ReadRunUsage              | `(globalDir string) (promptTokens, completionTokens *int, err error)`                                                                            | Read usage.json from run global; return nil,nil when missing/invalid.                                                              |
| **executor**            | WriteUsageFile            | `(dir string, promptTokens, completionTokens int) error`                                                                                         | Write usage.json under dir.                                                                                                        |
| **executor**            | RunResult                 | Add `PromptTokens`, `CompletionTokens *int`                                                                                                      | Carry usage from run global to PATCH.                                                                                              |
| **executor**            | reportRunSuccess          | —                                                                                                                                                | Set `req.PromptTokens`, `req.CompletionTokens` from `result` when present.                                                         |
| **session**             | Session (struct)          | Add `promptTokens`, `completionTokens int`; getters `PromptTokens()`, `CompletionTokens()`                                                                 | Hold accumulated usage for the session.                                                                                            |
| **session**             | AddUsage                  | `(prompt, completion int)`                                                                                                                                 | Add this turn’s usage to session totals.                                                                                            |
| **session**             | sessionFile               | Add `PromptTokens`, `CompletionTokens int` (json prompt_tokens, completion_tokens, omitempty)                                                              | Persisted session JSON.                                                                                                            |
| **session**             | NewSessionFromData        | Add params `promptTokens, completionTokens int`                                                                                                                                 | Restore session including accumulated usage.                                                                                      |
| **session**             | SaveToDir / LoadFromDir   | —                                                                                                                                                | Include usage in sessionFile; on load, missing fields → 0.                                                                       |
| **cmd**                 | runPrintMode              | —                                                                                                                                                | After Process: `session.AddUsage(stats...)` then persist; then `executor.WriteUsageFile` if stats have tokens.                    |
| **tui**                 | (model after reply)       | —                                                                                                                                                | Before `PersistAfterReply`, call `session.AddUsage(stats.PromptTokens, stats.CompletionTokens)`.                                 |


## How they work together

**Data flow**

1. **LLM call (non-stream)**
  Client calls API; response has `Usage`; we map to `llm.Usage` and return with content and toolCalls.
2. **LLM call (stream)**
  We iterate stream chunks; when a chunk has usage (if SDK exposes it or we use a custom decoder), we keep the last non-zero usage; return it with content and toolCalls.
3. **Agent loop**
  Each iteration gets `(content, toolCalls, usage, err)`. On success, add `usage.PromptTokens` and `usage.CompletionTokens` to running totals. When the loop exits (final reply or max iter), return `RunStats{ToolCalls, PromptTokens, CompletionTokens}`.
4. **CLI print mode (worker context)**
  Worker sets `BUILDMAX_HOME=<run_global_dir>` and runs `buildmax -p <input> --session-id <id>`. After `Process` returns, `runPrintMode` has `stats` with token counts; it calls `executor.WriteUsageFile(config.DataDir(), stats.PromptTokens, stats.CompletionTokens)`, so `usage.json` is written under run global.
5. **Executor after run**
  `RunTask` has `runGlobal` (paths.RuntimeChatRunGlobalDir(...)). After `runBuildmaxCmd` returns, it calls `ReadRunUsage(runGlobal)`; if usage is present, sets `result.PromptTokens`, `result.CompletionTokens`. `reportRunSuccess` builds `PatchChatRunRequest` with status, endedAt, output, artifact, and optional PromptTokens/CompletionTokens; sends PATCH.
6. **Server**
  Worker handler decodes PATCH body (including optional `prompt_tokens`, `completion_tokens`); calls `UpdateChatRunStatus(..., req.PromptTokens, req.CompletionTokens)`; store writes to `chat_run` columns.

7. **Session (multi-turn accumulation)**
  After each agent reply, the caller (print mode or TUI) has `RunStats` with token counts for that turn. Call `session.AddUsage(stats.PromptTokens, stats.CompletionTokens)` to add to the session’s running totals, then call `PersistAfterReply`. SaveToDir writes sessionFile with prompt_tokens and completion_tokens. On next load, LoadFromDir restores those totals (or 0 for old files); the next turn’s usage is added on top, so the file always holds cumulative usage for the whole session.

**Streaming usage note**

- go-openai `ChatCompletionResponse` has `Usage`; we use it for non-stream.
- go-openai `ChatCompletionStreamResponse` currently does not include `Usage`. If the provider sends usage in a chunk, options are: (1) use a fork or newer SDK that adds `Usage` to the stream type, or (2) implement a small custom stream reader that decodes each chunk into a struct that includes `Usage`. Design keeps the same return shape for stream and non-stream; streaming can return zero usage until the SDK or custom reader supports it.

## DB and migration

- Add columns to `chat_run`: `prompt_tokens INT NULL`, `completion_tokens INT NULL`. GORM tag so existing code and migrations stay consistent. If the project uses GORM AutoMigrate, ensure the model change is applied; otherwise add an explicit migration step.

## Changes for review

- **internal/llm/types.go** — Add `Usage` struct. **internal/llm/client.go** — Return usage from `ChatWithTools` and `ChatWithToolsStream` (third return value); map from `resp.Usage` (non-stream) and from stream when available.
- **internal/agent/agent.go** — Extend `RunStats` with `PromptTokens`, `CompletionTokens`; extend `LLMCaller` to return `usage llm.Usage`; in `processLoop` accumulate usage and set stats.
- **internal/model/models.go** — Add `PromptTokens`, `CompletionTokens *int` to `ChatRun` (gorm + json snake_case).
- **internal/storage/entity/interfaces.go** — Add `promptTokens, completionTokens *int` to `UpdateChatRunStatus`. **internal/storage/entity/chat_run.go** — Implement; add usage to updates map; ensure DB columns exist (migration or AutoMigrate).
- **internal/workerapi/types.go** — Add `PromptTokens`, `CompletionTokens *int` to `PatchChatRunRequest`.
- **internal/server/worker_handlers.go** — Pass `req.PromptTokens`, `req.CompletionTokens` into `UpdateChatRunStatus`.
- **internal/executor/usage.go** (new) — `UsageFilename`, `ReadRunUsage(globalDir)`, `WriteUsageFile(dir, prompt, completion)`. **internal/executor/executor.go** — `RunResult` gains usage fields; after run read usage and set on result; `reportRunSuccess` includes usage in PATCH.
- **internal/session/session.go** — sessionFile: add `prompt_tokens`, `completion_tokens` (omitempty). Session: add internal promptTokens, completionTokens; AddUsage(prompt, completion int); PromptTokens(), CompletionTokens(); NewSessionFromData(..., promptTokens, completionTokens int). SaveToDir / LoadFromDir read and write usage (missing → 0).
- **internal/cmd/print.go** — After Process: session.AddUsage(stats.PromptTokens, stats.CompletionTokens); then PersistAfterReply; then executor.WriteUsageFile when stats have tokens.
- **internal/tui/model.go** — Before each PersistAfterReply after an assistant reply, call session.AddUsage(stats.PromptTokens, stats.CompletionTokens) (ensure RunStats is in scope at those call sites).
- **Tests** — LLM client usage (mock); agent accumulation; executor ReadRunUsage/WriteUsageFile; session save/load with usage and AddUsage accumulation; optional store/handler test for usage persistence. Update mocks: `LLMCaller`, `ChatRunStore.UpdateChatRunStatus` signatures.

