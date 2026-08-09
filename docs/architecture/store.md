# Store

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
- quota_tier / usage aggregation
- webhook key

Table names are singular per project convention.

## Key Boundaries

| Layer | Package | Role |
|-------|---------|------|
| Contracts/entities | `internal/core/model` | Shared structs and repository interfaces |
| GORM implementation | `internal/infra/db` | MySQL-backed store implementing those interfaces |
| Object storage | `internal/infra/objectstore` | Team files and run artifacts, local FS or S3/MinIO |

## Notes

- Public entity IDs use prefixed IDs such as `u_`, `tm_`, `c_`, `t_`, and `r_`.
- JSON/API fields use `snake_case`.
- `internal/bootstrap/server.go` opens the DB and injects the store into handlers and services.
- See also: [Server](server.md), [Configuration](config.md).
