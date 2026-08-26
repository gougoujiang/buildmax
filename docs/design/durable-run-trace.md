# Durable Run Trace

## Status

- roadmap_priority: `P0.5`
- status: `phase 1 implemented` (§8 phase 1 landed; follow-ups in §7 still open)
- implements: [trust-harness.md](./trust-harness.md) §3.3
- follows: [hook-system.md](./hook-system.md)
- roadmap: [../ROADMAP.md](../ROADMAP.md)
- created_at: `2026-06-03`

## 1. Purpose

[trust-harness.md](./trust-harness.md) §3.3 calls for a **durable run trace**
that explains what happened during an
Agent run: model calls, tool calls, approval decisions, hook execution, file
changes, compaction, errors/retries, token usage and timing, subagent
relationships, sandbox mode, and the memory/instruction sources used.

The shared Agent Core already emits a structured in-memory event stream
(`core/agent.Event` via `RunLoopOpts.EventSink`), but nothing persists it. When
a run ends, the only durable artifacts are the session messages and the rotated
log file — neither of which reconstructs *what the Agent did and why*.

This document defines a persistent, bounded, redacted trace built on the
existing event stream, attached at the one chokepoint every surface already uses
(`agentapp.RunPrompt`), so CLI/TUI, Desktop, agent eval, and the worker all
produce traces with no per-surface code.

This is the prerequisite for §3.4 (activity views), §3.7 (subagent
traceability), §3.8 (worker diagnostics), and Portal run diagnostics.

## 2. Direction

- **Build on the event stream, do not invent a parallel one.** The trace is an
  `EventSink` consumer. New record types are only added when the event stream
  cannot already express something (this pass adds none — it maps the existing
  9 `EventKind`s plus a synthetic `run_start`).
- **One chokepoint.** `agentapp.RunPrompt` is the single call site for CLI
  (`print.go`, `tui_model.go`), Desktop, evaluation, and the worker
  (`agentapp/taskrun`). Wiring the recorder there covers every surface.
- **Fail open, always.** A trace failure (disk full, permission denied, encode
  error) must never break or slow a run. Every recorder error is logged at warn
  and dropped.
- **Bounded and redacted by default.** Large tool results and model content are
  truncated; common secret shapes are scrubbed before bytes hit disk.
- **Inspectable, not yet a product.** Phase 1 persists JSONL that the activity
  views (§3.4) and a future `buildmax trace` command read. No UI in this pass.

## 3. Storage model

### 3.1 Layout

```
<BUILDMAX_HOME>/sessions/<session_id>/traces/<run_id>.jsonl
```

- One file per `RunPrompt` invocation (one run).
- Grouped by session so a conversation's runs sit together.
- This record first put that directory at `<BUILDMAX_HOME>/traces/<session_id>/`.
  [local-session-storage.md](./local-session-storage.md) §10 moved it inside the
  session bundle; the per-run contract is unchanged, only where the file sits.
  The traces root still exists for what belongs to no session — a local
  background job writes `<BUILDMAX_HOME>/traces/jobs/<job_id>.jsonl`.
- `run_id` uses the prefixed-ID format (CLAUDE.md §6.3) with prefix `rt_`
  ("run trace"): `rt_<20 base36 chars>`.
- JSONL: one self-describing record per line, append-only. JSONL survives a
  crash mid-run (every completed line is valid) and streams cheaply.
- Each record is flushed as it is written, not buffered until close. The
  survives-a-crash property above is the whole point of the format, and a
  buffered tail silently takes it away: the records are lost precisely on the
  runs that ended badly, and the last event left on disk is an earlier one than
  the last event that happened — which sends a reader diagnosing the failure to
  the wrong place. Trace volume is a handful of records per model call, so the
  per-record write is not a cost worth trading that for.

### 3.2 Record schema

All keys are snake_case (CLAUDE.md §6.1). One Go struct with `omitempty`
fields and a `type` discriminator; fields irrelevant to a record type are
omitted.

