# Local Session Storage

> **Audience:** contributors and security reviewers · **Status:** planned
>
> Related: [session architecture](../contribute/architecture/session.md),
> [sessions and traces](../guide/sessions-and-traces.md),
> [durable run trace](durable-run-trace.md),
> [context durability](context-durability.md),
> [durable Agent sessions](../proposals/durable-agent-sessions.md), and
> [session trees and mailboxes](../proposals/session-tree-and-agent-mailbox.md).

## 1. Decision

Replace each local session's mutable whole-file JSON with a session-owned
directory whose records each carry their own contract:

```text
<BUILDMAX_HOME>/sessions/
  index.json

  <session_id>/
    meta.json
    history.jsonl
    traces/
      <run_id>.jsonl
    artifacts/
      <artifact_id>
    writer.lock
```

- `meta.json` is the small, atomically replaced current metadata record.
- `history.jsonl` is the authoritative append-only resumable conversation
  journal.
- `traces/<run_id>.jsonl` keeps the existing bounded, redacted, fail-open run
  diagnostics, one file per run.
- `artifacts/` owns large content referenced by history records.
- `writer.lock` enforces one mutable owner without treating stale process state
  as durable metadata.
- `index.json` is a rebuildable projection for the session picker.

The design borrows continuous item persistence and linked conversation history
from JSONL-based coding agents without multiplexing metadata, resumable content,
run diagnostics, and artifacts into one unbounded file.

## 2. Current State And Problem

`internal/core/session.Session` currently serializes directly to
`<BUILDMAX_HOME>/sessions/<id>.json`. It contains:

- user, assistant, and tool messages;
- assistant tool calls and tool results;
- provider-owned reasoning state and non-text parts;
- accumulated prompt and completion tokens;
- compaction boundary and summary;
- durable notes and todos; and
- the additional system prompt.

`SessionManager.Save` marshals the complete object and calls `os.WriteFile`
after a completed turn. A separate `sessions.json` stores title, workspace,
creation time, and pin state for listing. The selected model lives only in
`SessionContext`, so reopening a session currently falls back to a default.

This shape has four problems, worst first:

1. A long Agent turn may complete many model and tool steps before the session
   is saved. A crash loses the distinction between work that never started and
   work that may already have changed the world.
2. A flat `Messages` array has no durable identity for a checkpoint, rewind, or
   fork point.
3. Direct overwrite can leave the only file unparsable after interruption,
   disk exhaustion, or machine failure.
4. Every turn rewrites all previous messages, tool results, provider state, and
   image data.

The order matters. Write amplification is the most visible problem and the
least important one: on a local single-user session it is wasteful, not
incorrect, and on its own it would justify a buffer, not a format. Uncertain
tool outcomes and missing item identity are what the current shape cannot
express at all, and they are what the rest of this document is for.

Atomic replacement is a worthwhile stop-gap for the current format. It does
not solve incremental durability, tool-outcome uncertainty, or linked history.

## 3. Goals And Non-Goals

### 3.1 Goals

- Persist each completed resumable item once instead of rewriting the complete
  history.
- Preserve exact `llm.Message` values, including tool calls, provider state,
  content parts, and provenance.
- Commit enough state during a turn to recover safely after interruption.
- Give every history item a stable identity and logical parent.
- Support resume, conversation checkpoint, rewind, and fork without making the
  run trace authoritative.
- Keep title lookup and session listing fast.
- Isolate metadata, history, trace, artifact, and writer-lock failure policies.
- Keep the stored form neutral about which model provider produced a turn, and
  extensible to records this version does not define.
- Give foreground, background, and nested subagents private durable sessions
  without exposing them as ordinary user sessions.
- Preserve the single-binary local experience and keep Agent Core independent
  of storage implementations.

### 3.2 Non-goals

- Replaying or rolling back arbitrary external side effects.
- Capturing or restoring workspace file state, and treating a conversation
  checkpoint as a filesystem, process, remote service, or database snapshot.
- Persisting credentials, approval grants, process handles, or shell sessions.
- Making a local transcript tamper-proof audit evidence.
- Implementing Server synchronization, Team sharing, or remote authorization.
- Sharing a physical history prefix between forked sessions in the first
  implementation.
- Supporting simultaneous writers to one session.
- Preserving or migrating the Alpha whole-file JSON format.

## 4. Record Ownership

