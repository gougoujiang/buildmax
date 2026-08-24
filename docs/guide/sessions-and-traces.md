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
| Lives in | `<BUILDMAX_HOME>/sessions/<id>/` | the same folder, under `traces/` |
| Read by | You, and the agent on resume | You, when something went wrong |

## Sessions

Each session is a folder, and everything about it lives inside:

```text
<BUILDMAX_HOME>/sessions/
  index.json                    what the picker reads
  <id>/
    meta.json                   title, workspace, model, token and cost totals
    history.jsonl               the conversation, one record per line
    traces/<run_id>.jsonl       one file per run
```

`history.jsonl` is append-only: each message, tool call, and tool result is
written as it happens rather than the whole file being rewritten at the end of a
turn. That is what lets an interrupted run be picked back up — and what lets
BuildMax tell a tool call that never started from one that may already have
changed something on disk. When it resumes a session that stopped mid-tool, it
says so rather than assuming either way.

Token counts and cost cover the whole session: every turn, the title-generation
call, each compaction, and any work the run delegated to a subagent.

Titles are generated from the first exchange, which is why `/sessions` is
readable rather than a list of ids.

### Resuming

```bash
buildmax --continue              # most recent session
buildmax --resume <session-id>   # a specific one
buildmax --session-id <uuid>     # load if it exists, otherwise create it
```

`--session-id` is the one to script with: it makes a run idempotent against a
known id instead of depending on what happens to be most recent.

In the TUI, `/sessions` opens the picker. The conversation is written as it
happens, so an interrupted run keeps everything up to the moment it stopped —
not just up to the last completed reply.

A session can be open in one place at a time. Opening one that another window or
process is already running reports that it is in use rather than letting two
runs write over each other.

### Compaction

When a conversation approaches the model's context window, older messages are
compacted into a summary, recorded in the journal as a `compaction` record
naming exactly which messages it covers. The `pre_compact` hook can block this,
and `post_compact` observes it — see [hooks.md](hooks.md).

## Traces

Every run writes one JSONL file:

```text
<BUILDMAX_HOME>/sessions/<session_id>/traces/<run_id>.jsonl
```

Run ids are prefixed `rt_`. The file opens with a `run_start` record and three
records describing what the run started with, then one record per event, then a
terminal `run_end`:

| Record | Emitted when |
|---|---|
| `run_start` | The run begins |
| `sandbox_boundary` | Always, right after `run_start` — reports the boundary the run actually ran under. `"sandboxed": false` means nothing confined the run's `Bash` commands |
| `prompt_layers` | Always — what the run was told before the conversation started, and how much of each |
| `plugins` | Always — which plugins the run loaded, and for one installed from a Git checkout its commit and whether the working tree was dirty |
| `llm_start` / `llm_end` | Each model call |
| `tool_start` / `tool_end` | Each tool call |
| `tool_denied` | A tool call was blocked — by a hook, or by permission |
| `context_compacted` | The conversation was compacted, with what the summarization itself cost |
| `run_end` | The run finished, with its totals, an error message if it failed, and a `delegated` breakdown when it started subagent runs |

Because it is JSONL, ordinary tools work:

```bash
# every tool call in the last run
jq -r 'select(.type=="tool_start") | .tool_name' \
   ~/.buildmax/sessions/<session>/traces/<run>.jsonl

# what got blocked
jq 'select(.type=="tool_denied")' ~/.buildmax/sessions/<session>/traces/*.jsonl

# how the run ended
jq 'select(.type=="run_end")' ~/.buildmax/sessions/<session>/traces/<run>.jsonl
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

A subagent's run is traced beside its parent's, in the same session directory,
with `is_subagent: true` and `parent_run_id` naming the run that delegated to it.

## Statistics

`buildmax stats` folds both records into one answer for a session:

```bash
buildmax stats                  # the most recent session
buildmax stats <session-id>     # a specific one
buildmax stats --json           # the full record, including the truncated tail
```

It reports spend with the cache breakdown and what caching saved, how close the
session came to its context window, how many bytes each tool put back into that
window, the split between model time and tool time, and how much of the run a
delegation did.

In the TUI, `/stats` shows the same figures for the session on screen, from the
live session rather than the last saved copy.

Tokens and cost come from the session file; timings and per-tool detail come
from the traces. Where a trace is missing, those lines say so rather than
reporting zero — see [reference/cli.md](../reference/cli.md).

## Related

- [hooks.md](hooks.md) — acting on events rather than recording them
- [reference/cli.md](../reference/cli.md) — `--output jsonl` for live events on stdout
