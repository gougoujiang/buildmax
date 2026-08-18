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
| Scheduler | `internal/server/scheduler` | Claims pending task runs and launches workers |
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
  including `POST /tasks/{task_id}/cancel` — see "Cancelling a run" below
- Artifacts: `/api/teams/{team_id}/task-runs/{task_run_id}/artifacts...`
- Run trace: `/api/teams/{team_id}/task-runs/{task_run_id}/trace`
- Managed model calls: `/api/teams/{team_id}/task-runs/{task_run_id}/llm-calls` —
  what a run spent and on which approved alias, without prompts or the
  operator's catalog routing
- Usage: `/api/usage`, `/api/teams/{team_id}/usage`
- Audit trail (owner only): `/api/teams/{team_id}/audit-events`
- Webhook keys (user-scoped, not team-scoped): `/api/webhook-keys...`
- WebSocket: `/api/teams/{team_id}/ws`
- Worker API: `/api/worker/task-runs/{task_run_id}...`, including
  `/llm/completions` so a worker needs no provider credential
- Inbound webhook: `/api/webhook`

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

## Notes

- User-facing Portal APIs are team-scoped wherever work ownership matters.
- Worker APIs use worker-token auth rather than user JWT auth. Managed inference
  is the exception: `/llm/completions` takes the run token the scheduler minted
  for that run, so the call carries a user, a team, and a run rather than only
  "a worker" — see [design/worker-run-token.md](../../design/worker-run-token.md).
- Signing in returns two credentials. The access token is a signed JWT the
  server does not store; the refresh token is a `user_refresh_token` row, which
  is what makes a session revocable. `auth.go` owns both, and every rotation
  stays inside the session named by the access token's `sid` claim.
- Team membership checks live in handler helpers such as `team_authz.go`.
- `POST /api/login` is disabled unless a dev OTP is configured — see
  [deploy/authentication.md](../../deploy/authentication.md).
- See also: [Store](store.md), [Portal](portal.md), [Boundaries](packages.md).