| Data | Authority | Why |
|---|---|---|
| Session ID, kind, creation time | `meta.json` and history header | The directory and journal remain independently identifiable |
| Title, pin | `meta.json` | Presentation metadata does not affect model context |
| Selected model | `meta.json` | Current choice for the next turn |
| Workspace root for the next turn | `meta.json` | A current selection, exactly like the selected model |
| Model and workspace root a turn ran under | `turn_started` history record | Facts about a specific turn |
| Token totals | `meta.json` | Local aggregate reporting; per-call evidence remains elsewhere |
| Messages and tool results | `history.jsonl` | Required to reconstruct provider history |
| Tool crossed execution boundary | `history.jsonl` | Required to classify interrupted effects safely |
| Compaction summary and boundary | `history.jsonl` | Changes the model-visible history on a particular branch |
| Notes, todos, additional prompt | `history.jsonl` | Durable state injected into later model requests |
| Current history head | `head_selected` history record | Derived on load and never stored as metadata, so it cannot disagree with the branch it names |
| LLM/tool timing, working directory, sandbox, plugins, denial diagnostics | per-run trace | Bounded operational evidence, not resume input |
| Large or binary content | `artifacts/` plus a history reference | Independent size, integrity, and lifecycle controls |
| Active writer | OS lock plus `writer.lock` diagnostics | A process fact that must become stale automatically |

`meta.json` and `history.jsonl` intentionally do not have one transaction, and
neither is a projection of the other. They hold different kinds of authority:

- `history.jsonl` is authoritative for everything that reconstructs a
  conversation: messages, tool outcomes, compaction, and durable state.
- `meta.json` is authoritative for the session's current selections and
  running aggregates: title, pin, workspace, selected model, token totals, and
  accumulated cost. These are settings and counters, not summaries of the
  journal. No amount of replay recovers them, and no history record needs to.
- `index.json` is the only pure projection in this design, and the only file
  claimed to be rebuildable by scanning.

Two authorities are safe here because their failures are not symmetric: losing
a current selection degrades presentation and defaults, while losing history
breaks resume. A stale title or index row must never make conversation recovery
incorrect.

The split only works if nothing straddles it, so `meta.json` holds nothing that
`history.jsonl` also determines. The current head is a `head_selected` record
and is derived on load (§6.2); it is not mirrored into metadata. A
last-sequence counter is not stored either. Both are known for free the moment a
session
opens, because opening replays the journal, so caching them in metadata
would buy no speed and cost a consistency rule.

Immutable duplication is different and is allowed: session ID, kind, and
creation time appear in both `meta.json` and the history header, which is what
makes each file independently identifiable. Values fixed at creation cannot
drift, and §6.1 already defines a mismatch between them as corruption rather
than as a stale copy to reconcile. The rule is therefore about mutable state:
two records may repeat a constant, and must not repeat a variable.

## 5. `meta.json`

The per-session metadata file is small enough to replace atomically:

```json
{
  "version": 1,
  "id": "<uuid>",
  "kind": "user",
  "created_at": "2026-08-23T10:00:00Z",
  "updated_at": "2026-08-23T10:30:00Z",
  "title": "Design local session storage",
  "workspace": "/repo",
  "pinned": false,
  "selected_model": "anthropic/claude-opus",
  "prompt_tokens": 12000,
  "completion_tokens": 1800,
  "cache_read_tokens": 9000,
  "cache_write_tokens": 1200,
  "cost": {"currency": "USD", "total": 4200},
  "cost_incomplete": false
}
```

`kind` is `user` or `subagent`. A child adds immutable lineage metadata:

```json
{
  "kind": "subagent",
  "parent_session_id": "<uuid>",
  "parent_run_id": "<run id>",
  "parent_tool_call_id": "call_1",
  "agent_type": "explorer",
  "delegation_depth": 1,
  "hidden": true
}
```

A forked user session adds:

```json
{
  "forked_from": {
    "session_id": "<parent uuid>",
    "checkpoint_id": "<checkpoint item id>",
    "head_id": "<history item id>"
  }
}
```

`cost` is `llm.Cost` verbatim, including its cache and baseline breakdown.
`cost_incomplete` says a turn ran against an unpriced model or in a currency
this build does not convert, because a total that silently dropped half a
session is worse than one labelled incomplete. Cache token counts break
`prompt_tokens` down rather than add to it; a reader totalling a session must
not sum all three.

Title, pin, workspace, and selected-model changes write a temporary file, sync
it, and atomically replace `meta.json`. They do not append conversation events.

If metadata is damaged, the session recovers with conservative defaults: no
title, no pin, the configured default model, and zeroed aggregates. Reported
usage is then wrong, which is acceptable, because it is reporting.

None of those defaults can make recovery incorrect, because nothing here
selects model-visible state. The current head is the value that would, and it
is not in this file: after any rewind the last physical record belongs to the
branch the user abandoned, so a metadata copy that failed to keep up would
silently resume abandoned work. §6.2 derives the head from the journal instead,
which removes the disagreement rather than defining a winner for it.

`workspace` is a current selection like `selected_model`, not presentation
metadata: it decides where the next turn's tools run and which workspace
`AGENTS.md` is appended to the system prompt. Losing it therefore degrades a
resumed session to the configured default root, which is visible to the user
and correctable, not silent.

If history is damaged, valid metadata may keep the session visible but cannot
make it resumable.

