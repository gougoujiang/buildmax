# Local Projects And Project Memory

> **Audience:** contributors and security reviewers · **Status:** planned
>
> Roadmap priority: P0.5 local follow-on. CLI/TUI and Desktop are in scope;
> Portal, worker task runs, and team memory are not.

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

Project memory is stored as a set of small user-readable Markdown files inside
the Project bundle — one file per memory — with a generated `MEMORY.md`
index over them. Only the index is rendered on every model call, as a
lower-authority context block rather than a system-prompt instruction layer.
The Agent reads a memory's body when the index line suggests it is relevant,
and writes one memory at a time. Users can inspect, edit, delete, clear, or
disable them. No automatic extraction pass ships in the first implementation.

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
| Project memory | Planned per-Project memory files | Carry stable, project-specific knowledge across CLI/Desktop sessions and related Git worktrees |

Global user memory, team memory, and reusable Agent memory remain separate
future scopes. A global mandatory preference can already be an instruction in
`<BUILDMAX_HOME>/AGENTS.md`; that does not make it user Memory.

What separates the two is not scope but origin. A preference the user **stated**
is normative and belongs in an instruction layer, where the user authored it and
can see it. A pattern the Agent **inferred** from repeated interaction — "the
option lists I offer get cut to the first entry, so lead with the
recommendation" — is something the user never said and may not agree with. An
unconfirmed claim about a person cannot be written into a normative file, which
is why this class is Memory by construction rather than by placement. §9.2
bounds what may be inferred at all.

### 2.3 Session History

Session History is the ordered record needed to reconstruct a conversation:

- user and assistant messages;
- assistant tool calls and tool results;
- provider-owned message state and non-text parts;
- turn, recovery, rewind, and compaction records; and
- replacements of session-scoped notes and todos.

`history.jsonl` remains the authority. Project memory is not copied into every
session journal because doing so would create stale snapshots and make shared
documents look session-owned. A `MemoryWrite` call and its result remain
in the session journal as provenance for the mutation, and a body returned by
`MemoryRead` is an ordinary tool result there, while the memories
themselves live in the Project bundle.

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

Inside `memory/`, the arrangement is an index over one file per memory, not a
single document. The instance inspected held eleven memories totalling 14.4 kB
of bodies under a 1.5 kB `MEMORY.md` whose every line is a link plus a
one-clause hook. Each body carries YAML frontmatter — a slug, a one-line
description, a type, the session that wrote it, and a modification time — then
the fact, then **Why** it is believed and **How to apply** it. Bodies link to
each other with `[[slug]]`, and a link to a memory that does not exist yet is
tolerated as a marker rather than treated as an error. Several bodies name
their own source of truth and the date they were last verified against it,
which is how a memory that summarizes something expensive stays useful without
pretending to be current.

Two properties are what BuildMax takes from this. **The index is the part that
is always loaded; bodies are read on demand.** That is why the store can hold
ten times the always-resident budget without the resident cost growing. And
**one memory is one file**, so two sessions recording different facts never
contend, and a stale write can damage one memory rather than the whole store.

The path is also the identity, which creates the weakness relevant here. The
same BuildMax repository appeared once for the primary checkout and again for
each worktree: twenty-two worktree directories, each with its own session
JSONL. Only the primary checkout's directory held `memory/`, and a session
running in a worktree read that one. So the association is repaired for
memory and not for sessions — which is direct evidence for the split this
record proposes, and for §8.2's claim that physical placement and logical
ownership need not be the same hierarchy. Moving a checkout, opening it
through a symlink, or using a second related root still splits whatever the
repair layer does not cover.

BuildMax adopts the readable per-project memory bundle and its index-plus-files
shape, but not the escaped path as identity.

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
- Global user memory or automatic synchronization between machines (§20).
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
Project ID. A local subagent inherits it for provenance; it does not inherit
memory context, per §9.4.

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

Alias resolution means evaluating symlinks, not only cleaning the string.
`filepath.Clean` plus `filepath.Abs` — what `workspaceAliases` does today —
is not enough for the locator, because Git returns its own spelling of the
common directory: on macOS a workspace reached through `/var` and a common
directory reported under `/private/var` are the same directory and must not
produce two Project keys. Both sides of every comparison are resolved before
they are compared or stored.

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

