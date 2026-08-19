# Context Durability

## Status

- roadmap_priority: `P0.5`
- status: `phases 1–3 implemented` (§8 phases 1–3 landed; phase 4 open)
- implements: [trust-harness.md](./trust-harness.md) §3.6 (agent memory, session memory)
- follows: [hook-system.md](./hook-system.md), [durable-run-trace.md](./durable-run-trace.md)
- roadmap: [../ROADMAP.md](../ROADMAP.md)
- created_at: `2026-08-19`

## 1. Purpose

A long-running Agent forgets. Not because anything is wrong with the model, but
because the message list is a queue: `TrimHistory` drops its head and
compaction replaces its head with a lossy summary. Anything that lives only as
a message eventually stops existing.

Two things must not be subject to that:

- **Who the Agent is.** A user-authored role and its hard constraints
  ("you are a law consultant", "never push to main"). Static for the life of
  the session.
- **What the Agent knows and is doing.** Decisions already made, paths already
  ruled out, constraints the user stated once in turn 3, and the current unit
  of work. Dynamic, agent-authored, updated throughout the run.

Today the first is partly solved and the second is not solved at all. This
document defines one storage rule, one rendering slot, and the compaction
integration that makes both survive.

It also fixes a defect in the existing compaction path (§3.3) that causes the
oldest context to be lost outright rather than summarized.

## 2. Current Baseline

What already works, and where the seams are.

**The system message is immune to trimming.** `callLLM`
(`internal/core/agent/agent.go:245`) trims only `history`, then prepends the
system message unconditionally:

```go
history = TrimHistory(history, systemTokens, contextWindow, 0)
messages := append([]llm.Message{{Role: "system", Content: systemPrompt}}, history...)
```

This is the property the whole design rests on. Anything rendered into the
system message from live state is re-derived every call and cannot age out.

**Sub-agents already have a custom role.** `SubAgentDef`
(`internal/tool/agent_def.go:17`) parses frontmatter plus a markdown body, and
the body becomes that agent type's system prompt. The primary agent has no
equivalent: `BuildEffectiveSystemPrompt` (`internal/agentapp/assembly.go:16`)
composes `DefaultSystemPrompt` + model line + global `AGENTS.md` + workspace
`AGENTS.md`, with no per-agent layer.

**Compaction exists and persists across turns.** `RunLoop` compacts at 80% of
the context window, keeping a 20% verbatim reserve
(`internal/core/agent/context.go:12`), and `Session.AddCompaction`
(`internal/core/session/session.go:109`) advances the boundary so a later turn
starts from the compacted view.

**`TodoWrite` is stateless by design.** `internal/tool/todo_write.go:29` says
so explicitly: it validates and formats a list, and stores nothing. The list
exists only as a tool result message — so it is compacted away like any other
message, and because it is the only copy, that is real loss.

## 3. Gaps

### 3.1 No durable agent role for the primary agent

A user cannot say "this agent is a law consultant" and have that hold for the
whole session. The workspace `AGENTS.md` is the closest thing, but it is
workspace-scoped, not agent-scoped, and one workspace may host several agent
roles.

### 3.2 Task and situation state live only in messages — fixed ✅

Everything the Agent learns during a run — the goal, the acceptance criteria,
the decision to use approach B after A failed, the fact that the contract is
governed by New York law — exists only as messages. Compaction is asked to
preserve it (`internal/agentapp/compactor.go:9`) but that is an LLM's
best effort over an unbounded amount of material, and it degrades with every
pass.

### 3.3 Compaction summaries do not accumulate — earlier context is lost — fixed ✅

This is a defect, not a design gap.

`RunLoop` calls `opts.Compactor.Compact(ctx, toSummarize)` where `toSummarize`
is a prefix of `history`. The previous summary is **not** in `history` — it
lives in `Session.CompactionSummary` and is rendered into the system prompt by
`internal/agentapp/app.go:500`. `AddCompaction` then overwrites it outright:

```go
s.CompactionSummary = summary
```

So the summary produced by compaction *N* contains nothing from compaction
*N-1*. After two compactions the earliest context is not summarized — it is
gone. The longer the session, the more completely the beginning disappears.
This is the direct mechanism behind "the Agent forgot the original
instruction".

