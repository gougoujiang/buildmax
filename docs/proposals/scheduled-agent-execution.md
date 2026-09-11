# Scheduled Agent Execution

> **Audience:** contributors and early adopters · **Status:** proposal — under
> discussion

Opened: 2026-09-11

Related current records:
[agent execution and Task threads](../design/agent-execution-and-task-threads.md)
(§4.5 already names a "future schedule" as a typed trigger origin),
[workflow runtime](../design/workflow-runtime.md) (defers schedules until
admission, quota, and recovery are proven),
[system administration](../design/system-administration.md) (disabled-creator
and quota handling), and
[server coordination](../design/server-coordination.md) (multi-replica claim
safety).

## Contents

- [1. Question](#1-question)
- [2. Why Now, And What Exists](#2-why-now-and-what-exists)
- [3. Goals](#3-goals)
- [4. Non-Goals](#4-non-goals)
- [5. First-Principles Shape](#5-first-principles-shape)
- [6. The Schedule Entity](#6-the-schedule-entity)
- [7. Firing: Claim, Admit, Advance](#7-firing-claim-admit-advance)
- [8. Time Semantics](#8-time-semantics)
- [9. Authorization, Quota, And Runaway Control](#9-authorization-quota-and-runaway-control)
- [10. Surfaces](#10-surfaces)
- [11. Disposition Of `ChannelCron`](#11-disposition-of-channelcron)
- [12. Options And Decisions](#12-options-and-decisions)
- [13. Decisions And Open Questions](#13-decisions-and-open-questions)
- [14. Delivery Phases](#14-delivery-phases)

## 1. Question

Should a Space be able to run an Agent automatically on a recurring time
schedule — "every weekday at 09:00, summarize the new Issues" — with no human
triggering each run, and if so, what is the smallest set of concepts that
delivers it coherently on the existing Task execution plane?

## 2. Why Now, And What Exists

Nothing time-driven exists today. The evidence:

- No time-trigger field exists anywhere. There is no `next_fire_at`,
  `cron_expr`, `run_at`, or equivalent in any `xxxRow` struct or domain type.
- `internal/service/conversation/channel/types.go` defines `ChannelCron =
  "cron"` and lists it in `ValidChannels()`, but there is no cron Adapter, no
  cron parser, and no timer. `docs/current-state.md` records that "cron remains
  channel vocabulary". It is an unbacked placeholder, not a partial
  implementation. §11 disposes of it.
- `internal/server/scheduler` is a dispatch poller: it moves already-`PENDING`
  TaskRuns to workers. It never *creates* a run. A schedule needs something to
  create the run when a wall-clock time arrives.

So this is a new capability, not the completion of a half-built one. The
execution plane it plugs into, however, already exists and is the reason the
new surface can stay small: `agent-execution-and-task-threads.md` §4.5 already
declares "API, Webhook, And Future Schedule" as typed, non-conversational
trigger origins that "create Space-owned Tasks through the same service" and
"record a typed trigger source". This proposal fills in that named slot.

## 3. Goals

- A Space member can create a recurring trigger that runs a chosen Agent with a
  fixed input on a cron schedule in a chosen timezone.
- Each firing produces one ordinary Task and TaskRun through the existing Task
  application service, with a typed `schedule` trigger source, so status,
  cancellation, quota, trace, artifacts, and audit have exactly one owner —
  the same one every other run has.
- Firing is safe under multiple server replicas: a due schedule fires exactly
  once per due time, never zero times because two replicas each assumed the
  other would, never twice because both claimed it.
- Server restart across a due time does not silently skip work or replay a
  backlog of historical fires.
- A schedule's history is discoverable as the list of Tasks it created; the
  schedule row itself stays small.

## 4. Non-Goals

- **Continuing one long-lived thread.** Each firing is a fresh objective and a
  fresh Agent session (a new Task), not a Continue on a growing thread. Thread
  accumulation would grow context and cost without a demonstrated need. A
  "continue the same Task" mode is a later question (§13), not this slice.
- **Local CLI scheduling.** The CLI is a single-run process with no resident
  loop or multi-replica coordination. A user who wants the local binary on a
  timer uses the operating system's own cron/launchd/Task Scheduler to invoke
  `buildmax`. Scheduling lives where a resident, coordinated process already
  lives: the server.
- **Event and webhook triggers.** Inbound events are a separate typed origin
  (`webhook` already exists as a trigger source). This proposal is time only.
- **Sub-minute granularity.** The smallest interval is one minute; finer
  cadence is a streaming/event concern, not a schedule.
- **Data migration.** Alpha: no released schedule data exists to preserve.

## 5. First-Principles Shape

The essential outcome is "a run happens at a time nobody is present for". Strip
it to what must be true:

1. Something durable remembers *what* to run (Agent + input, in a Space) and
   *when* (a recurrence rule + timezone). That is one new entity: `Schedule`.
2. Something resident notices a due time and admits the run. The server already
   runs resident poll loops in `internal/server/scheduler` (the dispatcher, the
   stale-run reaper, the retainers). A schedule dispatcher is one more loop of
   the same shape, not a new subsystem.
3. Admission already exists. `task.Service.CreateTask` validates Space, Agent,
   quota, and input and commits a Task plus first TaskRun atomically, tagging a
   `trigger_source`. A firing is a call to it with `trigger_source = schedule`.

Nothing else is required. In particular a firing needs no "schedule run" table:
the run it produces *is* a TaskRun, which already records input, trigger source,
status, usage, output, trace, and artifacts. A schedule's firings are queried
as `Tasks where trigger_source = schedule and schedule_id = X`. Adding a
parallel execution-record type would duplicate the plane this project spent
`agent-execution-and-task-threads.md` unifying.

The dependency direction stays the one that record already drew, with Schedule
as a peer of the other typed origins:

```text
Schedule (space-owned, time trigger)
        |
        v
  task.Service.CreateTask   <-- same service Issue, Workflow, API, Portal call
        |
        v
   Task + first TaskRun
        |
        v
  Scheduler -> Worker -> shared Agent runtime
```

## 6. The Schedule Entity

A `Schedule` is Space-owned, mirroring how Space is authoritative for every
execution resource. The proposed domain lives in a new `internal/core/schedule`
package (pure domain: no cron library, no infra), with its store in
`internal/infra/db` as a singular `schedule` table:

```text
Schedule
  id                      NewPublicID
  space_id                required, authoritative owner
  agent_id                required executor
  created_by              required actor; carried onto each Task
  name                    human label
  input                   the fixed prompt each firing runs
  cron_expr               recurrence rule
  timezone                IANA name, e.g. "Asia/Shanghai"
  enabled                 bool; a paused schedule keeps its row and next time
  next_fire_at            UTC instant the dispatcher claims on; the due index
  last_fire_at            UTC instant of the most recent fire (nullable)
  last_task_id            the Task the most recent fire created (nullable)
  consecutive_failures    bounds runaway cost (§9)
  created_at / updated_at
```

`next_fire_at` is the single fact the dispatcher queries and the compare-and-swap
target that makes firing exactly-once (§7). The schedule stores no list of past
fires; that list is a Task query.

Persisted JSON uses explicit `snake_case` tags; the table name is singular; the
id uses `NewPublicID` — per `entity-identity.md` and the repository conventions.

## 7. Firing: Claim, Admit, Advance

A new `ScheduleDispatcher` in `internal/server/scheduler` polls on a coarse
interval (a minute is enough given minute granularity). Each tick, for each due
schedule, it performs one compare-and-swap claim, exactly as `pollOnce` claims a
run with `TransitionTaskRun`:

```text
tick:
  candidates = store.DueSchedules(now)          # enabled AND next_fire_at <= now
  for s in candidates:
    next = cronNext(s.cron_expr, s.timezone, now)
    claimed = store.ClaimSchedule(s.id,
                expectedNextFireAt = s.next_fire_at,
                newNextFireAt      = next)       # conditional UPDATE
    if not claimed: continue                     # another replica took it
    task.Service.CreateTask(CreateTaskCmd{
        SpaceID: s.space_id, AgentID: s.agent_id,
        CreatedBy: s.created_by, Input: s.input,
        TriggerSource: RunTriggerSourceSchedule,  # new constant
        ScheduleID: s.id,                          # new optional Task origin
    })
    store.RecordFire(s.id, task.id, outcome)       # last_fire_at, last_task_id
```

The conditional update — advance `next_fire_at` only if it still equals what we
read — is what makes firing exactly-once across replicas, reusing the
optimistic-concurrency pattern the run scheduler already relies on. The claim
advances the clock *before* admission, so a `CreateTask` failure does not wedge
the schedule on the same due time forever; it is recorded as a failed fire and
the schedule proceeds to its next time (§9 bounds a schedule that fails every
time).

`RunTriggerSourceSchedule` joins the existing `RunTriggerSource*` constants in
`internal/core/task/task.go`. `Task` gains an optional `schedule_id` origin
relation, exactly as it already carries optional `issue_id` and
`workflow_step_run_id` — an origin, never an authorization parent
(`agent-execution-and-task-threads.md` §8.1).

## 8. Time Semantics

- **Timezone.** Cron is evaluated in the schedule's IANA timezone so "09:00"
  survives DST. Storage and all comparisons are UTC.
- **Missed fires coalesce.** If the server is down across one or more due times,
  the next `cronNext` is computed from `now`, not from the stale `next_fire_at`.
  A schedule that should have fired at 09:00 and 10:00 during an outage that
  ends at 10:30 fires once, immediately, then resumes at its next regular time.
  No backfill storm, no silent whole-day skip. The single catch-up fire is
  recorded as late.
- **Exactly-once per due time** under healthy operation follows from the
  compare-and-swap claim (§7).

Whether a long outage should suppress the single catch-up fire entirely (a
"don't run stale work" option) is deferred (§13); the default is one catch-up.

## 9. Authorization, Quota, And Runaway Control

- **Authorization.** Creating, editing, enabling, disabling, and deleting a
  schedule is authorized through `schedule.space_id` and current Space
  membership — the same rule Tasks use. The firing Task's `created_by` is the
  schedule's creator, so quota, audit, and the run token attribute to a real
  actor, matching how Issue-originated runs attribute today.
- **Disabled creator.** The run scheduler already fails a run whose creator was
  disabled. For a *repeating* trigger that would mint a failed Task every
  minute. So a firing whose creator is disabled pauses the schedule
  (`enabled = false`) with a recorded reason instead, and re-enabling is an
  explicit act. This reuses the disabled-account concept from
  `system-administration.md` rather than inventing schedule-specific auth.
- **Quota.** Each firing passes through the existing `QuotaChecker` in
  `task.Service`. A firing refused by quota is a recorded skipped fire, not an
  error that stops the schedule.
- **Runaway control.** A schedule whose Agent fails on every run would burn
  quota indefinitely. After N consecutive failed fires the dispatcher pauses the
  schedule and surfaces why. This is the one guard included rather than
  deferred, because "an unattended trigger that fails forever" is a concrete
  cost failure, not a hypothetical. N and whether pause-vs-throttle is the right
  response are tunable (§13).

## 10. Surfaces

- **API.** Space-scoped, peer to the Task routes and registered the same way
  (each handler subpackage's `Register`, composed in `routes.go`, matched by
  `openapi.json`):

  ```text
  POST   /api/spaces/{space_id}/schedules          { agent_id, name, input, cron_expr, timezone }
  GET    /api/spaces/{space_id}/schedules
  GET    /api/spaces/{space_id}/schedules/{id}
  PATCH  /api/spaces/{space_id}/schedules/{id}     { enabled?, input?, cron_expr?, timezone?, name? }
  DELETE /api/spaces/{space_id}/schedules/{id}
  ```

  `cron_expr` and `timezone` are validated at write time; an invalid expression
  is a `KindInvalid` refusal, never a row that fails silently at fire time.

- **Portal.** A Space-scoped Schedules view: list with next/last fire and
  enabled state; create/edit; enable/disable/delete; and a link from each
  schedule to the Tasks it created (a `trigger_source = schedule` filter on the
  existing Space task history, which already exists per §11.4 of the execution
  record). No new Task-history machinery is needed.

- **Deleting a schedule** removes only the trigger. Tasks it already created are
  independent execution history and are untouched — they are not the schedule's
  children.

## 11. Disposition Of `ChannelCron`

`ChannelCron` currently sits in the *Conversation* channel enum. That is the
wrong plane. `agent-execution-and-task-threads.md` §13.4 already removed the
synthetic Conversations that Issue and Workflow runs once created; a scheduled
run likewise creates a Task directly and never a Conversation. The typed source
of a scheduled run is a TaskRun `trigger_source`, not a conversation transport.

So this proposal removes `ChannelCron` from
`internal/service/conversation/channel` and its `ValidChannels()` list, and adds
`RunTriggerSourceSchedule` on the Task plane instead. This is the Alpha "fix the
wrong shape coherently, no compatibility layer" rule: the placeholder moves to
the plane it actually belongs on rather than being wired up where it sits. The
handful of tests naming `"cron"` as a valid conversation channel change with it.

## 12. Options And Decisions

| Decision | Chosen | Rejected alternative and why |
|---|---|---|
| Execution record per fire | Reuse Task + TaskRun | A dedicated `schedule_run` table duplicates the unified execution plane; the run already records everything. |
| Thread model | New Task each fire | Continue-the-thread grows context/cost with no shown need; deferred as a mode. |
| Trigger loop home | New loop in `internal/server/scheduler` | A separate service/binary adds a process and coordination surface for one poll loop. |
| Exactly-once | Compare-and-swap on `next_fire_at` | A leader lock or external scheduler (Temporal/cron sidecar) adds a dependency `workflow-runtime.md` explicitly declined as a default. |
| Placeholder | Remove `ChannelCron`, add `RunTriggerSourceSchedule` | Implementing a cron Conversation adapter would cement the wrong plane. |
| Local CLI timer | Out of scope; use OS cron | A resident CLI daemon duplicates the server's resident, coordinated loop. |
| Missed fires | One coalesced catch-up | Backfilling every missed slot risks a storm; silently skipping loses a signal. |

## 13. Decisions And Open Questions

Decided (2026-09-11):

- **Thread model: a new Task each fire.** Each firing is a fresh objective and
  Agent session; there is no Continue-the-thread mode in this slice.
- **Input: a fixed string.** No templating in the first slice.
- **Runaway response: pause after N consecutive failures.** The dispatcher sets
  `enabled = false` with a recorded reason; re-enabling is explicit.
- **Permission: ordinary Space membership.** Schedule create/edit/delete is
  authorized exactly as Tasks are; no separate operator capability.

Still open:

- The default N for the consecutive-failure pause (and whether a disabled-creator
  pause shares the same counter or is immediate). A first guess is a small
  single-digit N; settle it when phase 2 is written.
- Whether a long outage should suppress the single catch-up fire (a
  max-staleness bound) rather than always firing once. Default remains one
  catch-up (§8) until there is evidence a stale run causes harm.
- Whether a later "continue the same Task/session each fire" mode is worth
  adding once the fixed-objective slice has usage — and what would bound its
  context growth. Out of scope now; recorded so the default is a deliberate
  choice, not an omission.

## 14. Delivery Phases

Each phase is independently complete and testable; later phases do not rewrite
earlier ones.

1. **Core + store.** `internal/core/schedule` domain, `schedule` table and
   store with `DueSchedules`, `ClaimSchedule` (compare-and-swap), and
   `RecordFire`. MySQL-scope tests for the claim under contention, mirroring the
   run-scheduler concurrency tests. `RunTriggerSourceSchedule` and the optional
   `Task.schedule_id` origin.
2. **Dispatcher.** `ScheduleDispatcher` loop admitting due schedules through
   `task.Service.CreateTask`; cron parsing with timezone; coalesced catch-up;
   disabled-creator pause and consecutive-failure pause. Tests drive a fake
   clock and assert exactly-once and catch-up behavior.
3. **API + OpenAPI.** The five Space-scoped routes, cross-space authorization
   matrix coverage, and an exact `openapi.json` match.
4. **Portal.** Schedules list/create/edit/enable/disable/delete and the
   schedule-to-Tasks link, with browser evidence.
5. **Placeholder removal.** Remove `ChannelCron` and update the conversation
   channel tests, `docs/current-state.md`, and the reference/configuration and
   architecture docs in the same change.

A cron-expression parser is a new pure-Go dependency (parser only; the loop is
ours). It ships with its lockfile and license-check update per the dependency
rule, or is written in-package if a minimal predictable subset (the standard
five fields) is all the first slice needs — decided when phase 2 starts.
