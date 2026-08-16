# Sessions and Traces

> **Audience:** users · **Status:** current
>
> Trace design and remaining phases:
> [design/durable-run-trace.md](../design/durable-run-trace.md)

Two different records of what happened, for two different questions.

| | Session | Trace |
|---|---|---|
| Answers | "What did we talk about?" | "What did it actually do?" |
| Contains | The conversation, resumable | Every LLM call and tool call in one run |
| Lives in | `<BUILDMAX_HOME>/sessions/` | `<BUILDMAX_HOME>/traces/` |
| Read by | You, and the agent on resume | You, when something went wrong |

## Sessions

Each session is one JSON file, `sessions/<id>.json`, plus an index in
`sessions.json` that the picker reads without opening every file.

```json
{
  "id": "…",
  "title": "Add pagination to the list endpoint",
  "created_at": "…",
  "messages": [ … ],
  "prompt_tokens": 18234,
  "completion_tokens": 2011
}
```

Titles are generated from the first exchange, which is why `/sessions` is
readable rather than a list of ids. Token counts accumulate across the whole
session, including the title-generation call.

### Resuming

```bash
buildmax --continue              # most recent session
buildmax --resume <session-id>   # a specific one
buildmax --session-id <uuid>     # load if it exists, otherwise create it
```

`--session-id` is the one to script with: it makes a run idempotent against a
known id instead of depending on what happens to be most recent.

In the TUI, `/sessions` opens the picker. Sessions are saved after each
assistant reply, so an interrupted run does not lose the conversation up to that
point.

### Compaction

When a conversation approaches the model's context window, older messages are
compacted into a summary, recorded in the session as `compaction_idx` and
`compaction_summary`. The `pre_compact` hook can block this, and `post_compact`
observes it — see [hooks.md](hooks.md).

## Traces

Every run writes one JSONL file:

```text
<BUILDMAX_HOME>/traces/<session_id>/<run_id>.jsonl
```

Run ids are prefixed `rt_`. The file opens with a `run_start` and a
`sandbox_boundary` record, then one record per event, then a terminal
`run_end`:

| Record | Emitted when |
|---|---|
| `run_start` | The run begins |
| `sandbox_boundary` | Always, right after `run_start` — reports the boundary the run actually ran under. `"sandboxed": false` means nothing confined the run's `Bash` commands |
| `llm_start` / `llm_end` | Each model call |
| `tool_start` / `tool_end` | Each tool call |
| `tool_denied` | A tool call was blocked — by a hook, or by permission |
| `context_compacted` | The conversation was compacted |
| `run_end` | The run finished, with an error message if it failed |

Because it is JSONL, ordinary tools work:

```bash
# every tool call in the last run
jq -r 'select(.type=="tool_start") | .tool_name' \
   ~/.buildmax/traces/<session>/<run>.jsonl

# what got blocked
jq 'select(.type=="tool_denied")' ~/.buildmax/traces/<session>/*.jsonl

# how the run ended
jq 'select(.type=="run_end")' ~/.buildmax/traces/<session>/<run>.jsonl
```

### Bounded and Redacted

Free-text fields are truncated and common secret shapes are redacted before
anything is written. A trace is bounded in record count too — once the cap is
reached, further records are dropped, but `run_end` is still written so the file
always terminates properly.

Tracing **fails open**: a trace that cannot be written never breaks or slows the
run.

### Coverage and Disabling

Traces attach at the single point every surface runs through, so CLI, TUI,
Desktop, evaluation, and worker runs all produce them.

```bash
BUILDMAX_TRACE_DISABLED=1 buildmax -p "…"
```

On by default. Turn it off if you have a reason; it is the record you will want
when a run does something surprising.

## Related

- [hooks.md](hooks.md) — acting on events rather than recording them
- [reference/cli.md](../reference/cli.md) — `--output jsonl` for live events on stdout
