# Local Session Storage

> **Audience:** contributors and security reviewers · **Status:** planned
>
> Related: [session architecture](../contribute/architecture/session.md),
> [sessions and traces](../guide/sessions-and-traces.md),
> [durable run trace](durable-run-trace.md),
> [context durability](context-durability.md),
> [versioned workspace](versioned-workspace.md),
> [durable Agent sessions](../proposals/durable-agent-sessions.md), and
> [session trees and mailboxes](../proposals/session-tree-and-agent-mailbox.md).

## 1. Decision

Replace each local session's mutable whole-file JSON with a session-owned
directory containing four records with different contracts:

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

Phase 1 has no `snapshot.json`. Replay cost is measured before adding a
materialized-state cache.

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

This shape has four problems:

1. Every turn rewrites all previous messages, tool results, provider state, and
   image data.
2. Direct overwrite can leave the only file unparsable after interruption,
   disk exhaustion, or machine failure.
3. A long Agent turn may complete many model and tool steps before the session
   is saved. A crash loses the distinction between work that never started and
   work that may already have changed the world.
4. A flat `Messages` array has no durable identity for a checkpoint, rewind, or
   fork point.

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
- Give foreground, background, and nested subagents private durable sessions
  without exposing them as ordinary user sessions.
- Preserve the single-binary local experience and keep Agent Core independent
  of storage implementations.

### 3.2 Non-goals

- Replaying or rolling back arbitrary external side effects.
- Treating a conversation checkpoint as a complete filesystem, process, remote
  service, or database snapshot.
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
| Title, pin, workspace | `meta.json` | Presentation metadata does not affect model context |
| Selected model | `meta.json` | Current choice for the next turn |
| Model actually used | `turn_started` history record | A fact about a specific turn |
| Token totals | `meta.json` | Local aggregate reporting; per-call evidence remains elsewhere |
| Messages and tool results | `history.jsonl` | Required to reconstruct provider history |
| Tool crossed execution boundary | `history.jsonl` | Required to classify interrupted effects safely |
| Compaction summary and boundary | `history.jsonl` | Changes the model-visible history on a particular branch |
| Notes, todos, additional prompt | `history.jsonl` | Durable state injected into later model requests |
| Current history head | `meta.json` | Selects the active branch; every candidate head remains in history |
| LLM/tool timing, sandbox, plugins, denial diagnostics | per-run trace | Bounded operational evidence, not resume input |
| Large or binary content | `artifacts/` plus a history reference | Independent size, integrity, and lifecycle controls |
| Active writer | OS lock plus `writer.lock` diagnostics | A process fact that must become stale automatically |

`meta.json` and `history.jsonl` intentionally do not have one transaction.
They do not need one authority: a stale title or index row must not make
conversation recovery incorrect. Writes that affect model-visible state go to
history first. Metadata and the global index follow as projections or current
settings.

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
  "current_head_id": "<history item id>",
  "last_seq": 84
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

Title, pin, workspace, and selected-model changes write a temporary file, sync
it, and atomically replace `meta.json`. They do not append conversation events.
If metadata is damaged, the session can still recover history with conservative
defaults. If history is damaged, valid metadata may keep the session visible
but cannot make it resumable.

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
| `seq` | Contiguous append order starting at 1; detects missing or duplicate physical records |
| `id` | Stable identity of this history item |
| `parent_id` | Previous logical item on this branch; omitted only for the root |
| `ts` | UTC RFC 3339 diagnostic instant; never the ordering key |
| `type` | Closed discriminator for resumable-history semantics |
| `turn_id` | Optional correlation to the run and its trace |
| `data` | Type-specific payload with explicit `snake_case` fields |

Physical order stays append-only after rewind. Logical order is the parent
chain ending at `meta.current_head_id`:

```text
physical append: A B C D E F

logical history:
  A -> B -> C -> D
       \-> E -> F   (current head)
```

This keeps abandoned branches available without truncating or rewriting the
journal.

### 6.3 Record vocabulary

| Type | Payload | Resume effect |
|---|---|---|
| `turn_started` | Run ID, effective model, context window, input kind | Opens a turn and fixes its runtime identity |
| `message` | One complete `llm.Message` | Adds user or assistant provider history |
| `tool_execution_started` | Tool-call ID and tool name | Marks that an approved call crossed the side-effect boundary |
| `tool_result` | Tool-call ID, status, full text, content parts or artifact refs | Projects one tool-role message and closes the call |
| `compaction` | Covered head/range, full accumulated summary | Replaces the model-visible prefix on this branch |
| `notes_replaced` | Complete stamped note list | Replaces durable notes |
| `todos_replaced` | Complete stamped todo list | Replaces durable todos |
| `additional_prompt_set` | Complete validated text | Replaces the durable additional system prompt |
| `checkpoint` | History head, workspace snapshot ref, state digest | Names a stable conversation/workspace restore point |
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

### 7.2 Torn tail and corruption

The loader validates the header, schema, contiguous `seq`, unique IDs, parent
references, and record relationships. An incomplete final line is a torn tail
and may be truncated to the last newline while acquiring the writer role.
Invalid JSON, an unknown required type, a gap, or an invalid relationship
before the tail is corruption and fails load with a path and sequence number.

Read-only inspection never repairs files. Opening for continuation repairs the
torn tail and appends recovery state before accepting a new prompt.

### 7.3 Unknown tool outcome

An assistant tool call proves the model requested work. A
`tool_execution_started` record proves BuildMax approved the call and was about
to cross into the tool. A `tool_result` records the observed outcome.