A related cosmetic issue: after an in-run compaction the system prompt carries
two `<context_compaction>` blocks, because `app.go:500` already appended the
persisted one and `agent.go:152` appends the new one.

### 3.4 The permanent slot has no bound — summary bounded ✅

`CompactionSummary` is concatenated into the system prompt with no size limit.
The system prompt is the one place with no degradation path — it is re-sent at
full price on every call and cannot be trimmed. Any new permanent state
(§5.3) inherits this hazard and must be bounded by construction.

### 3.5 Attention dilution

Even when an instruction is present verbatim in the system prompt, adherence
degrades as the context fills with tool output. This is a model property, not
a context-management bug, and it is not fixed by storage. It is fixed by
re-stating the small set of invariants close to the generation point.

### 3.6 Minor: threshold comment disagrees with the constant — fixed ✅

`internal/core/agent/context.go:12` documents 0.75; the constant is 0.80. Fix
the comment while touching this file.

## 4. Direction

One rule governs everything below:

> **State that must survive lives outside the message list, is re-rendered on
> every call, and is bounded by construction.**
>
> The message list is a cache, not storage. It must be droppable without
> information loss.

Consequences that shape the rest of this document:

- **Do not classify agents by hand.** An earlier draft had the agent definition
  declare `memory: todo | facts | none`. Rejected: it pushes an ontology onto
  the user that the Agent does not need. One store, free-form content; a
  conversational agent simply leaves it empty, a task agent fills it with a
  checklist. The shape emerges from use.
- **Prefer runtime-guaranteed writes over model discipline.** A tool the model
  must remember to call will be forgotten exactly when context pressure is
  highest. Where a write matters, the runtime triggers it (§5.5).
- **Full replacement, never append.** Append-only state rots and grows.
  Rewriting the whole document on every write forces eviction as a side effect.
- **Bounded rendering is part of the storage contract**, not a later
  optimization (§5.6).

## 5. In Scope

### 5.1 Layered system prompt

`BuildEffectiveSystemPrompt` grows one layer and a fixed order:

| # | Layer | Source | Lifetime |
|---|---|---|---|
| 1 | Runtime base | `DefaultSystemPrompt` + model line | Static |
| 2 | User memory | `<BUILDMAX_HOME>/AGENTS.md` | Static per session |
| 3 | Workspace memory | `<workspace>/AGENTS.md` | Static per session |
| 4 | **Agent role** | agent definition body (§5.2) | Static per session |
| 5 | Compaction summary | `Session.CompactionSummary` | Changes on compaction |

Layers 1–4 are stable for the session, so they form a cacheable prefix. Layer 5
changes and therefore sits last. The anchoring block (§5.4) is rendered after
the messages, not here, so it never invalidates the prefix.

### 5.2 Agent role

Layer 4 is one slot holding free text. Two questions are separate and are
answered separately: what the slot contains, and who is allowed to fill it.

#### The slot

Free text, appended after layer 3. It is **additive** — it never replaces
layers 1–3, because replacing `DefaultSystemPrompt` would strip the tool-usage
conventions the runtime depends on and the resulting failure would look like a
bad model rather than a bad configuration.

One structural convention, and only one: an optional `## Invariants` section is
extracted and additionally re-stated in the anchoring block (§5.4). Its sole
purpose is to give §3.5 a small, explicitly user-chosen payload instead of
repeating the whole role every turn. Everything else in the slot is opaque
text.

Bounded at 8 KB (§5.6).

#### Writers

| Writer | Surface | Transport |
|---|---|---|
| `--append-system-prompt TEXT` | CLI, print mode | Flag |
| `--append-system-prompt-file PATH` | CLI, print mode | Flag naming a file |
| `--agent NAME` | CLI, print mode | Resolves a definition file; its body fills the slot |
| Agent definition record | Portal → worker | Worker API field (**not** argv — see below) |
| Agent picker | Desktop | In-process, no transport |

When more than one applies, concatenate in that order: the named definition's
body first, ad-hoc text after. A Portal agent may legitimately have both a
reusable base role and a per-instance customization. `--append-system-prompt`
and `--append-system-prompt-file` are mutually exclusive.

There is deliberately no replacing `--system-prompt` flag.

#### Portal does not pass a flag

The scheduler spawns the worker with one argument
(`internal/server/scheduler/runner.go:57`):

