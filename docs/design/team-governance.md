# Team Governance Foundation

## Status

- roadmap_priority: `P4`
- status: `partially_implemented` — roles, quota, and workflow lifecycle are
  shipped; audit/event visibility remains open
- follows: [enterprise-deployment.md](./enterprise-deployment.md)
- roadmap: [../ROADMAP.md](../../ROADMAP.md)
- created_at: `2026-05-17`

## 1. Decision

P4 should turn BuildMax's existing team controls into a practical governance
foundation for private team operation.

Several foundations already exist:

- team roles: `owner`, `admin`, `member`
- team-scoped quota service
- team usage endpoint
- centralized handler authorization helper
- workflow lifecycle: `draft`, `published`, `archived`
- workflow assignment and execution checks

The remaining work is not a giant enterprise policy platform. It is making the
existing controls visible, tested, and traceable enough that team admins can
trust shared automation.

## 2. Product Goal

Admins should understand:

- who can do what
- what shared resources are governed
- what capacity the team is using
- which workflows are draft, published, or archived
- which sensitive assets changed over time

The product should provide confidence without forcing users into a heavy admin
console.

## 3. Current Baseline

Backend anchors:

- roles in `internal/core/model/team.go`
- authz helper in `internal/server/handlers/team_authz.go`
- quota service in `internal/service/quota/service.go`
- quota routes in `internal/server/handlers/usage.go`
- workflow lifecycle in `internal/core/model/workflow.go`
- workflow lifecycle enforcement in `internal/service/workflow/service.go`
- team persistence in `internal/infra/db/team.go`
- default quota tier seeding in `internal/infra/db/quota_tier.go`

Frontend anchors:

- team settings in `portal/src/pages/settings/SpaceSettings.tsx`
- shared settings UI in `portal/src/pages/settings/shared.tsx`
- workflow pages in `portal/src/pages/workflows`
- issues assignment UI in `portal/src/pages/issues`

Current action model:

- `owner` manages team members.
- `owner` and `admin` manage agents, workflows, and workflow assignment.
- `owner`, `admin`, and `member` can run workflows.

## 4. Main Gaps

### 4.1 Governance Is Not Visible Enough

The backend has roles and checks, but the UI needs clearer admin-facing
explanation:

- what each role means
- why an action is unavailable
- which workflow states are runnable
- current team usage and quota limits

### 4.2 Permission Boundaries Need Broader Tests

Handler tests already cover some role behavior. P4 should make permission
boundaries systematic:

- members cannot mutate shared automation assets
- admins cannot manage ownership-sensitive membership actions
- owners can manage members
- workflow lifecycle restrictions apply consistently
- team-scoped resources cannot leak across teams

### 4.3 Workflow Lifecycle Needs Product Polish

The lifecycle exists, but it should be obvious in Portal:

- `draft`: editable, not assignable/runnable for shared work
- `published`: assignable and runnable
- `archived`: retained for history, not used for new work

The UI should avoid making users learn this by failed requests.

### 4.4 Sensitive Assets Are Not Traceable

BuildMax now has shared assets that affect team execution:

- webhook keys
- agent definitions
- workflows
- team members and roles
- quota tier assignment

There is no small audit/event model yet. P4 should define and implement the
smallest useful traceability slice.

## 5. In Scope

### 5.1 Team Quota UI And Documentation

Make quota visible where admins expect it:

- current team usage
- tier name
- rolling period
- run limit
- token limit
- over-limit behavior

Document:

- quota is team-scoped
- personal use is represented by the default personal team
- task creation/rerun checks the active team

### 5.2 Role And Permission Boundary Tests

Add a table-driven permission matrix for server handlers and services.

Minimum actions:

- manage team members
- create/update/delete agent
- create/update/publish/archive workflow
- assign issue to workflow
- run workflow
- read team resources
- create normal work

### 5.3 Workflow Lifecycle UI

Polish workflow list/detail copy and controls:

- show lifecycle badge
- disable unavailable actions with clear text
- hide archived workflows from default assignment choices
- keep archived workflows inspectable
- make publish/archive transitions explicit

### 5.4 Small Audit/Event Model

Define and implement a small append-only event model.

Recommended model:

```go
type TeamEvent struct {
	ID             uint   `json:"-"`
	EventID        string `json:"event_id"`
	TeamID         string `json:"team_id"`
	ActorUserID    string `json:"actor_user_id"`
	Action         string `json:"action"`
	TargetType     string `json:"target_type"`
	TargetID       string `json:"target_id"`
	Summary        string `json:"summary"`
	MetadataJSON   string `json:"metadata_json,omitempty"`
	CreatedAt      int64  `json:"created_at"`
}
```

Use a prefixed ID such as `ev_` through `internal/util.NewPrefixedID`.

Initial event actions:

- `team.member_added`
- `team.member_removed`
- `team.member_role_changed`
- `agent.created`
- `agent.updated`
- `agent.deleted`
- `workflow.created`
- `workflow.updated`
- `workflow.published`
- `workflow.archived`
- `webhook_key.created`
- `webhook_key.revoked`
- `quota.tier_changed` if quota tier editing exists

### 5.5 Event Visibility

Add a small Team Activity section in settings:

- latest 20-50 events
- actor
- action
- target
- time
- short summary

Do not build filtering, export, or compliance retention in the first slice.

## 6. Out Of Scope

- Custom roles.
- Policy DSL.
- Approval workflows.
- Per-agent or per-workflow permission lists.
- Immutable compliance archive.
- Audit export.
- Billing.
- Organization hierarchy.

## 7. Permission Matrix

Recommended starting matrix:

| Action | Owner | Admin | Member |
|---|---:|---:|---:|
| View team resources | yes | yes | yes |
| Create issue/conversation work | yes | yes | yes |
| Run assigned workflow | yes | yes | yes |
| Manage agents | yes | yes | no |
| Manage workflows | yes | yes | no |
| Assign issue to workflow | yes | yes | no |
| Manage team members | yes | no | no |
| Change member roles | yes | no | no |
| View team usage | yes | yes | yes |
| Change quota tier | yes | no | no |
| View activity events | yes | yes | no |

This matrix intentionally stays simple. If later enterprise customers need more
control, build from observed needs rather than inventing custom RBAC now.

## 8. Backend Plan

### M1. Permission Tests

- Add table-driven tests around `isRoleAllowed`.
- Expand handler tests for agents, workflows, issues, teams, webhook keys, and
  usage where action boundaries matter.
- Add cross-team access tests for sensitive reads and writes.

Acceptance:

- the permission matrix is enforced by tests

### M2. Governance Service Boundary

The current handler helper is acceptable for the first cut. If action checks
keep spreading, move the policy into a small service/package.

Target API:

```go
type TeamAuthorizer interface {
	Authorize(ctx context.Context, teamID, userID string, action TeamAction) (role string, err error)
}
```

Keep it boring: no policy DSL.

### M3. Team Event Store

Add model and store contracts:

- `TeamEvent`
- `TeamEventStore`
- `CreateTeamEvent`
- `ListTeamEventsByTeam`

Add GORM row with singular table name `team_event`.

### M4. Event Writes

Write events from successful sensitive actions.

Rules:

- write after the main mutation succeeds
- event write failure should be logged
- for MVP, event write failure should not fail the user action unless we decide
  traceability is mandatory for that action
- metadata should be compact JSON with snake_case keys

### M5. Event API

Add:

```text
GET /api/teams/{team_id}/events
```

Authorization:

- owner/admin can view
- member cannot view in the first slice

Response:

```json
{
  "events": [],
  "total": 0
}
```

## 9. Frontend Plan

### M1. Role Copy And Disabled States

Update Portal copy:

- team member role descriptions
- disabled action text for member/admin limits
- workflow lifecycle explanations

### M2. Team Usage Panel

In team settings, show:

- tier
- period
- runs used
- tokens used
- limits
- no-limit fallback when tier is unknown

### M3. Workflow Lifecycle UI

In workflow pages:

- badge state
- publish/archive actions for owner/admin
- unavailable run action for draft/archived
- assignment UI that defaults to published workflows only

### M4. Activity Section

In team settings:

- add "Activity"
- list recent team events
- use concise labels
- link targets when target route exists

## 10. Validation

Backend:

```sh
go test ./internal/server/handlers ./internal/service/quota ./internal/service/workflow ./internal/infra/db
```

Frontend:

```sh
cd portal && npm run build
```

Full:

```sh
./make test
```

Manual scenarios:

1. Owner can add/remove members.
2. Admin can manage workflows but cannot manage members.
3. Member cannot manage workflows or agents.
4. Published workflow can be assigned and run.
5. Draft/archived workflow cannot be assigned for new work.
6. Team usage is visible and matches active team.
7. Sensitive actions appear in Team Activity.

## 11. Risks

- **Too much governance too early**: avoid custom roles and approvals until
  basic traceability lands.
- **Audit noise**: only record meaningful sensitive actions in the first slice.
- **Action/event mismatch**: keep event action names stable and documented.
- **UI clutter**: make governance visible in settings and workflow pages, not
  everywhere.
- **Silent event failures**: log event write errors with enough context.

## 12. Open Questions

1. Should members be able to view Team Activity, or is it admin-only?
2. Should event writes be best-effort or required for sensitive actions?
3. Should workflow publish/archive require owner or allow admin?
4. Should quota tier changes be implemented in P4 or only documented?
5. Should webhook key creation/revocation require owner/admin only?

## 13. Recommended First PR

The first P4 PR should make existing governance explicit:

1. Add the permission matrix tests.
2. Polish team settings role/quota UI copy.
3. Polish workflow lifecycle UI states.
4. Add missing handler tests for forbidden role paths.

Then the second PR can add the small `team_event` model and Team Activity UI.