`selected_model` means what the next turn should use. `turn_started.model`
records what a completed or interrupted turn actually used. This distinction
preserves provenance without turning every UI setting change into conversation
history.

## 6. `history.jsonl`

### 6.1 Header

The first line is an immutable, self-identifying header:

```json
{"type":"history","version":1,"session_id":"<uuid>","created_at":"2026-08-23T10:00:00Z"}
```

The loader rejects an unsupported version before interpreting records. A
header session ID that disagrees with the directory or `meta.json` is
corruption, not an alternate spelling.

### 6.2 Physical and logical order

Every later line has both a physical sequence and logical history identity:

```json
{
  "seq": 42,
  "id": "<history item id>",
  "parent_id": "<parent history item id>",
  "ts": "2026-08-23T10:01:02.123456Z",
  "type": "message",
  "turn_id": "<run id>",
  "data": {}
}
```

| Field | Contract |
|---|---|
| `seq` | Physical append order starting at 1; also the optimistic-concurrency token in §14's `Append` |
| `id` | Stable identity of this history item |
| `parent_id` | Previous logical item on this branch; omitted only for the root |
| `ts` | UTC RFC 3339 diagnostic instant; never the ordering key |
| `type` | Closed discriminator for resumable-history semantics |
| `required` | Whether a reader that cannot interpret `type` must refuse the session rather than skip the record; see §6.4 |
| `turn_id` | Optional correlation to the run and its trace |
| `data` | Type-specific payload with explicit `snake_case` fields |

Physical order stays append-only after rewind. Logical order is the parent
chain ending at the current head:

```text
physical append: A B C D E F

logical history:
  A -> B -> C -> D
       \-> E -> F   (current head)
```

This keeps abandoned branches available without truncating or rewriting the
journal.

The head is derived from the tail of the journal, not stored beside it:

- with no `head_selected` record, the head is the last physical record;
- otherwise it is the last physical record after the final `head_selected`;
- if nothing follows that record, the head is the item it names.

The rule is exact because a branch switch inside one journal is always marked.
Rewind writes `head_selected` (§8.2) and fork writes a separate session
directory (§8.3), so no record can be appended to an abandoned branch without a
`head_selected` in front of it. Reading the head therefore costs a scan back
from the tail, not a second file that could disagree.

### 6.3 Record vocabulary

| Type | Payload | Resume effect |
|---|---|---|
| `turn_started` | Run ID, effective model, workspace root, context window, input kind | Opens a turn and fixes its runtime identity |
| `message` | One complete `llm.Message` | Adds user or assistant provider history |
| `tool_execution_started` | Tool-call ID and tool name | Marks that an approved call crossed the side-effect boundary |
| `tool_result` | Tool-call ID, status, full text, content parts or artifact refs | Projects one tool-role message and closes the call |
| `compaction` | Covered head/range, full accumulated summary | Replaces the model-visible prefix on this branch |
| `notes_replaced` | Complete stamped note list | Replaces durable notes |
| `todos_replaced` | Complete stamped todo list | Replaces durable todos |
| `additional_prompt_set` | Complete validated text | Replaces the durable additional system prompt |
| `head_selected` | Target history item and reason | Selects the branch later records extend |
| `checkpoint` | History head and state digest | Names a stable conversation restore point |
| `turn_finished` | Terminal status and optional error classification | Closes the turn |
| `turn_recovered` | Interrupted turn and uncertain tool-call IDs | Makes cold recovery explicit before new work |

`message` preserves the full portable `llm.Message`, not only text. An
assistant record therefore includes tool calls and opaque provider state. A
user record preserves background provenance and content parts.

`tool_result` is stored as its own typed history item rather than duplicated in
a separate generic message. The reducer projects it to the `role: tool`
message required by provider adapters.

Title, pin, workspace, selected-model preference, usage totals, plugins,
sandbox policy, and timing are deliberately absent. They belong to metadata or
run diagnostics rather than the conversation journal.

### 6.4 Provider neutrality and forward compatibility

Two properties matter more here than they would in a single-vendor transcript,
because BuildMax targets many providers and expects to grow features this
document does not foresee.

**Neutral about the producing protocol.** History stores the portable
`llm.Message`, never a provider's wire payload. Roles, tool calls, tool
results, content parts, and the turn lifecycle are BuildMax's own vocabulary,
so a journal written while talking to one provider replays against another.

The one provider-shaped value a record carries is `ProviderState`, and it is
tagged with the protocol that produced it precisely so a reader can tell
whether it still applies. Resuming under a different protocol — a switched
model, a changed gateway, a re-pointed config — drops provider state whose tag
does not match the active adapter instead of forwarding it. Dropping it loses
cached reasoning, which is a degradation; forwarding it would send one vendor's
opaque bytes to another vendor's endpoint, which is a defect. A session whose
model changed mid-conversation therefore holds mixed-protocol messages, and
each is judged on its own tag rather than on a session-wide assumption.

