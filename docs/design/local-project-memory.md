# Local Projects And Project Memory

> **Audience:** contributors and security reviewers · **Status:** implemented
> through phase 2; phase 3 (Desktop viewer/editor, a CLI inspection command
> beyond `buildmax doctor`, and the usage evidence) is not built.
>
> Roadmap priority: P0.5 local follow-on. CLI/TUI and Desktop are in scope;
> Portal, worker task runs, and team memory are not.
>
> Two things landed differently from §9.1 and §11.5 and are worth naming. An
> unusable document — over the limit, or not valid text after a hand edit — is
> skipped for the run and reported by `buildmax doctor`, but the write tool
> stays registered: the tool registry is cached per model, so withdrawing it on
> a file that can change at any moment would be a guarantee only by luck, while
> the digest rule already refuses a replacement from a model that was never
> shown the bytes. And Desktop still keys its runtime cache by Project alone,
> because it opens a Project at its default workspace and so has exactly one
> root per Project; the wider keying §11.5 describes is needed only when Desktop
> can enter a worktree.

Related: [context durability](context-durability.md),
[local session storage](local-session-storage.md),
[workspace root and worktrees](workspace-root-and-worktrees.md),
[Agent Core trust harness](trust-harness.md),
[session architecture](../contribute/architecture/session.md),
[CLI architecture](../contribute/architecture/cli.md), and
[Desktop architecture](../contribute/architecture/desktop.md).

## Contents