Ordering decides whether that flow is ever reached. A moved repository misses
lookup, so step 3 has already created a second Project with empty memory
before the user could have asked for anything. Creation must therefore
announce itself when the catalog holds Projects whose locators no longer
resolve: the new Project is created — a run is never blocked on a naming
decision — and the surface says a new Project was created and that N existing
Projects have missing locators, naming the relink command. Without that, the
recovery path exists and is never found, and the duplicate looks like the
feature working.

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
        MEMORY.md      # generated index
        <slug>.md      # one file per memory
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

One memory is one file. `<slug>.md` is authoritative for that memory and is
intentionally human readable:

```text
projects/<project_id>/memory/
  MEMORY.md            # generated index
  writer.lock
  merge-commit.md
  rejected-sse-transport.md
  fixture-layout.md
```

The file name is the slug and the slug is the memory's identity — there is no
second identifier that can disagree with it. Each file is frontmatter plus
body:

```markdown
---
name: rejected-sse-transport
description: SSE was rejected for the event stream; it cannot resume mid-turn
type: project
session_id: <session uuid that last wrote it>
updated_at: 2026-08-29T10:00:00Z
verified_at: 2026-08-29
---

The event stream uses WebSocket, not SSE.

**Why:** SSE was tried in the worker-stream spike and dropped because a
reconnect cannot resume a turn already in flight.

**How to apply:** do not re-propose SSE for streaming without addressing
resume. Related: [[worker-stream-contract]].
```

Frontmatter fields use `snake_case`, matching the repository's persisted-JSON
convention. `session_id` and `updated_at` give each memory its own provenance,
which is why there is no sidecar metadata file: the previous single-document
design needed one because a lone Markdown file had nowhere to record who wrote
it, and per-file frontmatter removes that need rather than moving it.

`verified_at` is optional and is what lets a memory summarize something
expensive without claiming to be current; §9.2 says when to use it.

`MEMORY.md` is a **generated projection**, not a hand-maintained file: it is
rebuilt from the memory files after every write, exactly as
`projects/index.json` is rebuilt from Project metadata, and for the same
reason — an index that can disagree with its sources is a defect surface with
no compensating capability. Users edit memories, not the index. A memory file
deleted by hand simply loses its line on the next rebuild.

A body whose frontmatter is missing, unparseable, or over budget is reported
and skipped rather than guessed at. Skipping one memory is a smaller failure
than rendering a line that promises a body the read tool cannot return.

There is no Project-memory revision journal in v1. The originating session
already records the tool call and result, and copying every replacement into
another append-only file would create an unbounded second history before a
retention need is observed.

## 9. Project Memory Contract

### 9.1 Content shape and budget

A Project holds at most **20 memories**. Each has a `description` of at most
**100 characters** and a body of at most **2,000**. Every limit is enforced on
write, not by truncating silently at render time.

Two budgets are in play because two things cost differently.

**The index is the resident cost.** It is rendered after the message list on
every call, and trailing blocks are never a prefix of the next call's request,
so they are paid for in fresh input tokens on every iteration of every session
in the Project — not once per write. Bounding the inputs bounds it by
construction: twenty lines of a slug plus a 100-character description land
near 3,200 characters, which is deliberately the ceiling `RenderSessionState`
already applies to invariants, notes, and todos combined. The renderer also
enforces 3,200 directly, so a hand-edited store cannot exceed it even if the
per-memory limits were bypassed.

The comparable budget is that anchor, not the additional system prompt. The
additional-system-prompt ceiling of 8,192 bounds user-authored normative text
the user chose to pay for; Project Memory is model-authored fallible text
placed closer to generation than the system prompt, so it is held to the
standard of the other always-rendered block.

**Bodies are a retrieval cost, paid only when read.** 2,000 characters each is
generous precisely because it is not resident: a memory can afford the Why and
the How that make it actionable, instead of being compressed into a bullet
that survives the budget by dropping its reason. Twenty of them is 40 kB on
disk that no run ever loads at once.

This is the split that makes the shape work. A single always-loaded document
must delete an old memory to admit a new one; here, growth costs one index
line, and pressure falls on the count and on writing a description worth its
line rather than on the knowledge itself.

Twenty is a starting bound chosen to be raised on evidence rather than lowered
after users have filled it. Phase 3 measures observed counts and sizes.

