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
- Auth: `/api/otp/request`, `/api/login`
- Teams and members: `/api/teams...`
- Agents: `/api/teams/{team_id}/agents...`
- Issues: `/api/teams/{team_id}/issues...`
- Workflows: `/api/teams/{team_id}/workflows...`
- Files: `/api/teams/{team_id}/upload`, `/files...`
- Conversations and tasks: `/api/teams/{team_id}/conversations...`, `/tasks...`
- Artifacts: `/api/teams/{team_id}/task-runs/{task_run_id}/artifacts...`
- Usage: `/api/usage`, `/api/teams/{team_id}/usage`
- Webhook keys (user-scoped, not team-scoped): `/api/webhook-keys...`
- WebSocket: `/api/teams/{team_id}/ws`
- Worker API: `/api/worker/task-runs/{task_run_id}...`
- Inbound webhook: `/api/webhook`

## Notes

- User-facing Portal APIs are team-scoped wherever work ownership matters.
- Worker APIs use worker-token auth rather than user JWT auth.
- Team membership checks live in handler helpers such as `team_authz.go`.
- `POST /api/login` is disabled unless a dev OTP is configured — see
  [deploy/authentication.md](../../deploy/authentication.md).
- See also: [Store](store.md), [Portal](portal.md), [Boundaries](packages.md).