```go
cmd := exec.CommandContext(ctx, r.workerPath, "--task-run-id", run.TaskRunID)
```

Everything else the worker needs it pulls back from the server —
`GET /api/worker/task-runs/{id}`, decoded into
`workerclient.GetTaskRunResponse`. A Portal-authored system prompt therefore
travels as a **new field in the worker API contract**, following the
`TaskRunLLM` precedent in `internal/infra/workerclient/api_types.go`, including
its back-compatibility convention: absent means a worker built before the field
behaves as it always did.

Argv is the wrong transport here for a reason this repository has already
recorded, three lines above the spawn call: the run token is delivered in the
environment rather than on the command line, *"where every process on the
machine could read it."* A user-authored system prompt is exactly the kind of
content that argument applies to — the Portal field is where someone pastes the
matter background, the client constraint, the internal policy. The same
reasoning also motivates `--append-system-prompt-file` locally, since `ps` is
no less readable on a developer's shared box, and since multi-line text is
awkward to pass through a shell in the first place.

Argv has a hard ceiling as well: Linux caps a single argument at
`MAX_ARG_STRLEN` (128 KB). The 8 KB bound sits far below it, but the failure
mode at the ceiling is poor, which is another reason the transport should not
be the size-limiting factor.

#### Resolution and visibility

The slot is resolved **at run start, by whoever assembles the run**, and the
last writer wins. There is no cross-run locking of a role.

- Portal re-resolves from the agent definition record on every run, so editing
  the field takes effect on the next run. That is what someone editing a field
  expects; refusing it would be a defect, not a safeguard.
- CLI writes the resolved value at session creation and reuses it on resume,
  unless a flag is passed again.
- The session stores the value **actually used for the run**. It is a record,
  not the authority.

What makes last-writer-wins safe is visibility, not enforcement: the run trace
records which system-prompt layers were loaded and their sizes. This is not a
new requirement — trust-harness §3.6 already states that the Agent should
expose which memory sources were loaded for a run, and §3.3 of that document
lists it as currently deferred. An identity change becomes observable rather
than something an error has to prevent.

(An earlier draft of this section proposed erroring when a session is resumed
under a different role. That is superseded: it would break the ordinary Portal
edit case, and the trace requirement covers the concern it was guarding.)

#### Named definitions and the subagent namespace

`--agent NAME` resolves from `config.AgentDefsSearchPaths`
(`internal/config/config.go:136`) — `<workspace>/.buildmax/agents/` then
`~/.buildmax/agents/` — the same files the `Task` tool already loads, in the
same `SubAgentDef` format (`internal/tool/agent_def.go:17`). No new file
format, no new discovery path:

```markdown
---
name: law-consultant
description: Answers contract questions for the current matter
---
You are a law consultant. Cite the governing statute for every claim.

## Invariants
- Never state a conclusion without naming the jurisdiction it rests on.
- If the jurisdiction is unknown, ask before advising.
```

The overlap is accepted for this pass: every subagent becomes launchable as a
primary role, and every role appears as a `Task` delegation target. The known
friction is that `description:` is written for the delegating LLM, which reads
oddly as a primary-role description. If that becomes a real problem, an
optional `scope: primary | subagent | both` frontmatter field resolves it —
deferred until someone hits it, rather than adding a concept on speculation.

`--agent` is a convenience over the flags, not the primary interface. The
primary interface is the free-text slot.

### 5.3 Session Notes

A bounded, agent-maintained list of short entries stored on the session and
rendered on every call.

Naming: the LLM-facing tool is **`NoteWrite`**, matching the PascalCase
convention and the `TodoWrite` pairing in `internal/tool/names.go`.
Alternatives were rejected for reasons that are worth recording, because the
tool name is prompt surface the model reads on every call:

| Rejected | Reason |
|---|---|
| `KeyNote` | Reads as "Keynote" in English — Apple's presentation app, keynote speech |
| `Whiteboard`, `Scratchpad` | Connote disposable scratch space; the opposite of the guarantee |
| `Memory` | Claimed by vector-store retrieval systems; over-promises |
| `Journal` | Implies append-only chronology, which §4 rejects |
| `Notebook` | Collides with Jupyter |

Storage — a new field on `session.Session`, persisted in the session file:

```go
// Note is one durable session note. Notes survive history compaction and are
// re-rendered on every model call.
type Note struct {
    Text      string `json:"text"`
    WrittenAt int    `json:"written_at"` // iteration index, for staleness judgement
}
```

