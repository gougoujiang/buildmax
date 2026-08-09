# Store

> **Audience:** contributors · **Status:** current

## Purpose

`internal/infra/db` provides the MySQL/GORM persistence implementation for the
repository contracts defined in `internal/core/model`.

The active persistence model is team-scoped for shared work:

- user
- team / team_member
- conversation / conversation_message
- issue
- agent
- workflow / workflow_run / workflow_step_run
- task / task_run / task_run_artifact
- quota_tier
- user_webhook_key

Table names are singular per project convention.

There is no usage table. `TeamUsageInWindow` aggregates on read: it counts
`task_run` rows joined to `task` by team and sums their prompt and completion
tokens, plus the title-generation tokens recorded on tasks created in the same
window. Metering therefore has no separate write path to keep in sync.

## Key Boundaries

| Layer | Package | Role |
|-------|---------|------|
| Contracts/entities | `internal/core/model` | Shared structs and repository interfaces |
| GORM implementation | `internal/infra/db` | MySQL-backed store implementing those interfaces |
| Object storage | `internal/infra/objectstore` | Team files and run artifacts, local FS or S3/MinIO |

## Notes

- Public entity IDs use prefixed IDs — `u_` user, `tm_` team, `i_` issue, `a_` agent,
  `w_`/`wr_`/`wsr_` workflow, workflow run, workflow step run, `c_` conversation,
  `cm_` conversation message, `t_` task, `r_` task run, `ar_` artifact,
  `f_` artifact item, `whk_` webhook key. Constants live in `internal/util/id.go`.
- Session IDs are the exception: they are internal and use UUIDs.
- JSON/API fields use `snake_case`.
- `internal/bootstrap/server.go` opens the DB and injects the store into handlers and services.
- See also: [Server](server.md), [Configuration](config.md).