**Extensible without breaking older readers.** `type` is a closed discriminator
for the semantics defined here, but the format still has to admit records this
version has never seen: a later feature, or a kind contributed by a plugin.
Every record therefore declares whether a reader that does not understand it
may skip it.

- A record that only adds information is skippable. An older binary logs it and
  continues.
- A record that changes what the model sees is required. A reader that cannot
  interpret it must refuse the session rather than resume a conversation it is
  silently mis-reducing.

A concrete case shows why the bit is decided per record rather than per format
version. Today a run cannot move between directories: Bash executes every call
at the workspace root, and file tools reject paths that escape it, so no record
is needed to say where a tool ran. If the runtime later gains a persistent
shell working directory, a change to it is model-visible and would arrive as a
required record — and older binaries would correctly refuse a session they
cannot place, instead of replaying it against the wrong tree.

That one bit is what lets §7.2 stay strict without turning the format into a
version trap. Unknown-and-skippable is a warning; unknown-and-required is
treated exactly like corruption, because in both cases the reduced history
would be wrong. Extension is therefore a decision about whether a new record
is model-visible, which is a question this design already asks of every record
type in §6.3.

## 7. Commit And Recovery Semantics

### 7.1 Turn commit boundaries

```text
accept prompt
  append + sync turn_started
  append + sync user message

call model
  append + sync assistant message

for each approved tool call
  append + sync tool_execution_started
  execute external effect
  append + sync tool_result

before next model request
  verify the current history prefix is durable

finish turn
  append + sync state changes and turn_finished
  atomically refresh meta.json and index.json
```

An item affecting resume must not become visible to Agent Core until history
accepted it. On persistence failure the turn stops. Session persistence cannot
use the run trace's fail-open behavior.

Batching is allowed only across boundaries that do not change failure
classification. `tool_execution_started` reaches stable storage before a tool
may change the world. Its result reaches stable storage before another model
request consumes it.

The strength of that guarantee is exactly the strength of the sync beneath it,
so the backend states which primitive it uses per platform. On macOS `fsync(2)`
returns once the write reaches the drive's cache, not the platter; only
`F_FULLFSYNC` waits for the device, and it costs tens of milliseconds. With a
cache-only flush a `tool_execution_started` record can be lost after the tool
already ran, and recovery would then classify as `not_started` work that
changed the world. Process crash and power loss are different guarantees and
this design does not conflate them: the classification in §7.3 is sound under
process and machine crash with an honest sync, and degrades to best-effort
under power loss on hardware that lies.

That choice is a latency budget as much as a correctness one. This protocol
syncs at least five times per turn plus twice per approved tool call, so a turn
with ten parallel tool calls crosses roughly twenty-five sync points on the
interactive path. Phase 1 therefore measures committed-turn latency and sync
count alongside the replay numbers in §13. Measuring only replay would tune the
path a user waits for once and ignore the one they wait for every turn.

### 7.2 Torn tail and corruption

The loader validates the header, schema, unique IDs, parent references, and
record relationships. An incomplete final line is a torn tail and may be
truncated to the last newline while acquiring the writer role. Invalid JSON, an
unknown required type, or an invalid relationship before the tail is corruption
and fails load with a path and sequence number.

A gap in `seq` is not on that list. With one writer and an append-only file
only the tail can tear, so a gap means either that something outside BuildMax
truncated or edited the journal — §3.2 puts tamper-evidence out of scope — or
that a record is missing, which parent-chain validation already rejects
wherever it could change the reduction. A gap confined to an abandoned branch
changes nothing the model sees. A second signal for a failure the first one
already reports buys strictness rather than safety, so `seq` stays an ordering
and concurrency device and is not a corruption check.

Read-only inspection never repairs files. Opening for continuation repairs the
torn tail and appends recovery state before accepting a new prompt.

Refusing to continue is not the same as offering no way out. A corrupt record
before the tail fails the load for continuation, and the loader also exposes a
read-only salvage mode that reduces the valid prefix up to the first bad record
and exports it as a new session directory. The damaged original is never
rewritten in place. A local session that can only be reported as broken, with
no path to the work inside it, is a worse outcome than a conservative refusal
plus an explicit recovery command.

### 7.3 Unknown tool outcome

An assistant tool call proves the model requested work. A
`tool_execution_started` record proves BuildMax approved the call and was about
to cross into the tool. A `tool_result` records the observed outcome.

| Durable history | Recovery classification |
|---|---|
| Assistant call only | `not_started` |
| Assistant call plus `tool_execution_started` | `outcome_unknown` |
| Matching `tool_result` | Known `completed`, `failed`, `denied`, or recovery-written `unknown` outcome |

`outcome_unknown` is never retried automatically. Recovery tells the model to
verify possible effects before retrying anything non-idempotent.