Shape constraints, enforced by the tool and not by the model's goodwill:

- At most **15 entries**, each at most **200 characters**.
- Entries are plain one-liners. Not a free-form document: prose rots
  invisibly, a short list stays legible and self-bounding.
- The call carries the **complete** list and replaces the stored one.
- Over-limit calls fail with a tool result naming the limit and asking the
  model to merge or drop entries. Per the repository conventions, that failure
  output must be useful to the LLM, not a bare error.

The tool description carries the behavioural contract, which matters more than
the name:

> These notes survive compaction of the conversation history and are shown to
> you on every turn. Record only what cannot be recovered afterwards and that
> you will still need: decisions and their reasons, approaches already ruled
> out, constraints the user stated once, facts about the situation that later
> answers depend on. Do not record what re-reading a file would recover, and do
> not narrate what you are currently doing. Pass the complete list; it replaces
> the stored one.

`TodoWrite` keeps its own tool identity — "start a task, mark it in_progress"
is a strong trigger convention worth preserving — but becomes stateful and
backed by the same session storage, so its list survives compaction and renders
in the same block. Whether the two tools eventually merge is left open (§11).

Reading and writing take different routes, and the split is forced by an
existing property of the runtime rather than chosen for symmetry.

**Reading** goes through an optional interface on the history, mirroring the
`CompactionHistory` extension (`internal/core/agent/agent.go:41`), so
`internal/core/agent` needs no dependency on session persistence:

```go
// NotesHistory is an optional extension of MessageHistory implemented by
// histories that carry durable session state.
type NotesHistory interface {
    MessageHistory
    NoteStore
}
```

**Writing** cannot use that route, because a tool has no reference to the
history. Nor can a tool hold the session: `AgentApp` caches one tool registry
per model name (`internal/agentapp/app.go`), so a single tool instance is shared
by every session using that model, and a session pointer on the tool would leak
one session's notes into another. The store therefore reaches tools through the
context, alongside the session ID that already travels that way:

```go
type NoteStore interface {
    Notes() []Note
    SetNotes(notes []Note, iter int)
    Todos() []Todo
    SetTodos(todos []Todo, iter int)
}

func CtxWithNoteStore(ctx context.Context, s NoteStore) context.Context
func NoteStoreFromContext(ctx context.Context) (NoteStore, bool)
```

The `iter` argument is what lets an entry keep its age across a full-list
rewrite: an entry whose text is unchanged keeps the iteration it first appeared
at, and only genuinely new entries are stamped with the current one. For a todo
the key is content *plus* status, so moving a task to `in_progress` restarts its
clock — that is the number worth reporting. `RunLoop` puts the iteration on the
context before executing tool calls.

A run whose context carries no store is not an error. Tools fall back to
formatting the list and saying plainly that it was not kept, because reporting
success for a note that then vanishes is worse than reporting nothing.

### 5.4 Anchoring block

One block, rendered after the message list on every call, carrying the state
that §3.5 requires close to the generation point:

```text
<session-state>
## Invariants
- Never state a conclusion without naming the jurisdiction it rests on.

## Notes
- [i12] Matter is governed by New York law; client is the lessee.
- [i40] Approach A (rescission) ruled out — outside the limitation period.

## Todo
- [in progress, 38 iterations] Draft the notice of default
- [pending] Check the cure period against §4.2
</session-state>
```

Rendering rules:

- Rendered as a `user`-role message appended after `history` in `callLLM`.
  This is safe on the OpenAI-compatible transport this repository uses
  (`internal/infra/llm/client.go`, `internal/infra/llmwire`), which accepts
  consecutive user messages. A future native Anthropic transport must merge
  consecutive user messages, since that API requires alternation.
- **Empty sections are omitted; an entirely empty block is not rendered.** A
  conversational agent that never writes notes pays nothing. This is what
  replaces the rejected per-agent classification: absence is the signal.
- Not persisted to session history. It is a projection, regenerated each call,
  and must never accumulate as messages.
- Excluded from `EstimateTokens` accounting only if it is also excluded from
  the request; otherwise it counts against the window like any message.

### 5.5 Compaction integration

Three changes, in descending order of value.