```jsonc
// run_start — synthesized by the recorder at construction
{"ts":"2026-06-03T10:00:00Z","type":"run_start","run_id":"rt_…","session_id":"c_…",
 "workspace":"/path","model":"…","is_subagent":false,"trace_version":1}

// subagent run_start — parent_run_id is the immediate parent, so a trace tree
// can be reconstructed one edge at a time. Top-level runs omit the field.
{"ts":"…","type":"run_start","run_id":"rt_child","parent_run_id":"rt_parent",
 "session_id":"…","is_subagent":true,"trace_version":1}

// sandbox_boundary — synthesized at construction; added after phase 1 so a run
// states the boundary it ran under. Written even when nothing confined the run,
// because an absent record reads as "not checked" rather than "not confined".
{"ts":"…","type":"sandbox_boundary","sandboxed":false,"backend":"none",
 "sandbox_mode":"auto_allow","sources":["default:worker","policy"]}

// plugins — synthesized at construction, for the same reason: naming what the
// run loaded from outside the workspace, and for a repository plugin whether
// that input could still change under it. Bounded metadata only, never package
// content. See design/plugin-marketplace.md §10.
{"ts":"…","type":"plugins","plugins":[
  {"name":"code-review","source":"repository","remote_url":"git@…",
   "commit":"…","branch":"main","dirty":false},
  {"name":"release","source":"marketplace","version":"1.2.0",
   "digest":"sha256:…"}]}

// iter_start  (EventIterStart)
{"ts":"…","type":"iter_start","iter":1}

// llm_start   (EventLLMStart)
{"ts":"…","type":"llm_start","iter":1,"context_tokens":1234,"context_window":128000}

// llm_end     (EventLLMEnd) — content bounded
// The cache counts are a breakdown of prompt_tokens, not an addition to it, and
// are omitted entirely when the provider reported none.
//
// prompt_tokens and friends are the run's totals so far; the call_* fields are
// what this one call did. Both are here because subtracting consecutive
// records to get the second goes wrong as soon as a call in between failed.
// cost is this call's, in nano-currency-units, and is absent when the model
// was unpriced — which is not the same fact as a call that cost nothing.
{"ts":"…","type":"llm_end","iter":1,"has_tool_calls":true,
 "prompt_tokens":1200,"completion_tokens":80,
 "cache_read_tokens":900,"cache_write_tokens":100,
 "call_prompt_tokens":1200,"call_completion_tokens":80,
 "call_cache_read_tokens":900,"call_cache_write_tokens":0,
 "cost":{"currency":"USD","uncached":900000,"cache_read":270000,"cache_write":0,
         "output":1200000,"total":2370000,"baseline":4800000},
 "content":"…"}

// tool_start  (EventToolStart) — args bounded + redacted
{"ts":"…","type":"tool_start","tool":"writefile","tool_call_id":"call_1","args":"{…}"}

// tool_end    (EventToolEnd) — result bounded + redacted
{"ts":"…","type":"tool_end","tool":"writefile","tool_call_id":"call_1",
 "result":"ok","duration_ms":12}

// tool_denied (EventToolDenied)
{"ts":"…","type":"tool_denied","tool":"bash","deny_reason":"policy"}

// context_compacted (EventContextCompacted)
{"ts":"…","type":"context_compacted","summarized":3,"kept":5}

// user_input  (EventUserInput) — a message queued mid-run, joining at an
// iteration boundary; bounded + redacted, because it is part of what the run
// was told to do and a trace that omitted it would misreport its instructions.
{"ts":"…","type":"user_input","iter":3,"content":"also check the tests"}

// user_input_blocked (EventUserInputBlocked) — a UserPromptSubmit hook refused it
{"ts":"…","type":"user_input_blocked","iter":3,"content":"…","deny_reason":"no secrets"}

// run_end     (EventRunEnd)
// cost here is the run's, summed from the calls above. cost_incomplete says a
// call did work that could not be priced, so the total understates the run.
{"ts":"…","type":"run_end","tool_calls":2,"prompt_tokens":1200,
 "completion_tokens":80,"cache_read_tokens":900,"cache_write_tokens":100,
 "cost":{"currency":"USD","total":2370000,"baseline":4800000},
 "error":""}
```