That repair is a history write, not a reduce-time fiction. Opening an
interrupted session appends one `turn_recovered` record naming the interrupted
turn and its uncertain tool-call IDs, followed by one `tool_result` with status
`unknown` for each of them. The synthetic result the model sees is therefore
itself durable and appears exactly once: a second interrupted open extends the
branch instead of replaying the same repair, and no reader has to know which
tool results were observed and which were reconstructed, because the record
says so.

The journal records the Agent's observed state. It cannot prove the filesystem,
a shell process, an HTTP endpoint, or a remote database matches that state.

## 8. Checkpoint, Rewind, And Fork

### 8.1 Checkpoint

A checkpoint names a stable conversation head:

```json
{
  "seq": 85,
  "id": "<checkpoint item id>",
  "parent_id": "<history item id>",
  "type": "checkpoint",
  "data": {
    "history_head_id": "<history item id>",
    "state_digest": "sha256:...",
    "reason": "user_prompt"
  }
}
```

`state_digest` covers the conversation state reduced at that head, so a restore
can be checked against what the checkpoint claimed rather than trusted.

**A checkpoint restores conversation, and only conversation.** It does not
capture the workspace, and this design does not propose a mechanism that would.
BuildMax has no versioned workspace capability and no design record for one:
the earlier record was withdrawn rather than implemented, as
[`trust-harness.md`](trust-harness.md) and the session-tree proposal both state.
A workspace snapshot field here would describe a withdrawn capability as
upcoming.

The consequence deserves stating rather than leaving to inference. Rewinding
past a turn that edited files, ran Bash, or called a network service leaves
every one of those effects in place: the model's history returns to an earlier
point and the world does not. A user who rewinds after a destructive edit gets
a conversation that no longer mentions it and a tree that still contains it, so
the surfaces offering rewind have to say what it does not undo.

If a versioned workspace capability is ever taken up, a checkpoint gains a
reference to one under §11's ordering rule — artifact bytes durable before
the record referencing them — and §6.4 decides whether older readers may skip
the extended record. That is an extension point, not a plan.

### 8.2 Rewind

Rewind appends a `head_selected` record naming the target checkpoint or
message. That is the entire operation. Because the head is derived rather than
stored, there is no second write to keep in step and no window in which a crash
could leave the selection half-applied. The old tail remains in
`history.jsonl`, and a new prompt appended after rewind names the selected head
as its parent, creating a new logical branch without truncation.

Rewind moves the conversation and nothing else. It does not restore the
workspace (§8.1), and it does not change the workspace root: that root is a
current selection in `meta.json`, so rewinding to a turn that ran somewhere
else leaves the next turn where the user last put it. The `turn_started` record
of the rewound-to turn still says which root it used, which is what makes the
difference inspectable rather than silent.

Compaction is branch-scoped. A summary records exactly which history head or
range it covers. A summary produced after the fork point cannot be reused by a
child that does not contain the summarized records. Raw history remains until
all checkpoint, rewind, and fork retention rules permit physical compaction.

### 8.3 Fork

Phase 1 physically copies the stable history prefix through the selected
checkpoint into a new session directory. It preserves item IDs, gives the child
an independent session ID and metadata record, and writes fork provenance into
`meta.json`.

Physical copying is O(n), but it gives the Alpha implementation clear
ownership:

- parent deletion cannot break the child;
- loading never walks another session;
- authorization and retention stay session-local; and
- no reference-counted history graph is required.

Artifacts referenced by the prefix are physically copied, for the same
ownership reason as the journal prefix. Because §11 keeps ordinary content
inline in phase 1, `artifacts/` holds only oversized and binary parts by then,
so the copy is bounded by what a session actually pasted or captured rather
than by how long it ran. A content-addressed store with reference counting is a
phase 3 option, taken only if measured fork cost justifies the
garbage-collection complexity it adds. Under either scheme a child must never
point into a deletable parent-owned artifact directory.

Shared prefixes, copy-on-write segments, and database-backed history graphs are
deferred until measured storage pressure justifies their deletion, retention,
and garbage-collection complexity.

Traces are not copied into the child resume context. Fork metadata names the
source checkpoint; new child runs create new trace files.

## 9. Subagent Sessions

Every foreground, background, or nested subagent gets the same directory shape
and its own history writer. It never writes into the parent's journal or state
store.

Subagent sessions are `kind: subagent`, hidden from the ordinary picker, and
excluded from `--continue`. Their header and metadata record the immediate
parent session, run, tool call, Agent type, and delegation depth. The parent
Task result remains the model-facing return path.

The private journal removes the current behavior where a subagent's entire
`session.Session` is discarded on return. It does not turn a current one-shot
subagent into a user-visible fork. A parent crash does not automatically restart
the Task tool: parent history classifies the call, while the child journal says
whether the child itself reached a terminal turn.

A sealed child is retained with its parent under the chosen retention policy.
Persistent supervision, mailboxes, automatic parent resume, user-visible child
navigation, and workspace isolation remain decisions in the session-tree
proposal.

## 10. Traces