**a. Forced checkpoint before compaction — shipped ✅.** Before summarizing, the
Agent is given a bounded turn to update its notes, prompted with the material
that is about to be lost:

> The transcript below is about to be removed from the conversation. Anything in
> it you will still need must be in your notes now — this is the last moment the
> material exists.

This is the single highest-value item in this document. It converts note-taking
from "hope the model remembers to call the tool" into a runtime-guaranteed
checkpoint at the exact moment information is destroyed, and it directly
addresses §1's failure mode. It costs one extra model call per compaction —
acceptable, since compaction already costs one.

As implemented:

- The checkpoint is an interface, `agent.StateCheckpointer`, on `RunLoopOpts`,
  in the shape `ContextCompactor` already established: `internal/core/agent`
  defines it, `internal/agentapp` implements it with the run's LLM client.
  `internal/core/agent` cannot build the tools itself — `internal/tool` imports
  it, so the dependency only runs one way.
- It fires inside `checkpointAndCompact`, after the `PreCompact` hook has
  allowed the compaction. It is not itself a hook: `PreCompact` runs
  user-configured external commands, and a blocked compaction destroys nothing,
  so there is nothing to check point.
- The turn budget is **two**, not one. A write rejected by validation — an
  over-long note list — would otherwise lose the material outright at the one
  moment it cannot be recovered, so the rejection is returned as a tool result
  and the model gets one correction. It stops as soon as a turn writes cleanly
  or produces no tool calls.
- The tool set is exactly `NoteWrite` and `TodoWrite`. With a file or shell tool
  in reach the model treats the checkpoint as a turn to keep working.
- The discarded messages are flattened into a transcript rather than replayed as
  structured messages. Replaying them would present assistant tool calls naming
  tools the checkpoint does not offer, and a transcript reads the way the
  question is posed — a record to review, not a conversation to continue.
- Failure is logged and compaction proceeds. Losing a checkpoint costs context;
  skipping the compaction it guards would cost the run.

**b. Accumulating summaries — fixes §3.3.** The previous summary must reach the
compactor. Prepend it to `toSummarize` as a synthetic message so compaction *N*
summarizes *(summary N-1 + newly discarded messages)* rather than the messages
alone. `AddCompaction` keeps its replace semantics, which are then correct
because the new summary subsumes the old one.

**c. Live-state anchoring for summary quality — shipped ✅.** The current notes
and open todos are passed into the compaction prompt as a relevance signal:
*these are live — preserve detail on anything bearing on them; compress
everything else to one line*. Before this, `compactionSystemPrompt` gave the
model no signal about what was still open and spent its budget evenly over
material of very unequal value.

`LLMCompactor` reads the store from the context it is already given, so
`ContextCompactor` did not have to widen.

Also in this pass: de-duplicate the `<context_compaction>` block so `RunLoop`
replaces the persisted block rather than appending a second one (§3.3).

### 5.6 Budgets

Every permanent slot gets an explicit ceiling. The system prompt cannot be
trimmed, so unbounded growth there is worse than losing a message.

| Slot | Bound | Behaviour at limit |
|---|---|---|
| Agent role (layer 4) | 8 KB | Reject at resolution time with a clear error naming the writer (flag, file, or API field) |
| `## Invariants` | 1 KB | Truncate at render; warn once |
| Compaction summary | ~2% of context window | Re-summarize the summary on the next pass |
| Notes | 15 entries × 200 chars | Tool call fails, model must merge |
| Todo | 30 entries | Completed entries collapse to a count |
| Anchoring block, total | ~800 tokens | Drop by priority: invariants > todo in-progress > notes > pending todo > completed |

## 6. Out Of Scope

- **Cross-session memory.** Notes are session-scoped. User, workspace, and team
  memory remain `AGENTS.md` files. Promoting a note to durable user memory is a
  separate design.
- **Automatic drift detection.** The runtime could count iterations since the
  in-progress todo last changed and inject a stronger reminder past a
  threshold — a task-level generalization of the existing `loopGuard`
  (`internal/core/agent/policy.go:36`). Deliberately deferred: it needs the
  stateful todo from §5.3 to exist first, and it should be designed against
  observed drift rather than guessed thresholds. The `written_at` field is
  specified now so the data is available when that design starts.