| Durable history | Recovery classification |
|---|---|
| Assistant call only | `not_started` |
| Assistant call plus `tool_execution_started` | `outcome_unknown` |
| Matching `tool_result` | Known `completed`, `failed`, or `denied` outcome |

`outcome_unknown` is never retried automatically. Recovery projects a tool
result telling the model to verify possible effects before retrying anything
non-idempotent.

The journal records the Agent's observed state. It cannot prove the filesystem,
a shell process, an HTTP endpoint, or a remote database matches that state.

## 8. Checkpoint, Rewind, And Fork

### 8.1 Checkpoint

A checkpoint binds a conversation head to an optional workspace snapshot:

```json
{
  "seq": 85,
  "id": "<checkpoint item id>",
  "parent_id": "<history item id>",
  "type": "checkpoint",
  "data": {
    "history_head_id": "<history item id>",
    "workspace_snapshot_id": "<artifact id>",
    "state_digest": "sha256:...",
    "reason": "user_prompt"
  }
}
```

The workspace artifact is durable before the checkpoint references it. A
conversation-only checkpoint omits `workspace_snapshot_id`. Restoring code and
restoring conversation remain two explicit operations; neither claims to undo
Bash, network, process, or external database effects that the workspace layer
did not capture.

### 8.2 Rewind

Rewind changes `meta.current_head_id` to a checkpoint or message. The old tail
remains in `history.jsonl`. A new prompt appended after rewind names the selected
head as its parent, creating a new logical branch without truncation.

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

Artifacts referenced by the prefix are physically copied initially or placed
behind a content-addressed store whose references survive parent deletion. A
child must never point into a deletable parent-owned artifact directory.

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

## 13. Snapshots And Physical Compaction

Phase 1 replays `history.jsonl` and records replay duration, record count, and
bytes. It does not create a second complete state file preemptively.

If measurements require a bound, a later `snapshot.json` contains the complete
materialized resume state, selected history head, last applied sequence, format
version, and digest of the covered journal prefix. At most one rolling snapshot
exists. It is a deletable cache and never replaces journal authority.

Snapshots are not written every turn. Rewriting a complete snapshot on every
turn while also appending turn-level JSONL would reproduce the current design
with duplicate storage.

Physical journal pruning is a separate decision from snapshotting. A prefix
cannot be removed while a branch, checkpoint, fork export, or remote revision
depends on it. No pruning ships until reachability and recovery are specified
and tested.

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
query or concurrency evidence requires it. The linked-history domain contract
stays backend-neutral so SQLite can be qualified later.

### 16.5 Shared fork prefixes in phase 1

Reference sharing reduces copy cost but makes child load, parent deletion,
authorization, retention, and garbage collection interdependent. Physical
prefix copying is intentionally simpler for Alpha.

## 17. Verification

Implementation is incomplete until tests establish:

- replay reconstructs messages, provider state, parts, compaction, notes,
  todos, additional prompt, and tool results exactly;
- physical sequence and logical parent validation reject gaps, duplicates,
  cycles, missing parents, and unsupported versions;
- torn final lines preserve every earlier committed record while earlier
  corruption fails load;
- failure before and after each tool boundary yields `not_started`, known
  outcome, or `outcome_unknown` as specified;
- parallel tool completion preserves committed history order and call IDs;
- rewind selects an earlier head without deleting the abandoned branch;
- fork copies exactly the selected stable prefix and survives parent deletion;
- compaction summaries never cross a branch they did not summarize;
- conversation-only, workspace-only, and combined checkpoint restore remain
  distinct operations;
- foreground, background, and nested subagents write isolated hidden bundles
  with immediate-parent lineage;
- hidden subagents never appear in the picker or become `--continue` targets;
- a second writer cannot mutate a live session;
- stale metadata and global indexes repair without changing history;
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

### Phase 1: Session bundle and linked history

- Add metadata schema, history header/items, reducer, JSONL backend, writer
  lock, and tail repair.
- Route message, tool, compaction, durable state, and additional-prompt changes
  through the committing context.
- Move traces under the session bundle, one file per run.
- Give subagents isolated hidden bundles.
- Cut CLI, TUI, Desktop, eval, worker paths, docs, and tests to the new format
  together.

### Phase 2: Checkpoint, rewind, and physical-copy fork

- Add stable checkpoints and optional workspace snapshot references.
- Add current-head selection and branch-aware replay.
- Add conversation rewind and independent child-session fork.
- Define artifact copy/retention rules for fork and deletion.

### Phase 3: Measured optimization and remote input

- Add a rolling snapshot only if replay measurements justify it.
- Externalize oversized content and qualify artifact garbage collection.
- Feed immutable checkpoint bundles into durable Agent session work after that
  proposal is accepted.
- Evaluate SQLite or shared history segments only from measured query,
  concurrency, or storage pressure.

## 19. Open Questions

- Which completed content unit should stream directly to history: a provider
  response item, a portable `llm.Message`, or both through one canonical
  adapter boundary?
- What size moves inline content into `artifacts/` without making ordinary tool
  resume dependent on excessive small files?
- Which cross-platform lock implementation covers CLI, Desktop, worker, and
  native Windows with the same single-writer guarantee?
- Which tools may declare an interrupted call safe for automatic retry, and how
  is that idempotency qualified?
- Which workspace backend can produce a checkpoint that covers direct tools and
  sandboxed Bash without claiming to capture external effects?
- How long are hidden subagent bundles retained after the parent receives their
  result?
- Should a graceful cancellation close a turn as `canceled` or remain a
  recoverable interruption?
- What measured replay duration, history bytes, and record count justify the
  first rolling snapshot?