`EventLLMDelta` is **not** persisted — streaming deltas are redundant with the
final `llm_end` content and would bloat the file. `llm_end.content` carries the
bounded final assistant text.

### 3.3 Bounding

- `content`, `args`, `result` are each truncated to `maxFieldBytes` (default
  4096) with a `… [truncated N bytes]` suffix. The cut backs off to a UTF-8
  rune boundary so a multi-byte character is never split — a partial rune would
  be re-encoded as U+FFFD by the JSON encoder.
- A per-run record cap (`maxRecords`, default 10000) guards a runaway loop;
  once hit, further records are dropped and a single `truncated` warning is
  logged. The `run_end` record is always attempted.

### 3.4 Redaction

Applied to `content`, `args`, `result` before writing:

- `Bearer <token>` → `Bearer [redacted]`
- `sk-<≥16 chars>` → `[redacted]`
- `(?i)(authorization|api[_-]?key|token|secret|password)` followed by `=`/`:`
  and a value → key kept, value → `[redacted]`

Redaction is intentionally conservative (keyword/shape based) to avoid mangling
normal output. The pattern set lives in one file (`infra/trace/redact.go`) so it
can grow without touching the recorder.

## 4. Layering

Mirrors the hook/MCP layout (contract in core, impl in infra, assembly in
agentapp):

```
internal/core/agent/event.go        # Event/EventKind — exists, unchanged
internal/config/trace.go            # TracesDir(), TraceEnabled(), env key
internal/infra/trace/record.go      # Record struct + FromEvent mapper + bounding
internal/infra/trace/redact.go      # redaction patterns
internal/infra/trace/recorder.go    # Recorder: open file, Record(Event), Close()
internal/agentapp/app.go            # RunPrompt tees EventSink into a Recorder
```

Import compliance: `infra/trace` imports `core/agent` and `config` (both
allowed for `infra`); `agentapp` already imports `infra/*` and `config`.

## 5. Runtime flow

```
agentapp.RunPrompt(ctx, sess, prompt, stream, approval, eventSink)
  │
  ├─ resolveRunContext → model, client, session
  │
  ├─ trace.NewRecorder(TracesDir, meta{run_id, session_id, workspace, model})
  │     └─ writes run_start,           (nil + warn on error → fail open)
  │        then sandbox_boundary, prompt_layers, plugins
  │
  ├─ sink := tee(recorder.Record, eventSink)   // recorder first, then caller
  │
  ├─ agent.RunLoop(… EventSink: sink …)
  │     └─ emits iter/llm/tool/compaction/run_end → recorder + caller both see them
  │
  └─ recorder.Close()                  // flush + close file; no extra run_end
```

- The recorder is the *first* leg of the tee so a panic in the caller's sink
  cannot lose trace data; both legs are best-effort.
- `RunResult` gains `TraceID string` so callers can point a viewer at the file.
  It is populated on the error returns too (`RunLoop` failure, `finalizeTurn`
  failure) — `RunLoop` emits `run_end` with the error before returning, so the
  trace is complete, and a failed run is exactly when the caller needs it.
- **Prompt blocked by hook** (early return before `RunLoop`): the recorder
  writes `run_start` then a `run_end{error:"blocked by hook: <reason>"}` and
  closes, so blocked turns still leave a one-line explanation.

## 6. Configuration

- `config.TracesDir()` → `<DataDir>/traces`.
- `config.TraceEnabled()` → true unless `BUILDMAX_TRACE_DISABLED` is a truthy
  value (`1/true/yes/on`). Default on; registered in `env_spec.go`'s `EnvVars()` inventory and
  `.env.example`.