- **Note-taking by sub-agents as a feature.** Sub-agent runs are short and
  their result is returned to the parent, so nothing is designed around them
  keeping notes. Isolation, however, is not optional: the parent's store arrives
  on the context a sub-agent inherits, so `internal/tool/subagent_runner.go`
  repoints it at the sub-agent's own session. Without that, a sub-agent calling
  `TodoWrite` would overwrite the task list of the run that delegated to it.
  The sub-agent's state is discarded when it returns.
- **Portal/Desktop UI for viewing and editing notes.** Trust-harness §3.6
  requires memory to be inspectable and user-controllable; the storage here
  makes that possible, and the surfaces specify it.
- **Semantic typing of notes.** No categories, no schema beyond text plus
  iteration. §4 explains why.

## 7. Runtime Flow

One iteration of `RunLoop`, with the new steps marked:

1. Read `history` from the session.
2. If estimated tokens exceed the compaction threshold:
   - **fire `PreCompact`, then give the Agent one turn to update notes over the
     messages about to be discarded (§5.5a);**
   - split into `toSummarize` / `toKeep`;
   - **prepend the previous summary to `toSummarize` (§5.5b);**
   - **pass live notes and open todos to the compactor (§5.5c);**
   - store the summary, advance the boundary, fire `PostCompact`.
3. Build the system prompt: layers 1–4 (stable) + layer 5 (summary, **replacing**
   any persisted block rather than appending).
4. Trim `history` to the remaining budget.
5. **Render the anchoring block from live state; append it after `history` if
   non-empty (§5.4).**
6. Call the model.
7. If the model calls `NoteWrite` or `TodoWrite`, **write through to session
   state** and persist; the tool result stays a normal message and is free to
   be trimmed later, because it is no longer the only copy.

Step 7 is what makes trimming safe: once state has a home outside the message
list, the historical tool results are redundant and dropping them loses
nothing. Compaction gets cheaper and safer at the same time.

## 8. Implementation Steps

### Phase 1 — fix what is already broken — shipped ✅

Independent of everything else; ship first.

1. Accumulate compaction summaries (§5.5b) — pass the previous summary into
   `Compact`.
2. De-duplicate the `<context_compaction>` block (§3.3).
3. Bound the summary (§5.6) and fix the threshold comment (§3.6).
4. Tests: three consecutive compactions preserve a fact stated before the first
   one; the system prompt never carries two summary blocks; the summary stays
   under its ceiling across ten compactions.

Landed as: `withPriorSummary`, `clampSummary`, `maxSummaryChars`, and
`RenderCompactionBlock` in `internal/core/agent/context.go`;
`CompactionHistory.PriorSummary` implemented by `session.Session`; the summary
append removed from `RunPrompt` so `RunLoop` is the block's only renderer.
Regression coverage in `internal/core/agent/compaction_test.go`, plus an
ownership guard in `internal/agentapp/prompt_test.go`.

### Phase 2 — session notes — shipped ✅

5. `session.Note`, `Session.Notes`, JSON persistence, `NotesHistory` interface.
6. `NoteWrite` tool + `names.go` entry; `TodoWrite` becomes stateful over the
   same store.
7. Anchoring block rendering in `callLLM`, with the §5.6 priority ladder.
8. Tests: a note written before a compaction is present in the request after
   it; an empty store renders no block; over-limit writes fail with usable
   output; the block is never persisted into session history.

Landed as: `Note`, `Todo`, `NoteStore`, `NotesHistory`, the context accessors,
the validators, the `Stamp*` age-preserving helpers, and `RenderSessionState`
in `internal/core/agent/notes.go`; `NoteEntries`/`TodoEntries` plus the
`agent.NoteStore` methods on `session.Session`; `internal/tool/note_write.go`
and a stateful `TodoWrite`; the block appended after `history` in `callLLM`.
Coverage in `internal/core/agent/notes_test.go`,
`internal/tool/note_write_test.go`, and a persistence round-trip in
`internal/agentapp/session_manager_test.go`.

Two things the plan did not anticipate. Writes go through the context rather
than the history, for the registry-caching reason recorded in §5.3. And
`TodoWrite` now rejects a list with more than one `in_progress` entry: the
anchoring block reports "in progress for N iterations" against a single active
task, and a list with two of them has no such answer.