The existing run-trace contract remains unchanged except for physical
co-location:

```text
<session_id>/traces/<run_id>.jsonl
```

One file per run preserves independent caps, terminal `run_end`, retention,
upload, and parent/child trace correlation. A single `trace.jsonl` for an entire
session is rejected because concurrent child or background runs, per-run
cleanup, and corruption would share one unbounded append target.

Where a tool ran belongs here too. A run cannot move between directories
(§6.4), but a single Bash command can still `cd` inside its own invocation or
name an absolute path, so the directory a call actually touched is operational
evidence rather than resume input. Recording it per call in history would stamp
mutable per-call context onto every record, which is the practice §16.6
rejects; the trace is bounded and already the place for it.

The trace stays bounded, redacted, and fail-open. History stays lossless for
resume and fail-closed at commit boundaries. A trace may summarize a history
item ID for correlation, but a resume path never reads trace content as state.

## 11. Artifacts

History initially stores ordinary text and structured results inline, matching
current exact-resume behavior. Content over a configured threshold and binary
parts move to `artifacts/` with media type, byte length, and digest in the
history reference.

Artifact bytes are synced before the referencing history item. An unreferenced
artifact may be collected; a committed history item must not point at missing
bytes. Fork and deletion operate on an explicit reference manifest rather than
discovering ownership from arbitrary paths.

Session content is not redacted because redaction would change what the resumed
model sees. Session directories use private permissions (`0700` directories and
`0600` files where supported). Credentials and approval grants never enter
metadata, history, traces, or artifacts as session state.

## 12. Index, Locking, And Lifecycle

`sessions/index.json` is the picker projection. It contains user-visible
sessions only, with ID, title, workspace, pin, kind, update time, and enough
lineage to group forks. It is written through temporary-file sync and atomic
rename, and rebuilt by scanning `meta.json` files when stale or damaged.

Listing reads `index.json` and does not scan. The projection exists so that
opening the picker costs one small file instead of one `meta.json` per session,
and that saving disappears if a stale projection quietly falls back to walking
directories. Rebuilding is therefore an explicit repair — triggered when the
index is missing, unparsable, or fails its own consistency check — and it is
the only path that walks session directories.

The session journal has one writer. The existing in-process turn guard remains,
and the file backend adds a cross-process lock. `writer.lock` may contain PID,
process start time, or owner diagnostics, but those values are not authority:
the OS lock decides ownership, and an abandoned file cannot keep a session
permanently busy.

Read-only inspection may read a stable prefix while a writer is active. A
second process cannot resume, rewind, fork from an unstable head, or append
until it owns the writer lock.

Deleting a session removes its directory only after resolving retained child
and artifact policy. Because this is a material destructive operation, the
caller identifies the exact session and surfaces whether child sessions or
shared artifacts remain.

## 13. Journal Growth And Physical Compaction

Phase 1 replays `history.jsonl` on open and records replay duration, record
count, and bytes.

Those measurements have a confounder worth naming. Replay parses the whole
journal to find the live branch, so its cost tracks total bytes on disk —
abandoned branches and full inline tool results included — not the length of
the conversation the model actually sees. A rewind-heavy session can therefore
open slowly for reasons that have nothing to do with how long its conversation
is. Phase 1 records live-branch records and bytes separately from journal
totals, and warns when a single journal crosses a configured size, so growth
becomes visible before it becomes a complaint.

Physical pruning is what would bound those bytes, and it does not ship here. A
prefix cannot be removed while a branch, checkpoint, fork export, or remote
revision depends on it. No pruning ships until reachability and recovery are
specified and tested.

## 14. Ownership And Interfaces

```text
internal/core/session
  history item types, validation, linked-history reducer, recovery analysis

internal/agentapp
  SessionManager, commit orchestration, metadata/index policy, lifecycle

internal/infra/sessionstore
  JSONL codec, atomic metadata files, locking, append/sync, tail repair
```

`internal/core/agent` continues to depend on history and durable-state
interfaces. It does not import files, JSONL, databases, or Server clients.
The persistence seam expresses session semantics rather than paths:

```go
type Store interface {
    Create(ctx context.Context, meta session.Meta) error
    Append(ctx context.Context, id string, expectedSeq uint64, items ...session.Item) error
    Load(ctx context.Context, id string, mode LoadMode) (session.Loaded, error)
    UpdateMeta(ctx context.Context, id string, update session.MetaUpdate) error
    List(ctx context.Context, includeHidden bool) ([]session.ItemSummary, error)
}
```

`expectedSeq` is the writer's own view of the tail: learned when it opened the
session and advanced by its own appends. It is never read back from metadata,
which stores no sequence counter.

Exact names may change during implementation. The boundaries may not: core
owns meaning and deterministic reduction; AgentApp owns when state commits;
infra owns physical durability.

Mutation APIs must prevent callers from changing resumable exported fields
without appending history. The materialized `Session` may remain a read model,
but messages, compaction, notes, todos, and additional prompt changes go through
the committing session context.

