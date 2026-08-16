# Team Governance Foundation

## Status

- roadmap_priority: `P4`
- status: `partially_implemented` — roles, quota, workflow lifecycle, the
  authorization matrix, and the first audit-trail slice are shipped; retention,
  export, and correlation remain open
- follows: [enterprise-deployment.md](./enterprise-deployment.md)
- roadmap: [../ROADMAP.md](../ROADMAP.md)
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

### 4.2 Permission Boundaries Need Broader Tests — RESOLVED

**This gap is closed.** A matrix drives real requests through the mux for an
owner, an admin, a member, a member of another team, and an anonymous caller,
covering every team-scoped route in `internal/server/handlers/routes.go`.
Driving requests rather than unit-testing the authorization helper is the
point: the role rules have more than one implementation, and a test of the
shared helper would pass while they drifted. A second test reads `routes.go`
and fails when a team-scoped route has no entry — and when an entry names a
route that no longer exists, because a dead row reads as coverage.

What it was built to prove:

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

### 4.4 Sensitive Assets Are Not Traceable — PARTLY RESOLVED

The audit trail described in §5.4 now exists, but it covers the identity and
model-catalog half of the list. Of the shared assets that affect team
execution:

- team members and roles — **recorded**
- the model catalog, which holds provider credentials — **recorded**
- webhook keys — not recorded
- agent definitions — not recorded
- workflows, including publish and archive — not recorded
- quota tier assignment — not recorded

The first slice was chosen for what a compromise costs rather than for how
often the asset changes: a membership change grants access to everything a team
holds, and a catalog change moves prompts and spending. The rest is additive —
each one is a `Record` call at the point of change plus a permanent action
string — and is the obvious second slice.

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

### 5.4 Small Audit/Event Model — SHIPPED, first slice

`model.AuditEvent` in `internal/core/model/audit.go` is the shipped shape. It
differs from what this section originally sketched in three ways that were
decisions, not drift:

- **`ActorType` plus `ActorID`, not `ActorUserID`.** A worker and the system
  itself take meaningful actions, and typing the actor was cheaper than
  inventing a user id for them.
- **No `MetadataJSON`.** A free-form JSON column is where prompts, request
  bodies, and credentials end up. `Detail` is a short non-sensitive note — a
  role name, a model alias — and nothing more.
- **A `TeamID` that may be empty.** A login is not team-scoped, and forcing one
  would have meant inventing a team for the event.

The event carries no prompts, no generated content, no tool output, and no
credentials: only who did what to which object. Run diagnostics live in the
durable run trace and per-call accounting in the `llm_call` ledger, because
those are different questions with different retention answers.

`AuditStore` is append-only by interface — there is no update or delete, since
a record that can be edited is not evidence. Action strings are persisted and
therefore permanent; renaming one rewrites history for every reader filtering
on it.

The actions that shipped are `user.login`, `team.member_added`,
`team.member_removed`, `llm_model.created`, `llm_model.enabled`,
`llm_model.disabled`, and `access.denied`. Two are worth stating for anyone
extending the list: a failed login is deliberately *not* recorded, because it
says nothing about who the actor was and would turn the trail into a place to
write arbitrary strings; and `access.denied` is the one action written on
failure, because a denial is what shows someone probing at a boundary.

A failed write is logged and dropped rather than failing the action that
triggered it. That is a real limit and is stated in `docs/start/support.md`:
the trail records what happened while the database was reachable, which is not
the same as guaranteeing every action was recorded. See open question 2.

The remaining actions from §4.4 are the second slice.

### 5.5 Event Visibility — SHIPPED

`GET /api/teams/{team_id}/audit-events` serves a team's trail newest-first with
limit/offset, and Portal renders it as an audit section in team settings
(`portal/src/features/audit/`).

It is **owner-only**, in the API and in the UI. The trail names who did what
including who was refused, which is administrative rather than collaborative
information — a member does not need to see that a colleague was denied
something. This answers what was open question 1.

