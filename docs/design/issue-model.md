# Issue Model: Hierarchy And Comments

## Status

- roadmap_priority: `P2 follow-on`
- status: `implemented`
- follows: [product-vision.md](./product-vision.md), [team-governance.md](./team-governance.md)
- roadmap: [../ROADMAP.md](../ROADMAP.md)
- created_at: `2026-08-17`

P2 made the issue the place where results are visible. This design makes it the
place where work is **broken down** and **discussed**.

Everything in [9. Backend Plan](#9-backend-plan) and
[10. Frontend Plan](#10-frontend-plan) is implemented. Deliberately not built:
everything under [6. Out Of Scope](#6-out-of-scope) — typed links, threaded
replies, `@mentions`, and realtime comment push. This is not a numbered roadmap
priority and `ROADMAP.md` is unchanged.

Current shape of the schema is in
[../contribute/architecture/data-model.md](../contribute/architecture/data-model.md);
this record keeps the reasoning.

## 1. Decision

Two additions to the issue entity, both additive to the schema:

1. **Hierarchy** — one nullable self-reference `issue.parent_issue_id`, an
   adjacency list capped at **two levels** (parent → child, no grandchildren).
2. **Comments** — a new `issue_comment` table with a team-scoped REST surface,
   flat (unthreaded), and open to three author kinds: user, agent, system.

Neither requires a `schema_migration` entry. A new table and a new nullable
column are exactly what `AutoMigrate` owns; `migrations` in
`internal/infra/db/migration.go` stays untouched.

Explicitly rejected alternatives are recorded in [5.1](#51-hierarchy-adjacency-list-capped-at-two-levels)
and [5.3](#53-comments-a-new-issue_comment-table).

## 2. Product Goal

Users should feel:

> I split this issue into sub-issues, I can see from the parent how far they
> got, and everything anyone — person or agent — said about this issue is one
> thread on the issue.

They should not need to know:

- adjacency lists, closure tables, or recursive queries
- that agent output arrives through a task run
- that a Tier 1 conversation is a different thing from a comment

## 3. Current Baseline

The issue entity today:

- `internal/core/issue/issue.go` — `Issue`, `CreateInput`, `UpdateInput`,
  `Store`
- `internal/infra/db/issue.go` — `issueRow`, table `issue`
- `internal/service/issue/service.go` — title/status/assignee validation
- `internal/server/handlers/work/issues.go` — list, create, get, patch, flow,
  agent-runs
- `internal/server/handlers/work/issue_outputs.go` — `latest_result` / `outputs[]`
  aggregation from task-run artifacts
- `internal/server/handlers/routes.go` — seven issue routes
- `internal/mock/issue.go` — in-memory store for handler tests
- `portal/src/pages/issues/Issues.tsx`, `IssueDetail.tsx`,
  `portal/src/features/issues/api.ts`

What already holds and must keep holding:

- Team is the authorization boundary. Every issue handler resolves the issue,
  then compares `issue.TeamID` to the path team.
- `assignee_kind` / `assignee_id` is a polymorphic reference validated in
  `internal/service/issue`, not by a constraint.
- Issues have **no delete path** — no `DeleteIssue` on `IssueStore`, no route.
- Task carries an optional `issue_id`, so an issue's agent runs are already
  findable through `ListTasksByIssue`.
- `handlePatchTerminalStatus` in `internal/server/handlers/worker/worker.go` fires
  `OnTaskRunTerminal` callbacks asynchronously when a run reaches a terminal
  status. This is the existing seam for "something finished on this issue".

## 4. Main Gaps

### 4.1 Issues Are Flat

`ListIssuesByTeam` returns one undifferentiated list ordered by `updated_at`.
A team that decomposes a piece of work has nowhere to put the decomposition, so
it either creates unrelated issues and tracks the relationship in prose, or
overloads a workflow — which is a *reusable linear plan*, not a breakdown of one
piece of work. Those are different objects and conflating them costs the
workflow its reusability.

### 4.2 An Issue Has No Discussion Surface

There is nowhere to write "this is blocked on the vendor" or "the agent's second
run is the one to read". The only writable text on an issue is its
`description`, which is a single mutable field with no author and no history.

### 4.3 Agent Output Is A Panel, Not A Statement

`aggregateIssueOutputs` produces `outputs[]` cards, and `IssueDetail.tsx`
derives a timeline client-side from runs, steps, and tasks. Both are read-only
projections of execution records. Neither can hold a sentence a person wrote,
which means the human and machine halves of an issue's history cannot interleave.

## 5. In Scope

### 5.1 Hierarchy: Adjacency List Capped At Two Levels

One column on `issue`:

| Column | Type | Null | Notes |
|---|---|---|---|
| `parent_issue_id` | `varchar(64)` | yes | `issue.issue_id` of the parent; `NULL` for a top-level issue |

Index: non-unique index on `parent_issue_id`.

Invariants, enforced in `internal/service/issue` (there are no database foreign
keys in this schema, so validation lives in the service, exactly as it does for
`assignee_kind` / `assignee_id`):

| ID | Invariant | Rejected with |
|---|---|---|
| H1 | The parent must exist and be in the same team as the child | `400 parent issue not found` |
| H2 | The parent must itself have `parent_issue_id IS NULL` | `400 issue hierarchy too deep` |
| H3 | An issue that already has children cannot be given a parent | `400 issue has sub-issues` |
| H4 | An issue cannot be its own parent | `400 invalid parent_issue_id` |
| H5 | Status and assignee stay independent per issue; the system never writes a parent's status from its children | — |

H1 deliberately reports a cross-team parent as *not found* rather than
*forbidden*. Distinguishing the two would confirm that an issue ID exists in
another team, and issue IDs are the identifiers Portal puts in URLs.

**Why depth two, and why an adjacency list.** With H2 in place, cycle prevention
is two O(1) lookups on a reparent — the candidate parent must have no parent,
and the issue being moved must have no children. Arbitrary depth needs ancestor
walking on every write and recursive CTEs to count descendants, which turns the
board's list query into a recursive one. The column shape is the same either
way: allowing depth *N* later is a change in
`internal/service/issue`, not a migration. A closure table or materialized path
would buy depth we have no product surface for and cost a second write path that
can disagree with the first.

**Deletion does not cascade** because issues cannot be deleted. If a delete path
is ever added, it inherits an explicit decision — orphan the children by
clearing `parent_issue_id`, or refuse — and that decision belongs to the PR that
adds it, recorded here at that time.

### 5.2 Progress Rollup Is Read-Only And Computed

An issue response gains three derived fields, none of them stored:

- `child_count` — number of issues whose `parent_issue_id` is this issue
- `done_child_count` — of those, how many have `status = 'done'`
- `comment_count`

They are computed per response page with one grouped query each:

```sql
SELECT parent_issue_id,
       COUNT(*)                                            AS total,
       SUM(CASE WHEN status = 'done' THEN 1 ELSE 0 END)    AS done
FROM issue
WHERE parent_issue_id IN (?)
GROUP BY parent_issue_id;
```

A stored counter would be a second source of truth that drifts the first time a
status update fails between the two writes. Two grouped queries per list page is
the cost, and it is bounded by the page limit, not by the team's issue count.

**A parent's status is never derived.** A parent may be marked `done` while
children are open; the API allows it and the UI shows a warning. Blocking the
transition would invent a completion policy that no team has agreed to, and
"why can't I close this" is a worse failure than a visible inconsistency the
user chose.

### 5.3 Comments: A New `issue_comment` Table

| Column | Type | Null | Notes |
|---|---|---|---|
| `id` | `bigint unsigned` | no | Row key |
| `public_id` | `binary(12)` | no | Public handle, unique |
| `issue_id` | `bigint unsigned` | no | `issue.id`; indexed |
| `author_kind` | `varchar(16)` | no | `user`, `agent`, or `system` |
| `author_id` | `varchar(64)` | no | A user's or an agent's handle; empty for `system`. Opaque, because the kind decides which |
| `body` | `text` | no | Markdown source, stored raw |
| `source_task_id` | `bigint unsigned` | yes | Set on an agent comment |
| `source_task_run_id` | `bigint unsigned` | yes | Set on an agent comment |
| `created_at` | `bigint` | yes | `autoCreateTime` |
| `edited_at` | `bigint` | yes | `NULL` until the body is changed |

Indexes: PK `id`; unique `public_id`; composite (`issue_id`, `created_at`).
The identifier shapes are the ones
[entity-identity.md](entity-identity.md) settled; the live schema is in
[../contribute/architecture/data-model.md](../contribute/architecture/data-model.md).

Add `PrefixIssueComment = "ic"` to `internal/util/id.go` and to the prefix list
in `docs/contribute/architecture/data-model.md`.

Decisions inside that table:

- **No `team_id`.** The row's team is its issue's team, and every issue handler
  already loads the issue to authorize. Denormalizing would create a second
  place for the authorization key to be wrong. This follows
  `conversation_message`, which likewise resolves its team through its parent.
- **`edited_at` instead of `updated_at`.** "Has been edited" is a fact readers
  should see; `NULL` means never edited, which is meaningful in the way this
  schema expects nullable timestamps to be.
- **Flat, not threaded.** Replies-to-replies need a second ordering rule and a
  collapse UI, for a surface that holds tens of comments, not thousands.
- **Body is capped at 16 KiB** and rejected with `400 comment too long` above
  it. Long content is an artifact; a comment points at one.
- **Ordering is `created_at ASC, id ASC`** — a thread reads oldest first.
  Prefixed IDs are random, so `issue_comment_id` is never a sort key.

**Why not reuse `conversation_message`.** That table is LLM turn history: it has
`role`, `tool_calls`, and `tool_call_id`, and its rows are replayed into a model
context. A comment is a durable statement *about a work object*, addressed to
people, that outlives any particular conversation. Merging them would force
every comment through a conversation and make "what did the team say about this
issue" a query over LLM turns — including tool traffic. The two objects share a
shape and not a purpose.

### 5.4 Agent-Authored Comments On Run Completion

When a task carrying an `issue_id` reaches a terminal status, post one comment
with `author_kind = "agent"`, `author_id` = the task's agent, and
`source_task_id` / `source_task_run_id` set.

The seam is an `OnTaskRunTerminal` callback registered in `internal/bootstrap`,
not code inside `handlePatchTerminalStatus`. The worker handler already fires
those callbacks in a goroutine after it has responded, which gives the property
this needs: **a failed comment write must not fail the run**, and a run's
terminal report must not wait on it.

Bounds, so the thread stays readable:

- one comment per terminal run, never per streamed chunk
- body is a bounded summary (2000 characters) plus a pointer to the run, not the
  artifact content — `aggregateIssueOutputs` remains the source for content
- a run that produced neither output nor error message posts nothing

### 5.5 Authorization

New actions in `internal/core/team/policy.go`, which `internal/server/access`
and the team service both decide against:

| Action | Owner | Admin | Member |
|---|---|---|---|
| `comment_issue` — create a comment | yes | yes | yes |
| `moderate_issue_comments` — edit or delete another author's comment | yes | no | no |

Reparenting is part of `PATCH /issues/{issue_id}` and needs only team
membership, like every other field on that endpoint except workflow assignment.

Editing and deleting your **own** comment needs only team membership; the
handler compares `author_kind == "user" && author_id == caller`. Agent and
system comments have no human author and are editable by nobody — they are the
record of what a run reported, and a record you can rewrite is not one.

### 5.6 Edit And Delete Are Hard Operations

`PATCH` replaces the body and stamps `edited_at`. `DELETE` removes the row.

Hard delete is chosen because this schema has no soft-delete precedent —
introducing `deleted_at` on one table sets a convention every future table has
to answer to, and `AutoMigrate` has no down path to walk it back. No audit event
is written: the audit model deliberately carries no content, so it could record
only that a deletion happened, which nobody has asked to query.

The cost is real and accepted: a deleted comment leaves no trace, so a thread
can lose the statement a later comment was replying to. A tombstone
(`deleted_at`, body cleared, "deleted by author" rendered) is the change to make
if that turns out to matter, and it stays available — adding a nullable column
later is additive DDL.

## 6. Out Of Scope

- typed links between issues (`blocks`, `duplicates`, `relates`) — a different
  table with a different UI, and no demand yet
- threaded replies, reactions, and attachments on comments
- `@mentions`, and mention-triggered agent runs. The seam exists — a comment
  create handler can call the same service `POST /agent-runs` uses — but parsing
  mentions, resolving them, and deciding notification behavior is its own design
- realtime comment push over WebSocket; the issue detail page polls
- moving a parent and its children across teams
- issue delete or archive
- rolling a child's `outputs[]` up into the parent's Results panel. The parent's
  Results panel must keep meaning "what this issue produced"
- CLI, TUI, and Desktop surfaces. Per
  [surface-positioning.md](./surface-positioning.md), issue administration is
  Portal's job and Desktop must not duplicate it
- an agent tool for reading or writing issues. Runtime tool names are
  `internal/tool/names.go`'s business and adding one is a separate decision

## 7. Product Surface

### 7.1 Issue Board (`Issues.tsx`)

- The board requests `?parent_id=none` and shows only top-level issues.
- A row with children shows `done/total` sub-issue progress and a comment count,
  and expands in place to list them. Children are fetched on expand, not with
  the page — a board of parents should not pay for every breakdown nobody
  opened.
- Adding a sub-issue happens on the parent's detail page, not from a board row.
  The board's job is scanning; splitting an issue up is something you do while
  looking at it. A second composer on the board would also mean two places that
  create issues with different fields.

The list API's **default** stays unchanged — no `parent_id` means every issue,
flat, as today. Portal opts into the filtered view. That keeps the endpoint
backward compatible for any other client while the board changes behavior.

### 7.2 Issue Detail (`IssueDetail.tsx`)

- A **Sub-issues** panel on a parent: child rows with status and assignee, a
  `done/total` count, and a one-field composer — a sub-issue starts as a title
  and everything else is filled in on its own page, so decomposing an issue
  stays one keystroke per piece. On a child, a link back to the parent instead.
- A **Discussion** panel: the comment thread, oldest first, with a composer.
  Author rendering distinguishes the three kinds — a person's name, an agent's
  name with a run link, and system text.
- The existing client-derived timeline stays, and gains comment events, so run
  activity and human statements interleave in one column.
- Results keeps its current meaning and content.

### 7.3 Composer

Markdown textarea, Cmd/Ctrl+Enter to submit, optimistic append with rollback on
error, character counter past 15 KiB. Own comments show edit and delete;
`moderate_issue_comments` holders see delete on any human comment.

## 8. API Design

All routes are team-scoped and registered in
`internal/server/handlers/routes.go`.

| Method | Path | Notes |
|---|---|---|
| `GET` | `/api/teams/{team_id}/issues?parent_id=` | `parent_id=none` for top-level only; `parent_id=i_…` for one parent's children; absent for all |
| `POST` | `/api/teams/{team_id}/issues` | body gains `parent_issue_id` |
| `PATCH` | `/api/teams/{team_id}/issues/{issue_id}` | body gains `parent_issue_id`; `""` clears it, matching the assignee convention already in `updateIssue` |
| `GET` | `/api/teams/{team_id}/issues/{issue_id}/comments` | `limit` / `offset`, oldest first |
| `POST` | `/api/teams/{team_id}/issues/{issue_id}/comments` | `{ "body": "…" }` |
| `PATCH` | `/api/teams/{team_id}/issues/{issue_id}/comments/{comment_id}` | `{ "body": "…" }` |
| `DELETE` | `/api/teams/{team_id}/issues/{issue_id}/comments/{comment_id}` | `204` |

`IssueResponse` gains, all additive:

```json
{
  "parent_issue_id": "i_…",
  "child_count": 3,
  "done_child_count": 1,
  "comment_count": 7
}
```

`issueFlowResponse` gains `parent` (a compact `IssueResponse` or absent) and
`children` (`[]IssueResponse`, empty for a child). Its `runs`, `agent_tasks`,
`outputs`, and `latest_result` stay issue-scoped and unchanged.

`issueCommentResponse`:

```json
{
  "id": "ic_…",
  "issue_id": "i_…",
  "author_kind": "agent",
  "author_id": "a_…",
  "body": "…",
  "source_task_id": "t_…",
  "source_task_run_id": "r_…",
  "created_at": 1755400000,
  "edited_at": null
}
```

The comment routes carry `{issue_id}` even though `{comment_id}` is unique,
because the issue is what authorizes the request. The handler loads the issue,
checks `issue.TeamID`, then checks that the comment belongs to that issue —
a comment ID from another issue is a `404`, not a successful edit.

## 9. Backend Plan

### M1. Hierarchy Schema And Store

- `parent_issue_id` on `issueRow` and `model.Issue`; `ParentIssueID *string` on
  `CreateIssueInput` and `UpdateIssueInput`.
- `ListIssuesByTeam` takes a `ListIssuesFilter{ ParentIssueID *string; TopLevelOnly bool }`
  instead of gaining a parallel method.
- `ChildStatsForIssues(ctx, issueIDs []string) (map[string]IssueChildStats, error)`
  and `ListIssueChildren(ctx, parentIssueID string) ([]Issue, error)`.
- Mirror all of it in `internal/mock/issue.go`.
- `ListIssuesByUser` is not extended. It is the legacy user-scoped path and
  `data-model.md` says not to build on it.

### M2. Hierarchy Service Invariants

H1–H4 in `internal/service/issue`, with `ErrParentNotFound`,
`ErrHierarchyTooDeep`, `ErrIssueHasChildren`, `ErrInvalidParent` mapped in
`writeIssueServiceError`. Applies to both create and update paths.

### M3. Comment Schema And Store

`issueCommentRow`, `model.IssueComment`, `model.IssueCommentStore`, the `ic_`
prefix, `issueCommentRow{}` appended to the `AutoMigrate` list, mock store.

### M4. Comment Service And Routes

`internal/service/issue` gains comment commands with body validation and the
author checks from 5.5. Four handlers, `IssueCommentStore` on
`handlers.Config`, and — following the existing pattern — a
`503 comments not configured` when the store is absent, so a deployment without
it still serves issues.

### M5. Agent Comment On Run Completion

An `OnTaskRunTerminal` callback in `internal/bootstrap` that resolves the task's
issue, formats the bounded summary, and writes the comment. Every failure path
logs and returns.

## 10. Frontend Plan

### M1. Sub-Issues

`parent_id` on `getIssues`, `parent_issue_id` on create and update, the board's
expandable rows with progress, and the detail page's Sub-issues panel with its
parent link and title composer.

### M2. Discussion

`features/issues/comments.ts`, a `Discussion` panel and composer, author
rendering for the three kinds, edit and delete affordances, polling on the
detail page.

### M3. Timeline Merge

Comment events folded into the `timeline` memo in `IssueDetail.tsx`, and a
parent-closed-with-open-children warning next to the status control.

## 11. Validation

Go tests:

- store: create with and without a parent; `ListIssuesByTeam` under each filter
  value; `ChildStatsForIssues` with mixed statuses and with an empty ID list
- service: H1 cross-team parent, H2 grandchild, H3 parent-with-children being
  adopted, H4 self-parent — each rejected, each leaving the row unchanged
- service: clearing a parent with `""`, and a create that names a parent in
  another team
- comments: create, list ordering and pagination, edit stamping `edited_at`,
  delete, body over 16 KiB, comment ID from a different issue returning `404`
- authz matrix: extend `team_authz_matrix_test.go` with `comment_issue` and
  `moderate_issue_comments`
- handlers: an issue in another team returns `404` from every new route
- terminal callback: a run on an issue writes exactly one comment; a store
  failure does not fail the run; a run with no output writes nothing

Portal: `./make check portal`, plus the board's filtered request and the
composer's optimistic append and rollback.

Full: `./make check ci`.

## 12. Risks

| ID | Risk | Response |
|---|---|---|
| R1 | The two-level cap gets challenged before it ships | Depth lives in the service, not the schema; raising it is a service change with no data migration. Ship the cap, revisit with a real request |
| R2 | Child stats become N+1 as the board grows | One grouped query per page, bounded by the page limit. A per-row count is a review-blocking mistake, not a tuning issue |
| R3 | Comments become a second chat and drift from Tier 1 conversations | Comments are statements about an issue and are never replayed into a model context. If an agent needs a comment as input, it arrives as an explicit run input, not implicitly |
| R4 | Agent comments flood the thread | One per terminal run, none for empty runs, summary bounded at 2000 characters |
| R5 | Polling means two commenters see stale threads | Accepted for phase 1. The hub already carries team-scoped connections, so an `issue.comment.created` event is a later addition, not a rewrite |
| R6 | Hard delete loses thread context | Accepted, and the reason it is an open question rather than a settled one |

## 13. Open Questions

Two questions from the first draft are now settled and are recorded in the
sections they belong to rather than here: this does **not** become a numbered
roadmap priority (`ROADMAP.md` is unchanged), and comment deletion is a **hard
delete** (see [5.6](#56-edit-and-delete-are-hard-operations)).

1. **Should a parent's completion be constrained by its children?** 5.2 says
   no. A softer option — a confirmation dialog rather than a rejection — is
   available if warnings prove too quiet.
4. **Should a new child inherit its parent's assignee?** Convenient for
   decomposing work assigned to one agent; surprising when a parent is reassigned
   later. Currently: no inheritance.
5. **What does `@agent` in a comment do?** The obvious answer is "start a run on
   this issue". It is out of scope here and it is the most likely next request.

## 14. Recommended First PR

M1 plus M2: the `parent_issue_id` column, the store changes, the four
invariants, the create and patch wiring, and the tests in
[11. Validation](#11-validation) that cover them. No UI, no comments.

It is the smallest slice that is verifiable on its own, it proves the invariant
placement before any of it has a consumer, and it leaves `issue_comment` — the
larger and more opinionated half — to a PR that can be argued on its own terms.
