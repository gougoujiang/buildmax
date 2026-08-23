# Server

> **Audience:** contributors · **Status:** current
>
> The live route list is the API's own `GET /openapi.json`, browsable at `/swagger/`.

## Purpose

`internal/server` provides the Go HTTP backend for Portal and worker callbacks.
It is started by `cmd/buildmax-server` through `internal/bootstrap/server.go`.

The server owns route registration, middleware, WebSocket handling, worker API
callbacks, and scheduler startup. Business workflows are delegated to
`internal/service/*`.

## Key Areas

| Area | Package / File | Role |
|------|----------------|------|
| Server wrapper | `internal/server/server.go` | Builds `http.Server`, middleware, static OpenAPI/Swagger routes |
| Handlers | `internal/server/handlers` | Portal API, worker API, webhook, WebSocket handlers |
| Scheduler | `internal/server/scheduler` | Claims pending task runs and launches workers; also runs the background sweeps — expired credentials, abandoned runs, and audit retention |
| Bootstrap | `internal/bootstrap/server.go` | Wires DB, storage, LLM, quota, handlers, scheduler |

## Main Route Groups

- Health and API description: `/healthz`, `/openapi.json`, `/swagger/`
- Auth: `/api/otp/request`, `/api/login`, `/api/token/refresh`, `/api/logout`
- Liveness and readiness: `/healthz`, `/readyz`
- Teams and members: `/api/teams...`
- Agents: `/api/teams/{team_id}/agents...`
- Issues: `/api/teams/{team_id}/issues...`
- Workflows: `/api/teams/{team_id}/workflows...`
- Files: `/api/teams/{team_id}/upload`, `/files...`
- Conversations and tasks: `/api/teams/{team_id}/conversations...`, `/tasks...`,
  including `POST /tasks/{task_id}/cancel` — see "Cancelling a run" below — and
  `POST /tasks/{task_id}/retry` — see "Retrying a run"
- Artifacts: `/api/artifacts/{artifact_id}` and `/content`, with
  `/api/teams/{team_id}/artifacts` for the team's listing and upload, and
  `POST /api/artifacts` for a client that has a login but has not chosen a team
  — an optional `?team_id=` is honoured and no team means the caller's personal
  one, which is how CLI and Desktop publish. The
  ID-addressed routes take the team from the record rather than the path, so
  they use `Guard.MemberOfResourceTeam` and answer a non-member with `404` — an
  `ar_` ID is an identifier, not a credential, and a `403` would make the route
  an oracle for which IDs exist. See
  [../../design/unified-artifacts.md](../../design/unified-artifacts.md)
- Run outputs (the compatibility surface):
  `/api/teams/{team_id}/task-runs/{task_run_id}/artifacts...`
- Run trace: `/api/teams/{team_id}/task-runs/{task_run_id}/trace`
- Managed model calls: `/api/teams/{team_id}/task-runs/{task_run_id}/llm-calls` —
  what a run spent and on which approved alias, without prompts or the
  operator's catalog routing
- Usage: `/api/usage`, `/api/teams/{team_id}/usage`
- Audit trail (owner only): `/api/teams/{team_id}/audit-events`, and
  `/audit-events/export` for the whole trail as CSV or JSONL. The export is
  itself recorded, and pages by keyset cursor rather than offset so a table
  written to while it streams cannot skip a record
- Webhook keys (user-scoped, not team-scoped): `/api/webhook-keys...`
- WebSocket: `/api/teams/{team_id}/ws`
- Worker API: `/api/worker/task-runs/{task_run_id}...`, including
  `/llm/completions` so a worker needs no provider credential and `/artifacts`
  so a run's agent can keep a file for the team. The worker never says which
  team it is writing to: the run token names the run, the run names the task,
  and the task names the team
- Inbound webhook: `/api/webhook`

## Conversation Turns

One turn per conversation runs at a time. The turn queue
(`internal/server/turnqueue`) owns a queue per conversation
and serializes every path into it — WebSocket messages, system turns reporting a
finished task run, and the HTTP `POST .../messages` and `POST .../conversations`
routes. It is server-scoped rather than connection-scoped because one
conversation is reachable from several connections at once.