If a direct filesystem edit makes a file invalid UTF-8 or over budget, that
memory is reported and skipped (§8.3). If the store as a whole cannot be read
— a missing directory is not an error, but an unreadable one is — the
runtime does not send a partial index that could mislead: it disables it for
the run, omits both tools, and reports the exact repair needed through
diagnostics and on the surface at run start. A source silently missing for a
whole session is the failure this reporting exists to prevent, so `doctor` is
not the only place it appears.

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

Every memory carries a `type`:

| Type | Holds |
|---|---|
| `feedback` | Guidance the user gave about how to work in this Project — corrections and confirmed approaches alike |
| `project` | Ongoing work, goals, decisions, and constraints not derivable from the code or its history |
| `reference` | Pointers to external resources: dashboards, tickets, specifications |

The fourth type in the inspected layout, a `user` scope holding who the user
is, is deliberately absent: that is global user memory, which §5.2 keeps out
of a Project-scoped store.

A `feedback` or `project` body states the fact, then **Why** it is believed —
when it came from, what it replaced — then **How to apply** it. That shape is
what separates a memory from a fact fragment: a reader who disagrees with the
Why can discard the memory on evidence instead of guessing at its warrant.

A `feedback` memory records what the user **wants**, never what the user
**is**. "Leads with the recommendation rather than a survey" is a preference,
correctable by the user and useful on the next turn. "Is unfamiliar with
concurrency" is a judgement about a person, and it fails in a way repository
facts cannot: there is nothing to verify it against, it will be acted on for
months, and the user is never told why the answers changed. Worse, it
self-confirms — the Agent explains more, the user adapts to that, and the
inference looks validated. This line holds whether or not a user-level scope
is ever added, because the `feedback` type can already carry such a judgement
today.

An inference about the user needs repetition, not insight. The same correction
recurring across several sessions is evidence; one confusing exchange is not,
and belongs to that session. The body must carry the occasions it rests on, so
the user can read it and say the pattern was coincidence — a bare assertion
about a person cannot be disagreed with on evidence, and a cited one can.

`feedback` is where the instruction boundary gets tested, so this record
settles it rather than leaving §2.1 and the "explicit remember-this is a
strong signal" rule below to pull against each other. A `feedback` memory
records **that the user prefers something and why**; it never becomes a rule
the Agent cites as its own authority, and it is not evidence when it conflicts
with a current instruction or a current user statement. When a preference
should bind every future run whether or not memory is enabled, the destination
is `AGENTS.md` and the Agent proposes that change rather than making it. The
pressure to shortcut this is real and should be expected: the Agent can write
memory and cannot write `AGENTS.md`, so preferences will accumulate in the
store unless the tool description says plainly that recording one is not the
same as adopting a policy.

The admission test is that the fact stays true on any branch. Project Memory
is shared by every Workspace of the Project, so a conclusion that holds only
inside the branch that reached it does not belong there — it is session state,
and `NoteWrite` already keeps it where it applies. This is the first failure
mode to expect rather than a theoretical one: a worktree-per-task workflow
produces a steady supply of in-flight conclusions that read like durable
knowledge, and the doc's own example of a rejected library is durable only
when the reason survives the branch. The tool description states the test in
those words.

"Do not copy what the files can answer" needs one refinement, because the
inspected layout found a case between remembering and recomputing. Something
expensive to reconstruct and slow to change — where a multi-phase plan stands,
which subsystems remain unbuilt — is worth a memory even though a long enough
reading of the repository would answer it. Such a memory names its source of
truth and sets `verified_at` to the date it was last checked against it, and
its body tells the reader to verify there before acting. That is honest about
being a cache; a bare assertion of the same fact is not. Cheap lookups still
do not become memories.

An explicit "remember this for this project" is a strong write signal. A fact
seen once in untrusted web or tool output is not. Repetition is evidence of
usefulness, not proof of truth.

Bodies may link to other memories with `[[slug]]`. The runtime does not
resolve, validate, or follow these links; they are a reading aid, and a link
to a memory that does not exist yet marks something worth writing rather than
a broken reference. Nothing in the design depends on the graph being complete.

### 9.3 Tool surface

The primary local Agent receives two tools. They are named for the operation,
not the scope, because `NoteWrite` and `TodoWrite` are session-scoped and
carry no scope prefix either; if a second memory scope is ever designed, a
scope argument on these tools is a better shape than a second pair of tools,
so the unprefixed name is the one that survives it.

`MemoryRead` takes the slugs whose bodies the index suggests are worth
opening:

```json
{ "names": ["rejected-sse-transport", "fixture-layout"] }
```