## 15. Alpha Cutover

Existing `<session_id>.json`, `sessions.json`, and the separate trace layout are
not migrated. The new runtime does not read, import, or dual-write the old
format.

CLI, TUI, Desktop, eval, worker restore, run-global upload, task conversation
projection, rename, pin, delete, documentation, and tests change together. No
production path continues treating the whole-file session JSON as current
storage.

Old files may remain inert until explicitly removed. Their presence does not
make them visible in the new index or reserve an identity in the new store.

## 16. Alternatives

### 16.1 Atomic whole-file JSON

Useful as a stop-gap against partial overwrite. It still rewrites complete
history and loses in-flight tool state, stable item identity, and cheap fork
points.

### 16.2 One multiplexed JSONL file

A single append stream gives metadata, history, runtime state, and diagnostics
one total order and makes a session easy to copy. It also couples lossless
resume data to redacted fail-open traces, repeats mutable metadata, prevents
independent retention, and grows one schema and file without bound.

This is appropriate for a single-writer transcript product that values one-file
portability above policy separation. BuildMax already has distinct session,
trace, artifact, worker, and Portal contracts, so the directory bundle is the
better boundary.

### 16.3 Flat message-only JSONL

This removes whole-history rewrites and supports resume. Without stable item
IDs, parent links, tool execution boundaries, compaction, and durable memory,
it cannot safely implement interruption recovery, rewind, or fork.

### 16.4 SQLite for all local sessions

SQLite gives transactions, indexing, and concurrent readers. It also adds a
database schema and migration runtime to the single-binary local path before
query or concurrency evidence requires it. It would be a new dependency rather
than a reuse of an existing one: `internal/infra/db` is the server store and
speaks MySQL through GORM, so no local path links a SQLite driver today.

The linked-history domain contract stays backend-neutral so SQLite can be
qualified later.

### 16.5 Shared fork prefixes in phase 1

Reference sharing reduces copy cost but makes child load, parent deletion,
authorization, retention, and garbage collection interdependent. Physical
prefix copying is intentionally simpler for Alpha.

### 16.6 Field-level compatibility with existing JSONL agents

Matching another coding agent's record fields — so that a BuildMax journal
opens in its tools, or its sessions replay here — is rejected because it
points the wrong way for this project. BuildMax is an open, multi-provider
runtime.
Its stored form should stay neutral about which model or protocol produced a
turn, and stay free to grow records for features no vendor's format
anticipates. Adopting one vendor's schema trades both away: it couples an open
runtime to a format it does not control, inherits that format's churn, and
turns every future extension into a compatibility question about somebody
else's product. §6.4 is the property being protected.

Scope reinforces the same answer. Compatibility only earns its constraint when
files actually cross the boundary, and importing foreign transcripts is nowhere
in the roadmap.

The structural lessons are taken, and §1 says so: parent-linked records instead
of a flat array, one durable record per completed item, and a self-identifying
first line. Those are properties of a well-formed journal, not of anyone's
product. Two common practices are deliberately not taken. One is the single
multiplexed stream, rejected in §16.2. The other is stamping mutable
per-session context — working directory, branch, build version — onto every
record, which inflates the journal and creates as many answers to a question as
there are lines; §4 keeps that in `meta.json` and §6.2 keeps the record header
minimal.

The execution boundary in §7.3 has no counterpart in the formats this design
learned from. A record proving BuildMax was about to enter a tool, distinct
from the record proving the tool returned, is what makes `outcome_unknown`
expressible at all. That difference is the reason this is not a port.

## 17. Verification

Implementation is incomplete until tests establish:

- replay reconstructs messages, provider state, parts, compaction, notes,
  todos, additional prompt, and tool results exactly;
- logical parent validation rejects duplicate IDs, cycles, missing parents, and
  unsupported versions, and a journal whose `seq` skips a value on an abandoned
  branch still loads;
- torn final lines preserve every earlier committed record while earlier
  corruption fails load;
- failure before and after each tool boundary yields `not_started`, known
  outcome, or `outcome_unknown` as specified;
- parallel tool completion preserves committed history order and call IDs;
- rewind selects an earlier head without deleting the abandoned branch;
- fork copies exactly the selected stable prefix and survives parent deletion;
- compaction summaries never cross a branch they did not summarize;
- resuming under a different protocol drops provider state whose tag does not
  match the active adapter instead of forwarding it, and a history holding
  messages from more than one protocol still replays;
- an unknown skippable record loads with a warning and an unknown required
  record fails exactly like corruption;
- restoring a checkpoint reproduces exactly the reduced state its
  `state_digest` names, and leaves workspace files untouched;
- foreground, background, and nested subagents write isolated hidden bundles
  with immediate-parent lineage;