- [1. Decision](#1-decision)
- [2. Why These Concepts Must Stay Separate](#2-why-these-concepts-must-stay-separate)
- [3. Current State And The Gap](#3-current-state-and-the-gap)
- [4. Lessons From Existing Local Layouts](#4-lessons-from-existing-local-layouts)
- [5. Goals And Non-Goals](#5-goals-and-non-goals)
- [6. Domain Model And Invariants](#6-domain-model-and-invariants)
- [7. Project Resolution](#7-project-resolution)
- [8. Storage](#8-storage)
- [9. Project Memory Contract](#9-project-memory-contract)
- [10. Model Context Composition](#10-model-context-composition)
- [11. Runtime Flows](#11-runtime-flows)
- [12. Visibility, Trace, And Diagnostics](#12-visibility-trace-and-diagnostics)
- [13. Privacy And Trust Boundary](#13-privacy-and-trust-boundary)
- [14. Ownership And Architecture](#14-ownership-and-architecture)
- [15. Deletion And Lifecycle](#15-deletion-and-lifecycle)
- [16. Alpha Cutover](#16-alpha-cutover)
- [17. Delivery Phases](#17-delivery-phases)
- [18. Acceptance](#18-acceptance)
- [19. Alternatives Rejected](#19-alternatives-rejected)
- [20. Open Questions](#20-open-questions)

## 1. Decision

BuildMax will add one shared local Project model for CLI/TUI and Desktop, and
Project will own the first cross-session memory scope.

The design makes three concepts deliberately separate:

| Concept | Meaning | Authority | Examples |
|---|---|---|---|
| Instructions | Protocol the Agent must follow | Normative | Runtime system prompt, global `AGENTS.md`, workspace `AGENTS.md`, additional system prompt |
| Memory | Agent-curated, fallible recall that may help future work | Descriptive | Session notes, todos, project preferences, project decisions |
| Session history | Ordered evidence of what happened in one task conversation | Historical | User and assistant messages, tool calls and results, lifecycle records |

A compaction summary is not promoted to Memory merely because it helps the
Agent remember. It is a lossy projection of Session History. The journal stays
authoritative, and the summary stays bound to the session and the branch it
summarizes.

The local hierarchy becomes:

```text
Local Project (stable identity and shared memory)
  ├── Workspace A (one actual execution root)
  │     └── Sessions that ran there
  ├── Workspace B (a worktree of the same Git repository)
  │     └── Sessions that ran there
  └── Project Memory (shared by those sessions)
```

Sessions remain physically under `<BUILDMAX_HOME>/sessions/`. Project
membership is a logical relationship carried by `project_id`; it is not a
reason to move session bundles under project directories.

For Git repositories, one Project covers the primary checkout and every linked
worktree that reports the same Git common directory. It does not automatically
combine independent clones or unrelated folders. For a non-Git directory, one
normalized root is one Project.

Project memory is stored as a bounded, user-readable `MEMORY.md` inside the
Project bundle. It is rendered as a lower-authority context block, not as a
system-prompt instruction layer. The primary Agent may replace it through a
dedicated `ProjectMemoryWrite` tool. Users can inspect, edit, clear, or disable
it. No automatic extraction pass ships in the first implementation.

## 2. Why These Concepts Must Stay Separate

The word "memory" has been carrying several incompatible meanings. That makes
storage decisions look simpler than they are and creates authority bugs: a
summary becomes an instruction, a user preference becomes an immutable rule,
or a transcript is treated as though it were a curated fact store.

The semantic class of content is determined by its contract, not by the role
or field used to transport it to a model. A compaction summary may currently be
appended to a system message for implementation reasons and still be derived
history. Project memory may be appended as a user-role context message and
still not be a user request. Transport cannot silently upgrade authority.

### 2.1 Instructions

Instructions say what the Agent must do. They are authored or selected by the
runtime or a person, not inferred from ordinary task experience.

The existing instruction layers remain:

1. Runtime prompt and capability-specific runtime layers.
2. `<BUILDMAX_HOME>/AGENTS.md`.
3. `<workspace>/AGENTS.md`.
4. The run's additional system prompt.

The two `AGENTS.md` files already provide the requested global and repository
layers. Project memory does not replace them and the Agent never writes either
one autonomously. When an observed preference should become mandatory policy,
the Agent may suggest an `AGENTS.md` change, but the user decides and performs
or authorizes that promotion.

### 2.2 Memory

Memory is information the Agent has selected from experience because it is
likely to improve later work. It is useful, not authoritative. It can be
incomplete, stale, or wrong, so it must be scoped, inspectable, editable, and
forgettable.

BuildMax has two memory lifetimes after this design:

| Lifetime | Existing or planned state | Purpose |
|---|---|---|
| Working/session memory | Existing notes and todos | Keep current decisions, constraints, and work state across trimming and compaction inside one session |
| Project memory | Planned `MEMORY.md` | Carry stable, project-specific knowledge across CLI/Desktop sessions and related Git worktrees |

Global user memory, team memory, and reusable Agent memory remain separate
future scopes. A global mandatory preference can already be an instruction in
`<BUILDMAX_HOME>/AGENTS.md`; that does not make it user Memory.

### 2.3 Session History

Session History is the ordered record needed to reconstruct a conversation:

- user and assistant messages;
- assistant tool calls and tool results;
- provider-owned message state and non-text parts;
- turn, recovery, rewind, and compaction records; and
- replacements of session-scoped notes and todos.

`history.jsonl` remains the authority. Project memory is not copied into every
session journal because doing so would create stale snapshots and make a shared
document look session-owned. A `ProjectMemoryWrite` call and its result remain
in the session journal as provenance for the mutation, while the resulting
shared document lives in the Project bundle.

### 2.4 Conflict Rules

When these sources disagree, BuildMax uses these semantic rules:

1. Instructions remain normative and Memory cannot override them.
2. A current explicit user correction supersedes stale Memory.
3. Session notes and todos describe the current task and normally outrank an
   older project recollection about that task.
4. Project Memory is supporting context, never proof that a fact is still
   true. The Agent verifies it when the workspace can answer directly.
5. A compaction summary is only a lossy view of prior history. Raw journal
   records win when a user or diagnostic surface inspects what happened.

These are meaning-level rules, not a new model-role hierarchy. The prompt text
must state them because provider protocols do not know BuildMax's taxonomy.

## 3. Current State And The Gap

### 3.1 What already works

BuildMax already has the session-local half:

- `NoteWrite` and `TodoWrite` replace bounded state persisted in the session
  journal;
- the state is re-rendered on every model call and survives compaction;
- a pre-compaction checkpoint gives the model a final chance to update it;
- compaction summaries accumulate and remain branch-scoped; and
- the four instruction layers are composed by `agentapp`.

The durable session shape is already sound:

```text
<BUILDMAX_HOME>/sessions/
  index.json
  <session_id>/
    meta.json
    history.jsonl
    traces/<run_id>.jsonl
    writer.lock
```

### 3.2 What is missing

There is nowhere for an Agent to retain a project-specific fact after the
session that learned it ends. Users can put such a fact in `AGENTS.md`, but
that turns descriptive recall into mandatory instruction and asks the Agent to
write a policy file it should not own.

Examples of suitable Project Memory are:

- the maintainer prefers narrow table-driven tests in this repository;
- an attempted library was rejected because it cannot preserve provider state;
- generated fixtures live outside the ordinary test-data directory for a
  reason that is not evident from the tree; or
- the same user correction has recurred across several sessions and is useful
  here, without being a protocol every Agent must obey.

Examples that do not belong there are:

- "never push to main" — an instruction;
- the exact conversation that reached a decision — Session History;
- "run the failing test next" — a todo;
- a compaction recap — derived Session History;
- a fact recoverable cheaply and unambiguously from current source; or
- a credential, access token, private key, or copied confidential payload.

### 3.3 Project and session identity are split today

Desktop currently owns a private `Project` record with a name and one folder,
stored in `<BUILDMAX_HOME>/projects/projects.json`. CLI does not resolve or
record that Project. Both surfaces store sessions in the one global sessions
directory, and `session.Meta` records a workspace path but no Project ID.

That causes visible inconsistencies already:

- CLI `--continue` chooses the newest session globally, not the newest session
  for the current repository;
- the TUI and Desktop session lists are global;
- Desktop clears project sessions by matching workspace path aliases; and
- a main checkout and its linked worktree look like different folders even
  though the existing Worktree capability treats them as one repository.

Adding memory to the current Desktop Project would make the split permanent:
Desktop and CLI could run against the same repository while reading different
memory or while CLI reads none. Shared Project identity therefore lands before
shared Project Memory.

## 4. Lessons From Existing Local Layouts

This section records observations, not compatibility targets. The local
layouts were inspected on 2026-08-29 and can change independently of BuildMax.

### 4.1 Claude-style path ownership

The inspected Claude layout groups session JSONL and a `memory/` directory
under an escaped absolute workspace path. Its strengths are simplicity and
obvious locality: browsing one directory shows the sessions and memory that
belong together.

The path is also the identity, which creates the weakness relevant here. The
same BuildMax repository appeared once for the primary checkout and again for
each worktree. Moving a checkout, opening it through a symlink, or using a
second related root splits the history and memory unless another layer repairs
the association.

BuildMax adopts the readable per-project memory bundle, not the escaped path as
identity.

### 4.2 Codex-style logical projects

The inspected Codex layout keeps physical session rollouts in a global dated
tree and records logical Project and root associations separately. This proves
the useful distinction: session placement and Project ownership do not have to
be the same hierarchy. Official Codex documentation likewise describes a CLI
project in terms of its starting directory and Desktop projects that can attach
folders, while local Memory is a separate store. See
[Projects](https://learn.chatgpt.com/docs/projects) and
[Memories](https://learn.chatgpt.com/docs/customization/memories).

BuildMax adopts logical ownership plus global session bundles. It does not
adopt arbitrary multi-root projects in v1: only roots demonstrably belonging
to one Git repository are grouped automatically.

## 5. Goals And Non-Goals

### 5.1 Goals

- Give CLI/TUI and Desktop one authoritative local Project identity.
- Make `--continue`, session lists, session clearing, and Desktop grouping use
  `project_id` instead of path coincidence.
- Share one bounded Project Memory across sessions and related Git worktrees.
- Keep the actual Workspace root explicit for tools, sandboxing, hooks,
  workspace configuration, diffs, and `AGENTS.md`.
- Make memory visible, editable, clearable, able to be disabled, and attributable to
  the session that last changed it.
- Prevent stale writes when two sessions update memory concurrently.
- Keep the CLI a Go-only single binary and reuse the existing JSON/Markdown
  local-storage approach.
- Record which instruction and memory sources a run loaded without copying
  sensitive content into traces.

### 5.2 Non-goals

- Portal, worker, team, organization, or shared server Project memory.
- Global user memory or automatic synchronization between machines.
- Arbitrary projects containing unrelated directories.
- Treating two independent clones as one Project based on a remote URL.
- Embeddings, a vector database, semantic search, or retrieval ranking.
- Automatic end-of-turn or background extraction on every conversation.
- Writing or rewriting `AGENTS.md` without explicit user authorization.
- Moving session bundles beneath Project directories.
- Importing Claude or Codex memory formats.
- A compatibility migration from the Alpha Desktop `projects.json` shape.

## 6. Domain Model And Invariants

### 6.1 Local Project

A Local Project is a stable opaque ID plus metadata that identifies one local
unit of work across sessions.

```go
type Project struct {
    ID               string
    Name             string
    Kind             Kind // git or directory
    DefaultWorkspace string
    GitCommonDir     string // git only
    CreatedAt        time.Time
    UpdatedAt        time.Time
    LastUsedAt       time.Time
}
```

The persisted shape uses explicit `snake_case` JSON tags and the repository's
existing opaque public-ID generator. Exact Go names may change during
implementation; these invariants may not:

- Project ID is stable and is the relationship key.
- Name is presentation metadata and never identity.
- A Git Project is identified locally by its canonical Git common directory.
- A directory Project is identified by one canonical absolute root.
- `DefaultWorkspace` is where a new Desktop run opens by default, not the set
  of directories the Project may contain.
- Project membership never widens a tool's allowed filesystem root. A run
  still has one Workspace at a time.

### 6.2 Workspace

Workspace remains the concrete directory a session currently executes in. It
decides:

- filesystem and Bash containment;
- the sandbox writable bind;
- workspace hooks, MCP configuration, skills, and subagent definitions;
- workspace `AGENTS.md`;
- Git diff and worktree operations; and
- the path recorded on `turn_started`.

Project Memory follows `project_id`; these facilities follow Workspace. A
worktree switch changes Workspace and derived workspace configuration while
leaving Project and its memory unchanged.

### 6.3 Session

`session.Meta` gains `project_id`:

```json
{
  "id": "<session uuid>",
  "project_id": "<local project id>",
  "workspace": "/repo/.buildmax/worktrees/memory-design"
}
```

For ordinary local CLI/Desktop sessions it is required at creation and does
not change later. A session may move among Workspace roots belonging to that
Project, but it does not silently move to another Project. A fork inherits the
Project ID. A local subagent inherits it for provenance and read-only memory
context.

The field remains optional at the serialization boundary because task-run and
other non-local sessions do not acquire a fake local Project merely to satisfy
the shape. CLI/Desktop constructors enforce the stronger local invariant.

### 6.4 One Project may cover several work directories

This is intentional but constrained:

```text
/work/buildmax                         ─┐
/work/buildmax/.buildmax/worktrees/a  ─┼─ same git common dir ─ one Project
/work/buildmax/.buildmax/worktrees/b  ─┘
```

Two unrelated directories are never joined automatically. Two clones with the
same remote are not joined either: remotes can change, forks can share a URL
shape, and credentials may appear in URLs. An explicit future multi-root
feature would need its own containment, UI, and deletion rules.

## 7. Project Resolution

Project resolution is one shared `agentapp` operation used by CLI and Desktop.
It receives a Workspace path and returns a Project plus the normalized
Workspace root.

### 7.1 Normalize the Workspace

The resolver:

1. makes the path absolute and clean;
2. resolves filesystem aliases where the current workspace machinery already
   does so;
3. verifies that it names an accessible directory; and
4. asks Git whether it belongs to a worktree.

This normalization is for lookup. The session still records the usable path
the runtime selected, and existing workspace-alias handling remains relevant
for old or user-entered spellings.

### 7.2 Git Project lookup

For a Git workspace, resolve both the worktree top level and the absolute Git
common directory. The common directory is shared by the primary checkout and
its linked worktrees, so it is the local repository key this feature needs.

Lookup order:

1. Find an existing Project whose canonical `git_common_dir` matches.
2. If found, touch `last_used_at` and use it.
3. If absent, create one with a new stable Project ID, a name derived from the
   worktree top-level directory, and this workspace as `default_workspace`.

The common-directory path is a locator, not a public identity. Moving an entire
repository can invalidate it. V1 handles that through an explicit relink when
the user opens the moved folder; it does not write an unsolicited marker into
`.git`, infer identity from remote URLs, or merge repositories speculatively.
The relink flow shows candidates with missing locators and requires the user to
choose; no heuristic silently joins memory domains.

### 7.3 Non-Git Project lookup

For a non-Git workspace, lookup uses the canonical absolute directory. A move
likewise requires explicit relink. Symlink aliases of the same existing root
must not create duplicates.

### 7.4 Resolution and corruption

An ambiguous match is an error, not an arbitrary choice. A damaged Project
index is rebuilt from Project metadata. A duplicate locator found during
rebuild is reported and neither Project is selected until repaired.

Automatic creation is safe because it creates only metadata under
`BUILDMAX_HOME`; it does not edit the repository. A failed Project write stops
new local session creation rather than launching a session with an identity it
cannot persist.

Resolution that may create or relink a Project takes the catalog writer lock,
repeats lookup after acquiring it, and only then writes. The second lookup is
what prevents CLI and Desktop processes starting in the same new repository at
the same time from creating two Project IDs.

## 8. Storage

The local layout becomes:

```text
<BUILDMAX_HOME>/
  projects/
    index.json
    writer.lock
    <project_id>/
      meta.json
      memory/
        MEMORY.md
        meta.json
        writer.lock

  sessions/
    index.json
    <session_id>/
      meta.json
      history.jsonl
      traces/
      writer.lock
```

### 8.1 Project metadata and index

`projects/<project_id>/meta.json` is authoritative for Project identity and
metadata. `projects/index.json` is a rebuildable picker and locator projection,
parallel to the session index. It contains enough information to resolve a
Workspace without opening every Project bundle.

Metadata files use private permissions and atomic replacement. The index is
rebuilt by scanning Project metadata when missing, invalid, or inconsistent.
It is never the only copy of Project identity. `projects/writer.lock` is an OS
advisory lock around create, relink, rename, and hard delete; ordinary lookup
reads a stable index without taking it. As with session locking, kernel
ownership decides liveness and an abandoned lock file is not authority.

### 8.2 Why sessions stay top-level

Nesting sessions beneath Projects was considered and rejected. It would make
the directory browser look tidy but would couple physical ownership to mutable
classification:

- a project relink or repair could require moving every session directory;
- global session IDs, locking, tracing, hidden subagent bundles, and index
  repair already work without a Project path;
- `--resume <id>` is naturally a global lookup; and
- future projectless or server-originated sessions do not need artificial
  parent directories.

Logical `project_id` gives filtering and ownership without those costs.

### 8.3 Memory files

`MEMORY.md` is the authoritative current document and is intentionally human
readable. `memory/meta.json` records:

```json
{
  "version": 1,
  "revision": 7,
  "digest": "sha256:<hex>",
  "updated_at": "2026-08-29T10:00:00Z",
  "updated_by_session_id": "<session uuid>",
  "updated_by_run_id": "<run id>"
}
```

The metadata describes the last BuildMax write; it is not a second copy of the
content. If a user edits `MEMORY.md` directly and its digest no longer matches,
the store accepts the Markdown as authority and reports a manual edit. The next
mutating open reconciles metadata under the writer lock before an Agent may
replace it. A read never rewrites the user's Markdown to match stale metadata.

There is no separate Project-memory revision journal in v1. The originating
session already records the tool call and result, and copying every complete
replacement into another append-only file would create an unbounded second
history before a retention need is observed.

## 9. Project Memory Contract

### 9.1 Content shape and budget

Project Memory is one Markdown document with a maximum of **8,192 Unicode
characters**. The limit is enforced on write, not by truncating silently at
render time. This matches the existing additional-system-prompt ceiling and
keeps the always-loaded context to roughly a few thousand tokens at worst.
If a direct filesystem edit makes the file invalid UTF-8 or over limit, the
runtime does not send a prefix that could change meaning: it disables that
source for the run, omits the write tool, and reports the exact repair needed
through diagnostics and the surface.

The recommended shape is a short heading and bullets grouped only when useful:

```markdown
# Project Memory

## Preferences
- Prefer narrow table-driven Go tests when several cases share one contract.

## Decisions
- Keep local session bundles top-level; Project membership is a logical ID.

## Known dead ends
- Do not key Project identity by escaped workspace path; worktrees split it.
```

The format has no required ontology. Headings are for readability, not a
schema the runtime interprets. Full-document replacement forces the Agent to
remove stale material instead of appending forever.

### 9.2 What the Agent may remember

The tool description and runtime prompt use a precision-first policy:

- remember stable project preferences, decisions and their reasons, recurring
  corrections, non-obvious conventions, and costly dead ends;
- do not copy facts that current files or commands can answer cheaply;
- do not store temporary task state, conversational narration, or raw tool
  output;
- do not turn observations into mandatory instructions;
- never store credentials or surprising sensitive content; and
- update or remove a memory when current evidence or the user contradicts it.

An explicit "remember this for this project" is a strong write signal. A fact
seen once in untrusted web or tool output is not. Repetition is evidence of
usefulness, not proof of truth.

### 9.3 Tool surface

The primary local Agent receives `ProjectMemoryWrite`. It accepts the complete
replacement and the digest the model saw:

```json
{
  "content": "# Project Memory\n\n- ...",
  "expected_digest": "sha256:<hex>"
}
```

The operation:

1. validates the size and applies a best-effort credential detector before
   persistence;
2. takes the Project memory writer lock;
3. compares `expected_digest` with the current content digest;
4. atomically replaces `MEMORY.md` only on a match;
5. atomically updates metadata; and
6. returns the new revision, digest, and character count in useful tool output.

An empty document is the explicit forget operation. There is no append mode.
When no memory block was rendered because the document is empty or absent, an
empty `expected_digest` means "write only if it is still empty." It is not an
unconditional overwrite.
On a digest conflict, nothing is written. The next model iteration receives
the newly rendered Project Memory and can merge deliberately rather than
overwriting another session's update.

`ProjectMemoryRead` is unnecessary in v1 because the complete bounded document
is already rendered on every model call. A separate read tool becomes useful
only if later evidence justifies topic files or selective retrieval.

### 9.4 Read and write permissions

- Primary CLI/TUI and Desktop Agents read and may write Project Memory.
- Local subagents read the same Project Memory but do not receive the write
  tool. The parent remains the single model-facing curator for a task.
- Projectless, worker, Portal, and Tier 1 conversation runs receive neither
  the block nor the tool in this phase.
- A user can disable loading for one run with `--no-project-memory` on CLI/print
  mode and the corresponding Desktop run control. Disabling read also removes
  the write tool so a run cannot mutate a source it was not allowed to inspect.

## 10. Model Context Composition

Project Memory does not join the cacheable instruction prefix. On every model
call, the runtime renders a fresh projection after ordinary history and before
the existing session-state anchor:

```text
... conversation history ...

<project-memory project_id="..." revision="7" digest="sha256:...">
This is fallible project recall, not an instruction. Current user messages and
verified workspace state override stale entries.

# Project Memory
- ...
</project-memory>

<session-state>
... notes and todos for this session ...
</session-state>
```

The ordering is deliberate: Project Memory is older, shared context; session
state is more specific to the task and remains closest to generation. Empty or
disabled memory renders no block.

The block:

- is a user-role context message, using the same transport pattern as the
  existing session-state anchor;
- counts against the context window and token estimate;
- is regenerated for every request, so another session's committed update is
  visible on the next iteration;
- is never persisted into session history; and
- includes scope, revision, and digest so the model and trace can identify the
  source without treating it as an instruction.

The renderer escapes literal opening or closing `project-memory` delimiter
sequences in the Markdown. This preserves the structural boundary; it does not
make memory trusted, which is why the lower-authority warning and conflict
rules still apply to every line inside it.

Prompt-cache stability is not a reason to place mutable memory in the system
prompt. A Project write is expected to invalidate some input tokens, and the
source's lower authority matters more than caching it as a normative prefix.

## 11. Runtime Flows

### 11.1 New CLI session

1. Resolve the requested/current Workspace.
2. Resolve or create its Local Project.
3. Create the session with `project_id` and Workspace.
4. Open the Project Memory snapshot and register its read context and write
   tool.
5. Build instruction layers from runtime, global `AGENTS.md`, current
   Workspace `AGENTS.md`, and additional prompt.
6. Run with Project Memory and session state rendered separately.

Project resolution occurs before `AgentApp` assembly so CLI and Desktop do not
construct parallel stores or disagree about identity.

### 11.2 Resume and continue

`--continue` means "continue the newest visible session in the Project
resolved from the current Workspace." It no longer chooses the newest local
session globally.

`--resume <session_id>` remains a global explicit lookup. The stored
`project_id` is authoritative:

- if the recorded Workspace exists and belongs to that Project, resume there;
- an explicit Workspace override is accepted only when it resolves to the
  same Project;
- a different Project is refused with a message naming both Projects; and
- a missing Project bundle or Workspace yields a visible detached-session
  recovery path, never silent reassignment.

The TUI session picker defaults to the current Project and offers an explicit
"all projects" view. Search can include Project name in the all-project view.

### 11.3 Worktree switch

The existing Worktree lifecycle already restricts a switch to the current Git
repository. After Project resolution lands, that check also proves the target
resolves to the same `project_id`. Workspace-derived hooks, skills, MCP,
`AGENTS.md`, sandbox, diff, and header follow the target. Project Memory does
not change.

### 11.4 Memory write

A normal Agent turn may call `ProjectMemoryWrite` whenever it identifies
durable project knowledge. The write commits before the tool returns. The
tool call and result are then committed to the session journal through the
ordinary tool boundary, giving the mutation a session and run provenance.

Unlike session notes, Project Memory does not get a mandatory pre-compaction
checkpoint in v1. Promoting session detail across every future session is a
higher-risk decision than preserving it inside the current session. A missed
memory costs convenience; a false or sensitive durable memory can mislead many
future runs. Usage evidence should decide whether a later extraction checkpoint
is worth that trade-off and model-call cost.

### 11.5 Desktop

Desktop stops owning a separate Project file and delegates list, create/open,
rename, and resolution to the shared Project manager. Opening a folder already
known through its Git common directory opens the existing Project rather than
creating a duplicate for the worktree.

The current runtime cache cannot assume one Project has exactly one folder.
Runtime instances are keyed by Project plus actual Workspace, or are scoped to
an open session. Project-level pending-message and one-run-at-a-time policy may
remain keyed by Project in the first phase; changing concurrency is not needed
to share identity or memory.

Desktop initially needs only a Project Memory viewer/editor and an enable
toggle. The editor uses the same digest-checked store as the Agent tool. It
must label memory as fallible recall and keep `AGENTS.md` in the instructions
surface rather than presenting both as one list.

## 12. Visibility, Trace, And Diagnostics

Every run trace gains source metadata without raw instruction or memory
content. This replaces the current `prompt_layers` record in the same cutover
rather than leaving two sources that describe the prompt differently:

```json
{
  "type": "context_sources",
  "project_id": "...",
  "workspace": "/repo/worktree",
  "instructions": [
    {"name": "runtime", "chars": 6200},
    {"name": "user_agents_md", "chars": 400},
    {"name": "workspace_agents_md", "chars": 900}
  ],
  "memory": [
    {"name": "project", "revision": 7, "digest": "sha256:...", "chars": 1200},
    {"name": "session_notes", "entries": 4},
    {"name": "session_todos", "entries": 3}
  ],
  "history_projection": {"compaction_present": true, "chars": 1800}
}
```

This replaces the current tendency to call every context source "memory". Raw
content stays out of the bounded trace because the session and Project stores
already own it and trace redaction is fail-open.

`buildmax doctor` should report, without content:

- the resolved Project ID and kind;
- the Workspace and, for Git, its common-directory locator;
- whether Project Memory exists, is enabled, and fits its budget;
- digest or metadata mismatch indicating a manual edit;
- duplicate or missing Project locators; and
- detached sessions referencing a missing Project.

## 13. Privacy And Trust Boundary

Project Memory is private local state but is still model-visible content.

- Project directories and files use `0700`/`0600` where supported.
- Memory is sent to the selected model on every call when enabled. The UI and
  user guide must say so plainly.
- The write tool refuses obvious secrets, but no detector proves content safe.
  The Agent contract remains "do not persist credentials or surprising
  sensitive information."
- External documents, tool output, repository files, and web pages cannot
  instruct the Agent to remember themselves. They are untrusted evidence; the
  Agent curates only verified facts useful to the Project.
- Memory content cannot grant tool permission, change sandbox policy, enable a
  plugin, or override hooks. Those controls remain outside model-authored
  state.
- Direct user edits are accepted as content but not silently reclassified as
  system instructions.

The user can clear memory without deleting sessions and can disable memory for
a run without deleting it. Clearing sessions does not clear Project Memory.

## 14. Ownership And Architecture

This is an explicit ownership change. The current architecture calls Desktop
Project local UI state; after implementation, Local Project becomes a shared
CLI/Desktop runtime concept.

| Responsibility | Owner |
|---|---|
| Local Project identity, validation, and Project Memory document contract | new pure `internal/core/localproject` capability |
| Atomic Project bundle, index, memory lock, digest, and permissions | new `internal/infra/localprojectstore` |
| Git worktree/common-directory discovery | `internal/infra/git` |
| Workspace-to-Project resolution and run wiring | `internal/agentapp` |
| Generic memory render/write seam used by the Agent loop | `internal/core/agent` interface, implemented by `agentapp` over the Project store |
| `ProjectMemoryWrite` and its LLM-facing contract | `internal/tool` |
| Project-scoped continue/list commands and TUI surfaces | `internal/interface/cli` |
| Folder picker and Project/Memory presentation | `internal/interface/desktop` |
| Session membership and index projection | `internal/core/session`, persisted by `internal/infra/sessionstore` |

No config or filesystem package enters `internal/core`. The Project store seam
lives beside its core model and is implemented by infra, following the session
storage boundary. `agentapp` remains the only runtime assembler shared by CLI
and Desktop.

When this lands, the architecture documents for Desktop, CLI, Session, Agent
Loop, and repository layout change in the same ownership-moving commit. The
old Desktop `Project`, readers, and writers are deleted rather than kept as a
compatibility layer.

## 15. Deletion And Lifecycle

Project presentation, memory, and sessions have separate destructive actions:

- removing a Project from Desktop navigation does not delete its sessions or
  memory; it is a presentation choice, not domain deletion;
- clearing Project Memory atomically replaces it with an empty document;
- clearing Project sessions selects by `project_id`, shows exact session IDs,
  and leaves memory untouched; and
- hard-deleting a Project bundle is offered only when no session references
  it, or after an explicit detach/delete decision names what will become
  unavailable.

There is no path-based cascading delete. A worktree removed from Git does not
delete the Project or its memory because other worktrees and sessions may
still depend on them.

The global session picker can still show a detached session. Resuming one does
not recreate or attach a Project silently; the user chooses a replacement
Workspace and confirms the Project association.

## 16. Alpha Cutover

The implementation cuts CLI and Desktop over together:

- create the shared Project store and resolver;
- add `project_id` to session metadata and its picker index;
- switch both surfaces and Project-session operations to the shared manager;
- add Project Memory context and tool;
- remove Desktop's private Project type and `projects.json` reader/writer; and
- update architecture, user documentation, traces, and tests in the same
  feature series.

No dual read or migration preserves the current Alpha Desktop
`projects/projects.json`. The new store may ignore that file. Local sessions
without `project_id` are likewise old Alpha data and do not become current
Project sessions by path inference during normal startup. If a one-shot
developer import helper proves useful while landing the change, it remains an
explicit command and is deleted before the format is treated as current.

## 17. Delivery Phases

### Phase 1 — shared Local Project identity

1. Add the core Project contract and JSON bundle store.
2. Add Git common-directory and non-Git root resolution.
3. Add `project_id` to local session metadata and the session index.
4. Make CLI new/continue/resume and TUI session listing Project-aware.
5. Move Desktop Project operations to the shared manager and delete the private
   store.
6. Keep sessions physically global and verify main checkout/worktree grouping.

Phase 1 changes no model context. It establishes the ownership boundary memory
needs and is independently testable.

### Phase 2 — bounded Project Memory

1. Add `MEMORY.md`, metadata, lock, digest, and atomic full replacement.
2. Add the optional Agent memory seam and render block.
3. Register `ProjectMemoryWrite` only on enabled local primary runs.
4. Give local subagents read-only context.
5. Add trace source metadata and diagnostic checks.
6. Document inspect/edit/clear/disable behavior for CLI/TUI and Desktop.

### Phase 3 — surface controls and evidence

1. Add the Desktop memory viewer/editor and TUI/CLI inspection command.
2. Measure size, write frequency, conflict rate, and how often users correct or
   clear memory without recording raw content.
3. Decide from evidence whether topic files, selective retrieval, or an
   automatic promotion checkpoint is justified.

Phases 1 and 2 are the feature. Phase 3 supplies the user-control acceptance
criteria and the evidence needed before making memory more automatic.

## 18. Acceptance

- Starting CLI in the primary checkout and a linked worktree resolves the same
  Project ID and Project Memory while preserving different Workspace roots.
- Two unrelated directories and two independent clones do not merge
  automatically.
- CLI `--continue` and the default session picker select only the current
  Project; an explicit all-project view and `--resume <id>` still work.
- Desktop and CLI opened on the same repository list the same Project sessions
  and read the same memory revision.
- A Project Memory written in one session appears on the next model iteration
  of another session in that Project.
- Concurrent stale replacement is refused by digest and loses neither writer's
  content silently.
- Project Memory never appears in the system-prompt instruction layers or the
  session journal as a copied context block.
- A memory cannot override `AGENTS.md`, tool permissions, hooks, sandbox, or a
  current user correction.
- Empty and disabled memory add no prompt tokens and expose no write tool.
- An over-limit or suspected-secret write fails with useful model-facing
  output and leaves the previous revision intact.
- Traces identify Project, Workspace, instruction layers, memory revision, and
  compaction presence without storing their raw text.
- Users can inspect, edit, clear, and disable Project Memory independently of
  sessions.
- Project/session deletion never cascades by path coincidence.

## 19. Alternatives Rejected

### 19.1 Put `memory/` under an escaped workspace path

Simple and browsable, but the path becomes identity. Worktrees, moves, and
aliases split one repository into several memory domains. Stable Project ID
plus a locator is the small extra layer that avoids this.

### 19.2 Move sessions under `projects/<id>/sessions/`

Physical nesting looks intuitive but provides no semantic capability that
`project_id` does not. It adds moves, cross-project resume complexity, and an
artificial home for projectless sessions while disrupting a durable store that
already has global IDs, locks, indexes, traces, and recovery.

### 19.3 Use `AGENTS.md` as Project Memory

Already available, but wrong in meaning and ownership. `AGENTS.md` is a
normative user/repository instruction source; memory is model-curated and
fallible. Letting the Agent rewrite it would silently convert experience into
policy.

### 19.4 Treat compaction summaries as long-term memory

A compaction summary is lossy, session-specific, and branch-specific. Promoting
it would carry task narration and stale conclusions across unrelated future
sessions while discarding the raw evidence needed to correct it.

### 19.5 Key Git Projects by remote URL or repository contents

Remote URLs change, can expose credentials, and do not distinguish clones;
content hashes change with commits and can collide across forks. Git common
directory is the exact local relation shared by linked worktrees and no wider.

### 19.6 Add embeddings and semantic retrieval now

One bounded document is cheap, inspectable, provider-neutral, and sufficient
to learn whether Project Memory is useful. Retrieval infrastructure adds
chunking, ranking, model compatibility, deletion, and explanation problems
before there is enough data to require it.

### 19.7 Automatically extract memory every turn

This adds a model call or hidden inference policy to every task, persists more
surprising content, and makes false memory common before user controls are
proven. Explicit Agent writes plus direct user edits are the safer first
instrumented path.

## 20. Open Questions

- Does an 8,192-character always-loaded document remain small enough in real
  repositories, or should the measured limit be lower?
- Do users want a manually triggered "review this session for project memory"
  action before they want automatic extraction?
- Is explicit relink sufficient after repository moves, or does usage justify
  an opt-in marker inside the Git common directory?
- Does one-run-at-a-time per Desktop Project remain the right concurrency rule
  once one Project can have several worktrees?
- When user-level Memory is designed, which project entries should be promoted
  rather than duplicated, and who authorizes that wider scope?

None blocks phases 1 or 2. Each has an observable signal and should be decided
from local use rather than by adding storage or automation speculatively.