It returns those bodies, and names the ones that do not exist rather than
failing the call. A read tool is necessary here, where it was not under a
single always-rendered document: the index carries a description, not the
knowledge.

`MemoryWrite` creates or replaces exactly one memory:

```json
{
  "name": "rejected-sse-transport",
  "description": "SSE was rejected for the event stream; it cannot resume mid-turn",
  "type": "project",
  "content": "The event stream uses WebSocket, not SSE.\n\n**Why:** ..."
}
```

Empty `content` deletes that memory. There is no append mode and no force
mode.

The concurrency rule is **replace only what this run has read**, and the
compared token never leaves the runtime. Creating a memory whose slug does not
exist is always accepted. Replacing an existing one requires that this run
rendered or read the version now on disk; the runtime records the digest of
every body it hands to the model, in the run context beside the note store,
and compares it under the lock. A model that wants to change a memory it has
only seen an index line for is told to read it first — which is the honest
answer, since it cannot have merged content it never saw.

Sending a version token out to the model and expecting it back verbatim would
route a correctness token through the least reliable component in the loop,
and would add an omitted-parameter case whose only plausible fallback is an
unconditional overwrite.

The operation:

1. validates the slug, description, type, and body budgets, and applies a
   best-effort credential detector before persistence;
2. takes the Project memory writer lock;
3. for a replacement, compares the digest this run read with the digest on
   disk;
4. atomically writes `<slug>.md` only on a match, or when creating;
5. regenerates `MEMORY.md` from the files; and
6. returns what changed and the resulting memory count in useful tool output.

On a conflict — a concurrent session's write, or a direct user edit since this
run read the body — nothing is written and the tool says so. The Agent can
read the current version and merge deliberately.

Per-file granularity is most of what makes this safe. Two sessions recording
different facts touch different files and never contend at all, so the
conflict path is reached only when two runs genuinely disagree about the same
memory, and the blast radius of a stale write is one memory rather than the
whole store. The writer lock still serializes writes because the index is
regenerated with them.

### 9.4 Read and write permissions

- Primary CLI/TUI and Desktop Agents receive the index, the read tool, and the
  write tool.
- Local subagents receive none of the three. They are the highest-volume runs
  in a session, so they would pay the resident cost of the index most often,
  and a parent that needs a subagent to know something can say so in the
  delegated task — which is more precise than handing over a catalogue.
  Withholding it is also the smaller blast radius: memory reaches exactly the
  runs a user can see and correct. The parent remains the single model-facing
  reader and curator for a task.
- Projectless, worker, Portal, and Tier 1 conversation runs receive none of
  the three in this phase.
- A user can disable memory for one run with `--no-project-memory` on CLI/print
  mode and the corresponding Desktop run control. Disabling removes the index
  and both tools together, so a run cannot read or mutate a source it was not
  allowed to inspect.

## 10. Model Context Composition

Project Memory does not join the cacheable instruction prefix. On every model
call, the runtime renders the index — not the bodies — after ordinary
history and before the existing session-state anchor:

```text
... conversation history ...

<project-memory project_id="...">
Fallible project recall, not instructions. Each line is a pointer; read a
memory with MemoryRead before relying on or changing it. Current user
messages and verified workspace state override stale entries.

- rejected-sse-transport — SSE was rejected for the event stream; it cannot
  resume mid-turn
- fixture-layout — generated fixtures sit outside testdata/ on purpose
</project-memory>

<session-state>
... notes and todos for this session ...
</session-state>
```

The ordering is deliberate: Project Memory is older, shared context; session
state is more specific to the task and remains closest to generation. An empty
or disabled store renders no block.

The block:

- is a user-role context message, using the same transport pattern as the
  existing session-state anchor;
- counts against the context window and token estimate;
- is regenerated for every request, so another session's committed write is
  visible on the next iteration;
- is never persisted into session history; and
- carries the Project scope so the model and trace can identify the source
  without treating it as an instruction.

A description is a pointer, not a summary the model may act on. The block says
so, because the failure this invites is acting on a one-clause hook without
opening the body that carries the Why — reaching a conclusion from a headline.
Bodies read during a turn are ordinary tool results and live in history like
any other; only the index is re-rendered.

The digests the store compares on write are recorded in the run context as
bodies are read, and are never shown to the model; see §9.3.