A message that arrives while a turn is running is queued, up to 10 per
conversation, and runs as its own turn afterwards. WebSocket clients see
`conversation.message.queued`, then `conversation.message.dequeued` when it
starts; `conversation.message.completed` carries `queued_remaining`. Past the cap
the message is refused with `conversation.error` carrying `code: "queue_full"`
(HTTP: `429`), which does not end the turn in flight. Queues are in memory. See
[Queued messages](../../design/queued-messages.md).

## Where A Run Came From

`GET /api/teams/{team_id}/task-runs/{task_run_id}` answers one run's
provenance: who or what asked, through which trigger, repeating which earlier
attempt, and the conversation message it was asked for in, quoted next to the
instruction the worker was given. Those last two are different texts — the
instruction is what Tier 1 decided to send — and holding both is the only way to
tell a constraint the model dropped from one the user never gave.

It is a separate route from the trace because it survives a different absence: a
run that failed before an agent started wrote no trace and still came from
somewhere. A message that cannot be read, or that belongs to another
conversation, is left out rather than failing the request.

## Reporting A Finished Run

A run that reaches a terminal status does two independent things
(`internal/server/handlers/task_result.go`). Every WebSocket connection on the
task's team receives `task.status.changed`, which is an invalidation and not the
outcome: a client answers it by re-reading the task. Separately, one Tier 1 turn
is submitted to the turn queue to report the outcome in the conversation that
started the task. That turn belongs to no connection, so a run finishing while
nobody is watching still leaves its reply in the conversation. The queue is in
memory, so a restart before the turn runs loses the reply — not the result,
which stays on `task_run`.

## Cancelling A Run

`POST /api/teams/{team_id}/tasks/{task_id}/cancel` stops the task's run. What
happens next depends on whether a worker already holds it:

- **Not dispatched yet** (`PENDING`): the run is claimed straight to `CANCELED`,
  its task is synced, and the response is `200`.
- **Dispatched** (`SCHEDULED` or `RUNNING`): the request is recorded on the run
  (`cancel_requested_at`, `cancel_requested_by`) and the response is `202`. The
  worker polls `GET /api/worker/task-runs/{task_run_id}`, sees `cancel_requested`,
  ends its agent loop, uploads what the run produced, and PATCHes `CANCELED`.
- **Already finished**: `409`. Cancelling twice while a run is stopping is not an
  error — the second call answers `202` again.

The server never ends a started run itself: only the run's own process can stop
its agent loop, and a status written from outside would describe a run that is
still executing. `StaleRunReaper` is the backstop for a worker that never
confirms, and finishes such runs as `CANCELED` after a grace period.

A canceled run keeps its output and artifacts. It stopped early, but what it
produced is real work, and discarding it would make cancelling more expensive
than waiting.

## Retrying A Run

`POST /api/teams/{team_id}/tasks/{task_id}/retry` runs the task's most recent
run again. It takes no body: the new run carries the previous run's input, and
records it in `retry_of_task_run_id` with `trigger_source` `task_retry`.

The input comes from the run rather than from the task because a task's later
runs can carry follow-up instructions, and repeating one means running that
again.

Three states answer `409`, each with its own reason:

- a run is already in flight — one task holds at most one active run, and the
  answer to a run taking too long is to cancel it first
- the task has never finished a run — there is nothing to repeat
- the task belongs to a workflow step — the workflow advances or fails its run
  from that step's outcome, so a retry started outside it would report a second
  outcome for a step that is already settled

`retry` creates a run the same way `POST /tasks/{task_id}/runs` does, so team
quota applies to it identically.

## Notes

- User-facing Portal APIs are team-scoped wherever work ownership matters.
- Worker APIs use the run token the scheduler minted for that task run rather
  than user JWT auth. The token carries the user, team, task, and run, and every
  route derives its resource scope from those claims. The old shared worker
  token remains only as a deprecated upgrade fallback — see
  [design/worker-run-token.md](../../design/worker-run-token.md).
- Signing in returns two credentials. The access token is a signed JWT the
  server does not store; the refresh token is a `user_refresh_token` row, which
  is what makes a session revocable. `auth.go` owns both, and every rotation
  stays inside the session named by the access token's `sid` claim.
- Team membership checks live in handler helpers such as `team_authz.go`.
- `POST /api/login` accepts a password or an operator-issued, single-use login
  code. The latter is the account-claim and recovery path because BuildMax has
  no mail channel — see
  [deploy/authentication.md](../../deploy/authentication.md).
- See also: [Store](store.md), [Portal](portal.md), [Boundaries](packages.md).