- hidden subagents never appear in the picker or become `--continue` targets;
- a second writer cannot mutate a live session;
- stale metadata and global indexes repair without changing history;
- a rewind followed by metadata loss resumes the selected branch rather than
  the abandoned physical tail, with no metadata field needing repair for it to;
- head derivation returns the same item as a full branch walk across journals
  with no rewind, one rewind, and repeated rewinds to different depths;
- a session whose runs used different workspace roots records the root each
  turn ran under, and replay does not restate earlier turns against the
  current root;
- salvage export recovers the valid prefix of a corrupt journal without
  modifying the original file;
- repeated interrupted opens append one recovery record per interruption and
  never duplicate a synthetic `unknown` tool result;
- committed-turn latency and sync count are recorded for a representative turn,
  including one with parallel tool calls, so the open-cost picture in §13 sits
  beside the cost paid on every turn;
- missing artifacts fail with an exact reference rather than silently dropping
  model-visible content;
- old Alpha files are ignored and no code path dual-writes them; and
- all tests use the isolated `BUILDMAX_HOME` supplied by `./make test`.

Property tests generate linked histories and compare incremental reduction with
full replay from every reachable head. Crash tests use real temporary files and
inject failure around file sync, metadata rename, tool execution, artifact
publication, checkpoint creation, and fork copying.

## 18. Delivery Phases

### Phase 0: Stop-gap atomic replacement

- Replace direct whole-session and index overwrite with temp-write, sync, and
  atomic rename if the final implementation does not land immediately.

`SessionManager` still writes both the session file and `sessions.json` with a
direct `os.WriteFile`, so this is an open gap rather than a hypothetical one.
It is small enough to land on its own and it retires problem 3 of §2 without
prejudging anything below.

### Phase 1: Session bundle and linked history

- Add metadata schema, history header/items, reducer, JSONL backend, writer
  lock, and tail repair.
- Route message, tool, compaction, durable state, and additional-prompt changes
  through the committing context.
- Move traces under the session bundle, one file per run.
- Give subagents isolated hidden bundles.
- Cut CLI, TUI, Desktop, eval, worker paths, docs, and tests to the new format
  together.

Phase 1 is a milestone, not one change, and it has three seams that land and
review independently. The core item types, the reducer, and their property
tests depend on no storage at all. The `internal/infra/sessionstore` codec,
locking, and tail repair are testable directly against temporary directories
with no agent in the picture. Only the third seam, the surface cutover, touches
user-facing behavior, and it stays atomic across CLI, TUI, Desktop, eval, and
worker paths because §15 rules out dual-writing. Nothing about that final
constraint requires the two layers beneath it to arrive in the same review.

### Phase 2: Checkpoint, rewind, and physical-copy fork

- Add stable conversation checkpoints.
- Add current-head selection and branch-aware replay.
- Add conversation rewind and independent child-session fork.
- Define artifact copy/retention rules for fork and deletion.

### Phase 3: Measured optimization and remote input

- Externalize oversized content and qualify artifact garbage collection.
- Feed immutable checkpoint bundles into durable Agent session work after that
  proposal is accepted.
- Evaluate SQLite or shared history segments only from measured query,
  concurrency, or storage pressure.

## 19. Open Questions

Grouped by the phase each one blocks, because only the first group has to be
answered before implementation starts.

### 19.1 Blocking phase 1

- Which cross-platform lock implementation covers CLI, Desktop, worker, and
  native Windows with the same single-writer guarantee?
- Which sync primitive each platform uses, and therefore whether §7.1 claims
  crash safety or power-loss safety.
- Should a graceful cancellation close a turn as `canceled` or remain a
  recoverable interruption? This fixes the meaning of `turn_finished`, so it
  cannot be deferred past the record vocabulary.

### 19.2 Blocking phase 2

- Which tools may declare an interrupted call safe for automatic retry, and how
  is that idempotency qualified?
- How long are hidden subagent bundles retained after the parent receives their
  result?
- How does a surface tell a user what a rewind did not undo, given §8.1 leaves
  workspace effects in place?

### 19.3 Blocking phase 3

- What size moves inline content into `artifacts/` without making ordinary tool
  resume dependent on excessive small files?
- What reachability rule makes a journal prefix safe to prune, given branches,
  checkpoints, and fork exports? Until it exists, §13 bounds nothing.

### 19.4 Resolved

Which completed content unit streams to history — a provider response item, a
portable `llm.Message`, or both through one adapter boundary — is already
answered by the existing type. `llm.Message` carries `ProviderState`, a
`{protocol, data}` pair holding the opaque reasoning state the producing
protocol requires back verbatim, alongside `Parts` for non-text content and
`Source` for background provenance. It round-trips a resume losslessly without
a second stored representation.

Journals that persist raw provider items do so because their portable message
type cannot carry opaque protocol state. That is not the situation here, and
storing wire payloads would also couple the journal to one vendor's format —
the neutrality §6.4 exists to protect. History stores the portable message, and
§6.3 needs no parallel vocabulary.