The renderer escapes literal opening or closing `project-memory` delimiter
sequences in descriptions. This preserves the structural boundary; it does not
make memory trusted, which is why the lower-authority warning and conflict
rules still apply to every line inside it, and to every body the read tool
returns.

Prompt-cache stability is not a reason to place mutable memory in the system
prompt, and the caching cost is not what the choice turns on. Blocks rendered
after the message list are already outside every reusable prefix: history
grows in front of them on the next call, so a trailing block is paid for in
fresh input tokens whether or not it changed. A Project write therefore
invalidates nothing extra. The real cost is the constant per-call cost of
carrying the index at all, which is what §9.1 budgets, and the reason to keep
memory out of the instruction layers is its lower authority.

## 11. Runtime Flows

### 11.1 New CLI session

1. Resolve the requested/current Workspace.
2. Resolve or create its Local Project.
3. Create the session with `project_id` and Workspace.
4. Open the Project Memory store, and register the index projection with the
   read and write tools.
5. Build instruction layers from runtime, global `AGENTS.md`, current
   Workspace `AGENTS.md`, and additional prompt.
6. Run with Project Memory and session state rendered separately.

Project resolution occurs before `AgentApp` assembly so CLI and Desktop do not
construct parallel stores or disagree about identity.

### 11.2 Resume and continue

`--continue` means "continue the newest visible session **in the current
Workspace**." It no longer chooses the newest local session globally, and it
does not widen to the Project.

Project is the scope of shared memory. It is deliberately not the scope of
resume, because the two answer different questions: memory asks what is true
about this repository, and resume asks which conversation the user was just
having. Those coincide in a single checkout and diverge exactly where this
design adds worktrees. Continuing at Project scope would let `buildmax -c`
in worktree B pick a session recorded in worktree A and, by the resume rule
below, execute there — moving the user's working root out from under them, in
the one workflow whose entire purpose is branch isolation.

When the current Workspace has no session but the Project does, `--continue`
does not silently borrow one. It reports how many sessions the Project holds
in other Workspaces and names the flag that widens the search
(`--continue --project`), which then resumes in the recorded Workspace with
that root printed before the first turn. A root change is never implicit.

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
Because a Project may span Workspaces, the picker shows each session's
Workspace and marks the ones outside the current root; choosing one is an
explicit act and prints the root it will run in. The picker may cross
Workspaces because the user is looking at the list; `--continue` may not
because the user is not.

### 11.3 Worktree switch

The existing Worktree lifecycle already restricts a switch to the current Git
repository. After Project resolution lands, that check also proves the target
resolves to the same `project_id`. Workspace-derived hooks, skills, MCP,
`AGENTS.md`, sandbox, diff, and header follow the target. Project Memory does
not change.

### 11.4 Memory write

A normal Agent turn may call `MemoryWrite` whenever it identifies
durable project knowledge. Each call writes one memory. The write commits
before the tool returns. The tool call and result are then committed to the
session journal through the ordinary tool boundary, giving the mutation a
session and run provenance, which the memory's own `session_id` frontmatter
mirrors.

Replacing an existing memory requires having read it (§9.3), so the ordinary
shape of an update is a read followed by a write rather than a blind
overwrite. That is one extra tool call on the path that changes durable shared
state, and none on the path that adds a new one.

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