Filtering, export, and compliance retention are deliberately still absent.

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

### M1. Permission Tests — DONE

Shipped as a route matrix rather than tests around `isRoleAllowed`, for the
reason given in §4.2: the helper is not the only implementation of the rules.
Every team-scoped route in `routes.go` has a row naming who may call it, the
rows are driven as real requests for five callers including a member of another
team, and a route without a row fails the build.

Acceptance met: the permission matrix is enforced by tests, and a new
team-scoped route cannot ship without someone deciding who may call it.

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

### M3. Team Event Store — DONE

Shipped as `model.AuditEvent` and `model.AuditStore`
(`RecordAuditEvent`/`ListAuditEvents`), with `auditEventRow` in
`internal/infra/db` on the singular table `audit_event`. The
naming landed on *audit* rather than *team event* because a login is not
team-scoped and the trail is evidence rather than an activity feed. See §5.4 for
the shape and for what was dropped from the sketch here.

### M4. Event Writes — DONE

Events are written after the mutation succeeds. A failed write is logged and
dropped, so a governance record never fails the action it describes — the
trade-off is stated in §5.4 and in `docs/start/support.md` rather than left for
an operator to discover during an investigation. The compact-JSON metadata rule
did not survive: there is no metadata column, only a short `Detail` string.

### M5. Event API — DONE

Shipped as:

```text
GET /api/teams/{team_id}/audit-events
```

Authorization is **owner-only**, narrower than the owner/admin sketched here,
for the reason in §5.5. Response is `{"events": [], "total": 0}` with
limit/offset paging.

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

### M4. Activity Section — DONE

Shipped in team settings as "Audit trail" (`portal/src/features/audit/`), with
concise labels and paging. A non-owner sees the section explain why it is empty
for them rather than seeing nothing, so the boundary is legible instead of
looking like a missing feature.

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

1. ~~Should members be able to view Team Activity, or is it admin-only?~~
   **Decided: owner-only**, narrower than either option. The trail records who
   was refused a request, and that is administrative information — see §5.5.
2. Should event writes be best-effort or required for sensitive actions? Still
   open, and now a shipped behaviour rather than a design choice: writes are
   best-effort. Making one required means deciding what the user sees when the
   action succeeded and the record did not.
3. Should workflow publish/archive require owner or allow admin?
4. Should quota tier changes be implemented in P4 or only documented?
5. Should webhook key creation/revocation require owner/admin only?

The remaining questions came from the retired *Audit and data governance*
proposal. Its recommended direction — an internal team ledger first, export
later — is what shipped; these are the parts that were not settled by shipping
it:

6. What retention applies to audit events, and is it configuration or an
   operational responsibility? Nothing expires today. `docs/start/support.md`
   says retention of artifacts, run state, and traces is the operator's to
   configure, but the audit table has no such answer and no operator control.
7. What correlation identifiers may connect a task, worker, model call, and
   artifact? The trail, the durable run trace, and the `llm_call` ledger are
   three separate records with no shared key, so an investigation that starts
   at an audit event cannot mechanically reach the run that caused it.
8. Does export need at-least-once delivery, and how would a consumer detect
   gaps? Deliberately deferred until the event shape stops changing, which the
   §4.4 second slice will do. A best-effort write (question 2) also means a gap
   is not always distinguishable from nothing having happened.
9. Who may read run traces, artifacts, and model usage? The audit trail's
   answer is settled; these three were never decided together, and they carry
   more than the trail does — a trace holds tool output.
10. Which deletion controls does a team get? There is no export or import
    command and nothing deletes a team's records, so "delete our data" has no
    answer beyond dropping the database and the bucket.

## 13. Recommended First PR

The first P4 PR should make existing governance explicit:

1. Add the permission matrix tests.
2. Polish team settings role/quota UI copy.
3. Polish workflow lifecycle UI states.
4. Add missing handler tests for forbidden role paths.

Then the second PR can add the small `team_event` model and Team Activity UI.