- Worker inherits the same env, so worker runs trace by default; their trace
  dir is the run-scoped `global/` BUILDMAX_HOME, keeping trace data with the
  run for later upload/diagnostics.

## 7. Out of scope for this pass

- Activity-view UI in TUI/Desktop (§3.4) — reads these files later.
- `buildmax trace` CLI inspector / list / export.
- Hook-execution and approval-grant records: only `tool_denied`
  (reason=`hook`/`user`) is visible from today's event stream. Dedicated
  `hook_*` / `approval_*` records need new events and are deferred.
- File-change records, sandbox-decision records, retry records — deferred until
  the event stream surfaces them.
- Distinguishing *why* a run failed. `run_end.error` carries a message, but a
  run that failed because a dependency was down reads the same as one whose
  model or task failed. `/readyz` answers whether a dependency is down now; it
  cannot answer why one run failed then. Classifying the failure needs a typed
  cause on `run_end`, which the event stream does not carry.
- Retention/rotation/GC of the traces directory. **Consequence to accept
  knowingly:** tracing is on by default and nothing ever deletes a trace, so
  the session bundles grow without bound — one trace file per run, each capped
  at 10000 records × ~4KB of bounded fields. Local use is unlikely to notice;
  a long-lived worker container that keeps its `global/` home across runs is the
  real exposure. Until a retention policy lands, operators can size that volume
  from the per-run cap or set `BUILDMAX_TRACE_DISABLED`.

## 8. Implementation steps

### Phase 1 (this pass)

1. `config/trace.go`: `TracesDir()`, `TraceEnabled()`, `EnvKeyBuildmaxTraceDisabled`; register in `env_spec.go`; add to `.env.example`.
2. `infra/trace/redact.go` + test: keyword/Bearer/sk- redaction.
3. `infra/trace/record.go` + test: `Record` struct, `FromEvent`, bounding.
4. `infra/trace/recorder.go` + test: open file under `<dir>/<session>/<run>.jsonl`, write `run_start`, `Record(Event)`, record cap, `Close()`; fail-open constructor.
5. `agentapp/app.go`: build recorder in `RunPrompt`, tee the sink, add `TraceID` to `RunResult`, handle the blocked-prompt early return.
6. `agentapp/trace_wiring_test.go`: end-to-end wiring tests over `RunPrompt` —
   trace file written and parseable (§9 bullet 1), `BUILDMAX_TRACE_DISABLED`
   writes nothing (bullet 3), an unwritable traces dir still completes the run
   (bullet 4). They drive the hook-blocked turn, the one `RunPrompt` path that
   reaches the recorder without a live LLM.
7. `./make test`.
8. Docs: mark `trust-harness.md` §3.3 in progress/done with the shipped record
   set; update `AGENTS.md` and `design/README.md`.

### Subagent linkage

Subagent traces record `parent_run_id` for their immediate parent. This forms a
trace tree without making a child depend on a root-run identifier: following
links yields the root, and future nested subagents compose the same way. When a
parent recorder could not start, no child trace is written rather than a
misleading unlinked child, preserving fail-open behavior.

### Follow-ups (later passes)

- Activity views read traces (§3.4).
- `buildmax trace` inspector.
- Hook/approval/file-change/sandbox records (need new events).
- Retention policy.

## 9. Acceptance

- After any `RunPrompt` on any surface, `<DataDir>/sessions/<session>/traces/<run>.jsonl`
  exists and contains a `run_start`, the per-iteration LLM/tool records, and a
  terminal `run_end`.
- Tool arguments/results and model content over the bound are truncated;
  Bearer/`sk-`/keyworded secrets are redacted.
- Disabling via `BUILDMAX_TRACE_DISABLED=1` produces no files and no errors.
- A forced recorder failure (unwritable dir) logs a warning and the run still
  completes normally.
- Worker task runs leave a trace in the run-scoped `global/` home.
```