Desktop initially needs a list of memories — slug, description, type, and when
each was last written — an editor for one at a time, per-memory delete, and an
enable toggle. The editor uses the same digest-checked store as the Agent
tool. It must label memory as fallible recall and keep `AGENTS.md` in the
instructions surface rather than presenting both as one list; the `feedback`
type makes that separation easy to blur and §9.2 is what the surface follows.

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
    {"name": "project_index", "entries": 11, "chars": 1400}
  ],
  "history_projection": {"compaction_present": true, "chars": 1800}
}
```

The record is written once per run and describes what the run was assembled
from. Session notes and todos are deliberately absent: they change every
iteration, so a per-run count of them would report the value at run start
while looking like a fact about the run. The memory index belongs here because
what the run was assembled with is a property of the run. Which bodies were
read, and any write, are tool calls — already in the session journal with
their own timestamps, and not something the trace should describe a second
time.

This replaces the current tendency to call every context source "memory". Raw
content stays out of the bounded trace because the session and Project stores
already own it and trace redaction is fail-open.

`buildmax doctor` should report, without content:

- the resolved Project ID and kind;
- the Workspace and, for Git, its common-directory locator;
- how many memories exist, whether memory is enabled, and whether the index
  fits its budget;
- memories skipped for unparseable frontmatter or an over-budget body;
- duplicate or missing Project locators; and
- detached sessions referencing a missing Project.

## 13. Privacy And Trust Boundary

Project Memory is private local state but is still model-visible content.

- Project directories and files use `0700`/`0600` where supported.
- When enabled, the index — every memory's slug and description — is sent to
  the selected model on every call. A body is sent when the Agent reads it,
  which most turns will not do for most memories. The UI and user guide must
  say both parts plainly; "only the index is always sent" is a real reduction
  in what leaves the machine, and overstating it would be worse than not
  claiming it.
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
| Local Project identity, validation, and the Project Memory document contract | new pure `internal/core/localproject` capability |
| Atomic Project bundle, both indexes, memory lock, digests, and permissions | new `internal/infra/localprojectstore` |
| Git worktree/common-directory discovery | `internal/infra/git` |
| Workspace-to-Project resolution and run wiring | `internal/agentapp` |
| Generic memory render/write seam used by the Agent loop | `internal/core/agent` interface, implemented by `agentapp` over the Project store |
| `MemoryRead`/`MemoryWrite` and their LLM-facing contract | `internal/tool` |
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
- deleting one memory removes its file and its index line, and is offered
  separately from clearing the store;
- clearing Project Memory removes every memory file and leaves an empty index;
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
- add the Project Memory store, its index projection, and both tools;
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

1. Add the memory files, frontmatter parsing, lock, digests, atomic
   single-memory replacement, and the generated `MEMORY.md` index.
2. Add the optional Agent memory seam and the index render block.
3. Register `MemoryRead` and `MemoryWrite` only on enabled local
   primary runs, and render the index on those runs only — subagents receive
   neither tool nor block.
4. Add a user-invoked command that reviews the current session for material
   worth remembering and proposes a replacement the user accepts or discards.
   The trigger is a person, never a turn boundary or a background pass, which
   is what separates it from the automatic extraction §19.7 rejects. Its exact
   surface, output, and whether it writes directly or only proposes are
   settled during this phase.
5. Add trace source metadata and diagnostic checks.
6. Document inspect/edit/clear/disable behavior for CLI/TUI and Desktop.

### Phase 3 — surface controls and evidence

1. Add the Desktop memory list/editor and the TUI/CLI inspection command.
2. Measure memory count, body sizes, write frequency, how often a rendered
   index line is followed by a read, conflict rate, and how often users
   correct or delete memories — without recording raw content.
3. Decide from evidence whether the count bound should rise, whether the index
   needs grouping or ranking once it is full, and whether an automatic
   promotion checkpoint is justified.

Phases 1 and 2 are the feature. Phase 3 supplies the user-control acceptance
criteria and the evidence needed before making memory more automatic.

## 18. Acceptance

- Starting CLI in the primary checkout and a linked worktree resolves the same
  Project ID and Project Memory while preserving different Workspace roots.
- Two unrelated directories and two independent clones do not merge
  automatically.
- CLI `--continue` in a worktree selects a session recorded in that Workspace
  and never one recorded in a sibling Workspace of the same Project; widening
  is explicit and prints the root it will run in. The default session picker
  selects only the current Project; an explicit all-project view and
  `--resume <id>` still work.
- A Workspace reached through a symlinked path resolves to the same Project as
  the path Git reports, and creates no second Project.
- Opening a moved repository creates a new Project and, in the same output,
  reports the unresolved locator and the relink command.
- Desktop and CLI opened on the same repository list the same Project sessions
  and render the same memory index.
- A memory written in one session appears in the index rendered on the next
  model iteration of another session in that Project, and its body is
  retrievable there.
- Two sessions writing two different memories both succeed, and the resulting
  index lists both.
- Replacing a memory this run has not read is refused and says to read it
  first. Replacing one whose body changed since this run read it — a
  concurrent session's write, or a direct user edit — is refused and leaves
  the stored memory intact. Both refusals are observable from the tool result
  alone; no memory-version token appears in either tool's input schema.
- Creating a memory whose slug does not exist succeeds without a prior read.
- A description containing the literal `project-memory` delimiter is rendered
  without breaking the block boundary.
- No memory body is rendered into context except as the result of an explicit
  `MemoryRead` call.
- Project Memory never appears in the system-prompt instruction layers, and
  the index is never persisted into the session journal as a copied context
  block.
- A memory cannot override `AGENTS.md`, tool permissions, hooks, sandbox, or a
  current user correction.
- An empty or disabled store adds no prompt tokens and exposes neither tool.
- A local subagent run renders no index and registers neither tool, while
  still recording the parent's `project_id`.
- A write that exceeds the memory count, the description limit, the body
  limit, or trips the credential detector fails with useful model-facing
  output and leaves the stored memory intact.
- A hand-edited memory file with unparseable frontmatter or an over-budget
  body is skipped, is named on the surface at run start rather than only in
  `doctor`, and does not prevent the other memories from rendering.
- Deleting a memory removes its file and its index line; the index is
  regenerated from the files and never retains an entry for a file that is
  gone.
- Traces identify Project, Workspace, instruction layers, memory count and
  index size, and compaction presence without storing their raw text.
- Users can inspect, edit, delete, clear, and disable Project Memory
  independently of sessions.
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

An index the model reads and chooses from is retrieval enough at this scale,
and it is inspectable, provider-neutral, and deterministic. Embedding
infrastructure adds chunking, ranking, model compatibility, deletion, and
explanation problems before there is enough data to require it. The index does
not become a ranking problem until it is full, which §17 Phase 3 measures.

### 19.7 Automatically extract memory every turn

This adds a model call or hidden inference policy to every task, persists more
surprising content, and makes false memory common before user controls are
proven. Explicit Agent writes plus direct user edits are the safer first
instrumented path.

What Phase 2 ships instead is a command the user invokes. That keeps the cost
and the surprise where a person asked for them, and it is the reason rejecting
automatic extraction does not also mean waiting for the Agent to volunteer
every memory it should have written.

### 19.8 One bounded always-loaded document

This record previously specified a single `MEMORY.md` of a few thousand
characters, replaced whole, rendered in full on every call. It was rejected
after inspecting a working store of the other shape (§4.1).

The single document is worse on three counts. Every memory is resident, so the
budget is spent on knowledge the current task does not need, and admitting a
new memory eventually means deleting an old one — the store stops growing at
the point where it starts being useful. Every write contends on one document,
so two sessions recording unrelated facts conflict for no reason, and a stale
write risks the whole store rather than one entry. And a budget that must hold
everything forces each memory down to a bullet, which is exactly where the
**Why** gets dropped and a memory becomes an unfalsifiable assertion.

The index-plus-files shape costs one extra tool call on the update path and a
read before a body can be used. That is the price of the three properties
above, and it is small because most memories are never read in a given turn.

## 20. Open Questions

- Are 20 memories and a ~3,200-character index the right always-loaded bound
  in real repositories, and what happens at the moment the store fills — does
  the index need grouping, or is deleting the weakest memory the right
  pressure?
- How often does a rendered index line actually lead to a read? A line that is
  never followed is either a description doing the whole job, which is fine,
  or a memory nobody needs, which is not, and the two are told apart by
  whether users correct the Agent afterwards.
- Does withholding memory from local subagents (§9.4) cost anything
  observable, or does a parent's delegated task already carry what a subagent
  needs?
- What shape should the Phase 2 session-review command take, and should it
  write directly or only propose? It ships in Phase 2 because Phase 3 measures
  write frequency and a near-zero count is uninterpretable on its own — it
  cannot distinguish memory being useless from the model never considering a
  write, and a user-invoked path produces the contrast that makes the
  measurement mean something.
- Is explicit relink sufficient after repository moves, or does usage justify
  an opt-in marker inside the Git common directory?
- Does one-run-at-a-time per Desktop Project remain the right concurrency rule
  once one Project can have several worktrees?
- Is a user-level scope worth adding, and what is the evidence? The signal is
  **the same correction recurring across sessions in unrelated Projects** —
  not the same fact duplicated across Projects, which only measures storage
  redundancy. Working agreements are the one memory class whose hit rate
  approaches every turn, so the resident-cost argument that keeps other global
  facts out does not apply to them; what does apply is that they are the least
  verifiable class in the store (§9.2). If the scope is added it is a `scope`
  field on the same files and the same tools, not a second store, and its
  writes should go through the Phase 2 user-invoked review rather than
  landing silently — confirmation is what makes an inference about a person
  auditable. Who authorizes a promotion from Project to user scope is part of
  the same question.

None blocks phases 1 or 2. Each has an observable signal and should be decided
from local use rather than by adding storage or automation speculatively.