Scope note: `NoteWrite` and `TodoWrite` are part of the workspace agent's tool
set. Tier 1 conversation runs (`internal/service/conversation/runtime`) build
their own narrow tool list for task orchestration and offer neither, so nothing
about this pass changes Portal conversation behaviour.

### Phase 3 — forced checkpoint — shipped ✅

9. Note-update turn on `PreCompact` (§5.5a).
10. Live-state anchoring in the compaction prompt (§5.5c).
11. Tests: a fact present only in the discarded window survives compaction via
    notes.

Landed as: `StateCheckpointer` and `checkpointAndCompact` in
`internal/core/agent/agent.go`; `NoteCheckpointer` in
`internal/agentapp/note_checkpoint.go`; live-state anchoring inside
`LLMCompactor.Compact`. Coverage in `internal/core/agent/checkpoint_test.go`
(ordering, iteration stamping, fail-open, skipped when a hook blocks
compaction, and the end-to-end survival claim) and
`internal/agentapp/note_checkpoint_test.go` (tool set, transcript and live
state in the request, retry after a rejected write, turn budget, the two
no-op cases, and the compactor's anchoring).

### Phase 4 — agent role

12. Layer 4 in `BuildEffectiveSystemPrompt`, with the 8 KB bound and
    `## Invariants` extraction into the anchoring block.
13. CLI flags: `--append-system-prompt`, `--append-system-prompt-file`
    (mutually exclusive), and `--agent` resolving from
    `config.AgentDefsSearchPaths`. Root command wiring in
    `internal/interface/cli/root.go`, alongside `--model`.
14. Worker API field for the Portal path in
    `internal/infra/workerclient/api_types.go`, following the `TaskRunLLM`
    back-compatibility convention, plus the server side that populates it from
    the agent definition record.
15. Session field recording the resolved value used for the run, and trace
    records naming which system-prompt layers were loaded and their sizes
    (trust-harness §3.6).
16. Tests: layers compose in the documented order; an over-limit slot is
    rejected with a message naming the writer; a worker without the new field
    behaves as before; the trace names every layer that contributed.
17. User documentation in `guide/`, linked from the design index.

Phases 1 and 2 are independently valuable. Phase 3 depends on 2. Phase 4 is
independent of 1–3 and can be reordered by roadmap need.

## 9. Acceptance

- A fact stated in the first turn of a session is still available to the model
  after three compactions — via notes, not via luck.
- A user-authored role holds for the whole session and is not weakened by
  compaction.
- A conversational agent that never writes a note pays no per-turn token cost
  for this feature.
- No permanent slot grows without bound; each has a specified behaviour at its
  limit.
- Dropping every historical `NoteWrite` / `TodoWrite` tool result from the
  message list loses no state.
- Compaction failure remains fail-open, matching the trace and hook
  conventions: a failed note checkpoint logs at warn and the run continues.

## 10. Layering

No architecture boundary moves. `internal/core/agent` learns about notes only
through the `NotesHistory` interface, exactly as it already learns about
compaction through `CompactionHistory`; `internal/core/session` owns the data;
`internal/tool` owns `NoteWrite`; `internal/agentapp` wires them. Nothing in
`internal/core` gains a dependency on config, infra, or service packages, so
the `internal/architecture` tests are unaffected.

## 11. Open Questions

- **Do `NoteWrite` and `TodoWrite` eventually merge?** Notes with a status
  field would subsume todos. Against: `TodoWrite`'s trigger convention is
  strong and well-learned, and merging costs that. Decide after Phase 2 has
  usage.
- **Should the anchoring block go in the system prompt tail instead of after
  the messages?** The system prompt is simpler and provider-neutral; after the
  messages is closer to the generation point and should adhere better. This
  design chooses after-the-messages; if adherence proves indistinguishable,
  moving it into the system prompt removes the transport caveat in §5.4.
- **Should a role be able to suppress inherited layers?** Layer 4 is additive,
  so a `law-consultant` role running in a Go repository also receives that
  workspace's `AGENTS.md` contribution rules. Usually harmless, since the later
  layer carries more weight, but "give me a clean role" is a reasonable ask. A
  frontmatter `inherit: false` would cover it. Deferred because the right
  default is not knowable before seeing real usage.
- **Should notes be redacted before persistence?** They are model-authored and
  land in the session file, which the durable run trace already treats as
  needing redaction. Probably yes, reusing the trace redactor — confirm during
  Phase 2.
