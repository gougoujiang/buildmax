# System Administration

## Status

- roadmap_priority: `P4`
- status: `implemented` — M1–M6 are shipped: the grant model, the operator
  command, admin route authorization, the account routes, account disablement,
  the system status and redacted configuration routes, the cross-team audit
  search and team metadata routes, the Portal administration area, and the
  model catalog surface. What remains is in §17, and none of it is a gap in
  the first slice
- follows: [team-governance.md](./team-governance.md) and
  [enterprise-deployment.md](./enterprise-deployment.md)
- relates to: [enterprise identity and access](../proposals/enterprise-identity-and-access.md)
  proposal, which owns OIDC/SCIM and must not be pre-empted here
- roadmap: [../ROADMAP.md](../ROADMAP.md)
- created_at: `2026-08-18`

## 1. Decision

BuildMax gets one deployment-scoped principal, the **System Administrator**,
persisted as a revocable grant on a user and never derived from membership in a
Team.

The three alternatives were weighed and rejected:

- **Leave operator work in the CLI, the database, and the cluster.** This is
  where it is today. It makes routine account management require the database
  password, and it produces no accountable record of who did what.
- **Treat a Team owner as a global administrator.** This collapses the one
  boundary the product actually has. Team is the ownership boundary for
  issues, conversations, artifacts, and traces; an owner who can read across
  teams makes that boundary advisory.
- **Configure one static administrator email.** It bootstraps in one line and
  then fails at everything after the first day: no second admin, no
  revocation without a redeploy, and an audit trail that can only name a
  string from a config file.

A persisted grant is the only option that supports more than one admin,
revocation without a redeploy, and an audit record naming a real account.

The grant is an **authority to operate the deployment, not a key to its
contents**. A System Administrator can create and disable accounts, read
system status, search the audit trail across teams, and see team and quota
metadata. They cannot read a team's prompts, tool output, artifacts, files, or
run traces without being a member of that team. That is enforced by the same
team checks every other caller passes, and proved by a test, not by a promise
in this document.

## 2. Product Goal

An operator of a private deployment should be able to answer these without a
database client, a `kubectl exec`, or the JWT secret:

- Who has an account, when did they last sign in, and can they still sign in?
- Someone is leaving today — how do I stop their access now?
- Someone forgot their password — how do I get them back in?
- Is this deployment healthy, on which version, with which schema applied?
- Who changed access, models, or configuration, in which team, and when?
- Which teams exist, how large are they, and what are they using?

And they should be unable to answer, through this surface, "what is that team
working on".

## 3. Current Baseline

What exists today, with the anchors this design builds on:

- Team roles and the team authorization helper:
  `internal/core/model/team.go`, `internal/server/handlers/team_authz.go`.
- The single funnel for user identity on every JWT route: `requireAuth` in
  `internal/server/handlers/auth.go`. Every authenticated handler reaches a
  user id through it.
- Route ownership: `internal/server/handlers/routes.go`, with a coverage test
  in `team_authz_matrix_test.go` that fails when a team-scoped route has no
  authorization row.
- The append-only audit trail: `internal/core/model/audit.go`,
  `internal/service/audit`, `internal/infra/db/audit.go`, and the team-scoped,
  owner-only `GET /api/teams/{team_id}/audit-events`.
- Operator commands that already run with database credentials:
  `internal/bootstrap/user_admin.go` (`buildmax-server user create |
  set-password | login-code`) and `internal/bootstrap/model_admin.go`
  (`buildmax-server model add | list | enable | disable`).
- Sessions: `model.RefreshTokenStore` can revoke one session
  (`RevokeSession`), and the access token is a signed JWT the server never
  stores.
- Liveness, readiness, and version: `internal/server/health.go`,
  `readinessChecks` in `internal/bootstrap/server.go`, and
  `config.VersionString()`.
- Schema state: the `schema_migration` table in `internal/infra/db/migration.go`.

Five gaps follow from that list, and they are what the first slice closes:

1. There is no deployment-scoped principal at all. `system_admin` does not
   exist in any form, including a flag on `user`.
2. `buildmax-server user ...` writes **no audit events**. Account creation,
   password setting, and code issuance — the most sensitive actions an
   operator takes — leave no record. `model_admin.go` does record its changes,
   so this is an inconsistency rather than a policy.
3. Nothing disables an account. A user row exists or it does not.
4. Nothing lists or revokes a user's sessions. `docs/deploy/authentication.md`
   already says so: signing someone out today means deleting
   `user_refresh_token` rows by hand.
5. A team's quota tier is set once, from `default_quota_tier`, at team
   creation. `internal/infra/db/quota_tier.go` has `GetQuotaTier` and
   `SeedDefaultQuotaTiers` and no way to assign a different tier afterwards.

## 4. Principals And What Each May Reach

The threat model is a table of principals, because the failure this design has
to prevent is one principal quietly acquiring another's reach.

| Principal | Authenticated by | May reach | May not reach |
|---|---|---|---|
| **User** | Access token from a password or login code | Resources of teams they belong to, at their role | Anything in a team they are not in |
| **Team owner** | The same token, plus an `owner` membership row | Membership, shared automation, and the audit trail of *that* team | Any other team; any deployment-scoped surface |
| **System Administrator** | The same token, plus an active grant row | Accounts, grants, system status, redacted configuration, cross-team **metadata** and audit, model catalog state | Prompts, messages, tool output, artifacts, files, and run traces of teams they are not in |
| **Worker run** | A run token naming one run | The four `/api/worker/*` routes for that run | Every user route; every other run |
| **Infrastructure operator** | Database, cluster, and secret access | Everything, by construction | — |

Two consequences are worth stating rather than implying.

**A System Administrator is not the top of a ladder.** They are a principal
with a different axis of authority. A team owner has depth in one team; an
admin has breadth over the deployment's operation and no depth anywhere. The
authority is not additive with membership: an admin who is also a member of a
team gets exactly the member's reach in it.

**The infrastructure operator is still more powerful than the System
Administrator**, and this design does not change that. It reduces how often
anyone needs to *be* the infrastructure operator, which is the actual goal: an
operator who runs routine work through an audited surface stops handing out the
database password for it.

Two attacks shape the specifics below:

- **A compromised admin session.** The blast radius is bounded by what the API
  can return: metadata and status, never content. Every action it takes is in
  the audit trail with the actor's user id. The session can be ended by any
  other admin, or by the operator command, revoking that user's sessions.
- **A cross-team read attempt.** An admin calling a team content route without
  membership is refused by the existing team check, because the admin routes
  are a separate tree and the grant is never consulted by `authorizeTeamAction`.
  §11 makes that a test rather than an assertion.

## 5. Authority Model

### 5.1 One Role, Persisted As A Grant

```go
// internal/core/model/system_grant.go
const SystemRoleAdmin = "system_admin"

// SystemGrant is one deployment-scoped authority held by one user.
type SystemGrant struct {
    ID            uint   `json:"-"`
    SystemGrantID string `json:"system_grant_id"`
    UserID        string `json:"user_id"`
    Role          string `json:"role"`
    GrantedBy     string `json:"granted_by"`
    GrantedAt     int64  `json:"granted_at"`
    // RevokedAt is nil while the grant is active. Revoking sets it rather
    // than deleting the row: who held authority and when is the question an
    // investigation asks, and a deleted row cannot answer it.
    RevokedAt *int64 `json:"revoked_at,omitempty"`
}

type SystemGrantStore interface {
    // ActiveSystemRoles returns the roles userID currently holds. Empty for
    // almost every caller, so it must stay a single indexed read.
    ActiveSystemRoles(ctx context.Context, userID string) ([]string, error)
    ListSystemGrants(ctx context.Context, includeRevoked bool) ([]SystemGrant, error)
    GrantSystemRole(ctx context.Context, userID, role, grantedBy string, now int64) (*SystemGrant, error)
    // RevokeSystemRole revokes the active grant and reports whether one was
    // found. Revoking an absent grant is not an error.
    RevokeSystemRole(ctx context.Context, userID, role string, now int64) (bool, error)
    CountActiveSystemGrants(ctx context.Context, role string) (int, error)
}
```

Persisted as `systemGrantRow` on the singular table `system_grant`, with a
unique index on `(user_id, role, revoked_at)` so one user cannot hold two
active grants of the same role. `SystemGrantID` uses the new `sg_` prefix
registered in `internal/util/id.go` and
[contribute/conventions.md](../contribute/conventions.md).

Three shape decisions, each of which was the other way at some point:

- **A `role` column, not a boolean.** Question 1 of the proposal asked whether
  a read-only `system_observer` and a support role are needed now. They are
  not — nobody has asked for them, and inventing an unused role is the mistake
  §16 warns about. But the column costs one `varchar` today and is the
  difference between adding a role later and migrating a boolean later. Only
  `system_admin` is accepted by the store until a second role has a caller.
- **Not a flag on `user`.** A flag has no granting actor, no timestamp, and no
  history. Those three are the entire point.
- **No `superuser`.** A permission whose scope is not written down grows
  silently. Every admin route names the role it needs, and §11 fails the build
  if one does not.

### 5.2 How It Is Checked

A separate helper alongside the team one, never inside it:

```go
// internal/server/handlers/system_authz.go
func (h *Handler) requireSystemAdmin(w http.ResponseWriter, r *http.Request) (userID string, ok bool)
```

It calls `requireAuth`, then `ActiveSystemRoles`, and on refusal records
`access.denied` with an empty `team_id` before writing the response.

`authorizeTeamAction` is not modified, and must not be. A grant is not an
argument to a team check; if it ever becomes one, the boundary in §4 stops
being true and no test would notice.

### 5.3 Status Codes

- No credential, or an invalid one: **401**, same as every other route.
- A valid credential without a grant: **403**.

Not 404. Hiding the existence of `/api/admin/*` is not achievable — the routes
are in an open-source `routes.go` and in the Portal bundle — and pretending
otherwise costs the clarity of a correct status code. What must not leak is
data, and the 403 carries none. This is the same shape as the existing
owner-only audit route, which Portal already reads a 403 from as "not for
you" rather than as a bug.

## 6. Bootstrap And Lockout Recovery

The first grant comes from an operator command, not from configuration:

```text
buildmax-server admin grant <email>     # grant system_admin
buildmax-server admin revoke <email>    # revoke it
buildmax-server admin list              # who holds one, and who used to
```

It joins `user` and `model` under the same rule those two already follow: it
reads the same `server.yaml` the server reads, so it works unchanged in a
container or a pod, and it requires the database credentials — which is the
correct price for minting deployment authority.

This answers proposal question 2, and it answers it by **not** adding a
configuration value. A `system_admins:` list in `server.yaml` would be a second
source of authority that the audit trail cannot describe, that revocation
cannot reach without a redeploy, and that would have to win or lose against the
table in every edge case. One source, one story.

Recovery follows from the same choice: the command works whether zero or ten
admins exist, needs no running server, and needs no Portal account. There is no
break-glass credential to store, rotate, or leak, because the recovery path is
the ordinary path.

One invariant guards the gap between the two:

- The **API** refuses to revoke the last active `system_admin` grant, with a
  message naming the command. An admin cannot leave the deployment with none
  by clicking.
- The **command** allows it, and prints what it means. The operator whose
  admin left the company needs exactly that, and they already hold the
  credentials that make the restriction meaningless.

Self-revocation through the API is allowed when another admin remains. Refusing
it would only mean asking a colleague to do the same thing.

## 7. Server Administration API

All routes are `/api/admin/*`, all require `system_admin`, and none takes a
`team_id` path parameter — an admin route that looked team-scoped would invite
exactly the confusion §4 exists to prevent.

### 7.1 First Slice

| Route | Returns | Refuses to return |
|---|---|---|
| `GET /api/admin/me` | The caller's grant: role, granted at, granted by | — |
| `GET /api/admin/grants` | Active grants, and revoked ones on request | — |
| `POST /api/admin/grants` | Grants `system_admin` to a user id | A grant to an account that does not exist |
| `DELETE /api/admin/grants/{user_id}` | Revokes it | The last active grant (§6) |
| `GET /api/admin/users` | Accounts, newest first, `?q=` on email, paged | Password hashes, login codes, token values |
| `GET /api/admin/users/{user_id}` | One account: email, name, quota tier, last login and platform, `has_password`, `disabled_at`, team memberships with roles, active session count | Everything in the row above |
| `POST /api/admin/users` | Creates an account and its personal team | — |
| `POST /api/admin/users/{user_id}/login-code` | Issues a single-use code, shown once | A code that can be read back later |
| `POST /api/admin/users/{user_id}/disable` | Disables the account (§8) | — |
| `POST /api/admin/users/{user_id}/enable` | Re-enables it | — |
| `DELETE /api/admin/users/{user_id}/sessions` | Revokes every refresh session, returns the count | — |
| `GET /api/admin/system` | Version, commit, schema migrations applied, readiness checks and their status, worker runner mode, signup and sandbox settings, run counts by status | Anything with a credential in it |
| `GET /api/admin/config` | The effective configuration, redacted, plus computed warnings | Every secret — **presence only**. Not a length, not a prefix, not a hash: each of those narrows a search for someone who has the response and wants the secret |
| `GET /api/admin/teams` | Teams with member count, quota tier, personal-team flag, created at | Team contents of any kind |
| `GET /api/admin/teams/{team_id}` | The same, plus members and roles, plus usage against the tier | Issues, conversations, artifacts, files, traces |
| `GET /api/admin/audit-events` | The trail across every team, filtered by `team_id`, `actor_id`, `action`, `since`, `until`, paged | Anything the event does not already hold |
| `GET /api/admin/llm/models` | The catalog: name, provider, model, capabilities, enabled | `api_key`, in any form |
| `POST /api/admin/llm/models/{model_id}/enable` · `/disable` | Retires or restores a catalog model | — |

`POST /api/admin/users` returns the created account and **no credential**. An
operator who wants the person to sign in issues a login code as a second,
separately audited call. Creating an account and minting a way into it are two
decisions, and the CLI already separates them for the same reason.

### 7.2 What Is Not In It, And Why

- **Model creation and provider credentials.** `buildmax-server model add`
  stays the only way to put an API key into the catalog. Adding a model over
  HTTP means a provider credential in a request body, in a proxy log, and in
  whatever the browser did with the form. Enable and disable are the
  operational half and carry no secret, so they ship; `add` does not.
- **Configuration writes.** `GET /api/admin/config` is read-only, and the
  reason is mechanical rather than cautious: `server.yaml` is read at process
  start, and a multi-replica deployment has one file per replica. A write
  through Portal would change one replica's view of the world, or none.
  Configuration stays source-controlled until there is a config store, which
  is separate work. This answers proposal question 6.
- **Logs.** No log endpoint in this slice. This answers proposal question 4,
  and the answer is: raw logs stay in the deployment's own observability
  system. Building a redacted log viewer means owning a redactor whose failure
  mode is leaking a prompt or a key into a surface designed to be safe, and
  `GET /api/admin/system` plus the audit trail plus the existing run views
  cover the questions §2 lists without it. If evidence from a real incident
  shows a gap, that evidence names the specific field to add — which is a
  better input than a log viewer built on a guess.
- **Cross-team content.** Nothing here reads a prompt, an artifact, a file, or
  a trace. This answers proposal question 5 with "no, not in this slice". A
  support path that reaches team content needs its own design covering
  request, team consent, expiry, redaction, and a distinct audit action; it is
  not a parameter on a route in this table.
- **Quota tier assignment.** `GET /api/admin/teams/{team_id}` shows the tier
  and the usage against it. Changing it needs a store method that does not
  exist (§3, gap 5), and it is the one item here that is a feature rather than
  an operator's window into existing state. It lands in M6 or later, after the
  read surface has shown which teams actually need it.

## 8. Account Disablement Semantics

This answers proposal question 3, which needs a decision per affected
credential rather than one sentence.

A `disabled_at *int64` column on `userRow`, nil for an ordinary account.

| What | Effect of disabling | Why |
|---|---|---|
| **Password login** | Refused after the password verifies, with `account_disabled` and 403 | Checking before verification would tell an unauthenticated caller which addresses are registered. Checking after tells the truth to the one person who could already prove the account is theirs |
| **Login code** | Same, and any outstanding code is spent | A code issued before the disable must not be a way back in |
| **Refresh** | Refused; all sessions revoked at disable time | This is the credential that can actually be revoked, so it is revoked immediately |
| **Access token** | Refused on the next request | See below |
| **Webhook keys** | Refused at `POST /api/webhook` | The route already resolves the key's owner; the check is one field on a row it has |
| **Pending task runs** | Failed at dispatch, with the reason in `error_message` | Nothing has started, and leaving them queued means a disabled account's work starting after the disable. *Cancelled* was the original word here and turned out to name a status BuildMax does not have — see below |
| **Running task runs** | Left to finish | Killing one loses work the *team* owns, and the run's credential is already scoped to that run and already expiring. The team, not the departing user, is the party harmed by a kill |
| **Run tokens already minted** | Not revocable | A signature, not a row. Bounded by scope and by `worker.run_token_ttl`, as [deploy/authentication.md](../deploy/authentication.md) already documents |

The access token is the interesting one. It is a signed JWT the server does not
store, so "revoke it" means "stop honouring it", and the only place that can
happen is where the identity is resolved: `requireAuth`. Adding the check there
costs one primary-key read per authenticated request.

That cost is affordable and it is worth being precise about why: every
team-scoped route in the product already calls `ListTeamMembers` on every
request, which is a wider read than this one. A disabled check that lands in
`requireAuth` is strictly cheaper than work the same request already does. If a
profile ever says otherwise, the fix is a short-TTL cache in front of it — with
the honest note that a cache is a window in which a disabled account still
works. No cache in the first implementation.

The alternative — wait out the access token TTL, which defaults to seven days —
was rejected. "Disable this account" that means "in about a week" is not the
feature.

There is no `CANCELED` run status, and this design did not add one. Introducing
a terminal status is a run-lifecycle change: the scheduler, `syncTaskStatusFromRun`,
and every Portal badge would have to learn it, and a status that half the readers
do not handle is how a run stops explaining itself. What shipped instead reuses
the dispatch-time refusal already in the scheduler — the same path a run takes
when its credential cannot be minted — so the run reaches a terminal state and
says why in `error_message`. Whether these should be distinguishable from a
failure is open question 6.

The guard fails open on a store error: a database blip must not turn into a
team's work being refused. A run starting for an account disabled moments ago is
the smaller harm, since that run's credential is scoped to it and expiring and
the account's sessions are already gone.

Enabling reverses the state and nothing else. Sessions stay revoked, runs that
failed stay failed, and the person signs in again. Undo is not a goal.

## 9. Audit Events

New actions, permanent once written:

```go
AuditSystemAdminGranted = "system.admin_granted"
AuditSystemAdminRevoked = "system.admin_revoked"
AuditUserCreated        = "user.created"
AuditUserDisabled       = "user.disabled"
AuditUserEnabled        = "user.enabled"
AuditLoginCodeIssued    = "user.login_code_issued"
AuditSessionsRevoked    = "user.sessions_revoked"
AuditModelEnabled       // exists
AuditModelDisabled      // exists
AuditAccessDenied       // exists; reused for admin routes, with team_id empty
```

All are written with `team_id` empty, because none of them is team-scoped. The
`AuditEvent` shape already allows that — it was designed for `user.login` — so
no schema change is needed.

Two things fall out of adding these:

**The operator commands record.** `buildmax-server user create`,
`set-password`, and `login-code` write the same actions as their API
equivalents, with `actor_type: system` and `actor_id: "buildmax-server"` —
the actor id `model_admin.go` already writes, so one act does not acquire two
names. This closed gap 2 of §3 in M1. `model_admin.go` already does this; `user_admin.go` will match it. An
operator action that leaves no record is worse than one that names the machine
instead of a person, and naming the machine is the honest description of what
happened.

**The trail needs a reader that is not team-scoped.**
`ListAuditEvents(teamID, ...)` cannot return a login or a grant, because those
have no team. `GET /api/admin/audit-events` therefore needs a second store
method with optional filters:

```go
SearchAuditEvents(ctx context.Context, f AuditFilter, limit, offset int) ([]AuditEvent, int, error)
```

`AuditFilter` holds optional `TeamID`, `ActorID`, `Action`, `Since`, `Until`.
The existing owner-only team route keeps its narrow method: a team owner asks a
narrower question and must not accidentally acquire the wider one.

The failure policy does not change. A failed audit write is logged and dropped
rather than failing the action, exactly as `internal/service/audit` documents
today. That policy is decided rather than pending — see
[team-governance.md](./team-governance.md) §12 question 2 — and this design
does not reopen it. What it does is sharpen the one part still open: the
actions added here are where the argument for best-effort is weakest, because a
grant that was made and not recorded is the case an investigation most needs.
Whether a grant is the action that earns a transactional record is that
residue, and this design makes it more urgent rather than answering it.

## 10. Portal Administration Surface

A separate `/admin` area, not another tab in team settings. The separation is
the product statement: this is not something a team owner has more of.

- A new `admin` segment in `portal/src/router.ts`, with sections
  `overview`, `accounts`, `teams`, `models`, and `audit`.
- A new `portal/src/features/admin/` for the API client and the pages.
  It shares presentational components and shares nothing else with
  `features/audit/`, whose team-scoped client stays as it is.
- Visibility comes from `GET /api/admin/me`. A 403 means no navigation entry
  and no route: a non-admin who types `#/admin` gets the home page, not a
  forbidden screen, because there is nothing there to tell them about.
- Hiding the entry is presentation. The server refuses regardless, and §11 is
  what proves it.

Page order follows §2's questions rather than the resource list:

1. **Overview** — version, schema state, readiness checks, worker mode, run
   counts. The first page because it answers "is this thing all right".
2. **Accounts** — search, inspect, create, issue a code, disable, revoke
   sessions. The page an operator opens on a joiner or leaver day.
3. **Teams** — teams, sizes, quota tiers, usage. Explicitly labelled as
   metadata, with no link into team content, because a link that 403s reads as
   a bug rather than as a boundary.
4. **Models** — catalog state, enable and disable, with `add` documented as a
   command rather than hidden.
5. **Audit** — the cross-team trail with filters, and a stable link from any
   event to the account or team it names.

Every page states its scope in one line. "This shows team metadata, not team
content" is not decoration: an operator who assumes otherwise will eventually
report a missing feature that is actually the design.

## 11. The Authorization Matrix

The team route matrix in `team_authz_matrix_test.go` exists because the checks
live in handler helpers rather than in one middleware, so a route that forgets
to call one is not a compile error. Admin routes have exactly that property, so
they get exactly that treatment — a sibling `system_authz_matrix_test.go`:

1. **Every `/api/admin/*` route in `routes.go` has a row.** A route without one
   fails the build, and a row naming a route that no longer exists fails too,
   because a dead row reads as coverage. Shipped in M1, and verified by adding
   an unguarded route and watching it fail — a coverage test that has never
   fired is not known to work.
2. **Four callers drive every route as real requests**: a system admin, a team
   owner with no grant, an ordinary user, and an anonymous caller. Expected:
   200-ish, 403, 403, 401.
3. **A revoked grant is a non-admin.** The same user, after
   `RevokeSystemRole`, gets 403 on every row. This is what proves revocation is
   a live check rather than a startup read.
4. **A grant is not a team key.** A system admin with no membership drives the
   team-scoped content routes and gets 403 on every one. This is the test that
   makes §4's table true, and it is the one to look at first if anybody
   proposes consulting the grant inside `authorizeTeamAction`. M4 restates it
   at the surface that most looks like an exception: an administrator who can
   read a team's name, size, and quota still cannot open its issues.
5. **No admin response contains a secret.** A response-body assertion over the
   account and config routes for `api_key`, `password`, `hash`, `secret`, and
   `token`. Crude, and it catches the realistic failure: someone returns a row
   struct instead of a response struct. Shipped in M2, which is where the first
   route that could carry one appeared.

Items 1–4 shipped in M1, along with two the sketch did not name: a grant store
error denies rather than admits, and a refused admin request is recorded with an
empty team while an unauthenticated one records nothing — there is no actor to
name, and an event keyed by an unauthenticated request would let anyone write
rows.

The proposal's "evidence needed" list asked for an end-to-end authorization
matrix and a lockout exercise. Items 1–4 are the matrix. The lockout exercise
is a store-level test: grant, revoke through the API until one remains, watch
the API refuse, revoke the last one with the command, then grant again — all
without touching the database directly.

## 12. Out Of Scope

- A second or third system role, until one has a caller (§5.1).
- Organization hierarchy, custom roles, per-resource ACLs.
- OIDC, SAML, SCIM. The [identity proposal](../proposals/enterprise-identity-and-access.md)
  owns those. The only constraint this design places on it: an identity
  provider may *add* grants, and the operator command must keep working, so an
  IdP outage cannot lock a deployment out of its own administration.
- A log viewer, a log search, or anything SIEM-shaped (§7.2).
- Configuration writes (§7.2).
- Cross-team content access of any kind (§7.2).
- Audit export, retention policy, and deletion controls. They are open in
  [team-governance.md](./team-governance.md) §12 and this design does not
  change their answer.
- Billing, and any claim of strict spending control. The `llm_call` ledger
  records what was spent after the fact; reservations do not exist.

## 13. Backend Plan

### M1. Grant Model, Command, And Authorization — DONE

`model.SystemGrant` and `SystemGrantStore`, `systemGrantRow` on `system_grant`,
the `sg_` prefix, `buildmax-server admin grant | revoke | list`,
`requireSystemAdmin`, the audit actions, and the first route —
`GET /api/admin/me` — so that the matrix test had something to cover from the
start.

Acceptance met: a grant can be made, listed, and revoked from the command line;
a granted user gets 200 on `/api/admin/me` and an ungranted one gets 403; every
transition is in the audit trail.

Two things landed differently from the sketch above, both deliberate:

- **The audit actor is `buildmax-server`, not `cli`.** `model_admin.go` was
  already writing that string for the same kind of act, and a trail with two
  names for one actor is worse than a name that only says which binary ran.
  `system_grant.granted_by` carries the same string for the same reason.
- **`requireSystemAdmin` authenticates before it checks the store**, the
  reverse of `withUserAndStore`. Checking the store first would answer an
  anonymous caller with "system administration not configured", which says
  something about the deployment to someone who has proved nothing about
  themselves.

### M2. Accounts — DONE

The `/api/admin/users` routes, `disabled_at`, the login/refresh/webhook
refusals, session revocation, the scheduler guard, the `requireActiveUser`
check, and the account audit actions. The `/api/admin/grants` routes landed
here too rather than in M1: the last-grant invariant needs a route to enforce
it on, and account lifecycle and authority lifecycle are the same operator's
job.

Acceptance met: an operator can create an account, issue a code, disable it,
and see the disabled account refused on the next request — with each step in
the trail.

Three things landed differently from the sketch:

- **Pending runs fail rather than cancel**, for the reason in §8.
- **`requireActiveUser` refuses only an account that exists and is disabled.**
  A token naming an account the store does not have is allowed through
  unchanged. Nothing deletes accounts, so this is not a state a deployment
  reaches, and deciding what an unknown subject means on every route at once is
  a separate change from disablement.
- **An administrator cannot disable their own account.** They would lock
  themselves out mid-request with no way back except the operator command, for
  a mistake that is easy to make and pointless to allow.

### M3. System Status And Redacted Configuration — DONE

`GET /api/admin/system` and `GET /api/admin/config`. The readiness checks
bootstrap already registers are converted into the admin API's probe shape and
reported rather than re-invented, so the status page and `/readyz` cannot
disagree. Redaction is a whitelist of fields that may be shown, not a blacklist
of fields that must be hidden — a blacklist fails open on every field added
later.

Acceptance met, with three decisions worth recording:

- **The redacted view lives in `internal/config`**, as `ServerConfig.Redacted()`,
  not in the handler. When someone adds a configuration field, the decision
  about whether it may be shown is then in the file they are already editing.
  A test walks `ServerConfig` for fields whose names mark them as credentials
  and fails when one is unaccounted for, so the whitelist cannot silently fall
  behind the struct.
- **Version and deployment facts arrive from bootstrap.** `internal/server` may
  not import `internal/config` — the architecture test enforces it — and the
  same rule that keeps infrastructure detail out of the readiness endpoint
  keeps configuration detail out of this one.
- **`sandbox_surface` is reported empty**, because no worker path passes one.
  Reporting a boundary that is not applied would be worse than reporting none,
  which is the rule AGENTS.md already states for this field.

`GET /api/admin/system` degrades rather than failing: each of its reads is
best-effort, because a status page that answers 500 when one of its five
questions cannot be answered tells an operator nothing during exactly the
outage they opened it for. A failed dependency is named and not explained,
matching `/readyz` — connection errors carry DSNs, endpoints, and bucket names,
and the reason belongs in the log where an operator already has to be.

### M4. Cross-Team Audit And Team Metadata — DONE

`SearchAuditEvents` with `AuditFilter`, `GET /api/admin/audit-events`, and the
two team metadata routes. The existing team route is untouched, deliberately: a
team owner asks a narrower question, and handing that reader the wider method is
how a team-scoped route quietly acquires a deployment-scoped answer.

Acceptance met. Two details worth recording:

- **`team_id=none` is a filter value.** An empty `team_id` already means "any
  team", so the events no team-scoped reader can ever see — logins, grants,
  account actions — needed a spelling of their own to ask for.
- **`AuditFilter` has no free-text field, and should not grow one.** The trail
  holds who did what to which object; a text query would invite a `Detail LIKE`
  scan over a column whose whole purpose is to stay small and structured.

`AdminTeam` carries no field a member wrote or an agent produced. The pressure
on that boundary is real and will arrive as a reasonable-sounding request — an
issue count, then a list, then titles — so the test asserts the response
mentions no issue, conversation, artifact, task, workflow, or trace at all.

### M5. Portal `/admin` — DONE

The router segment, `features/admin/`, and four of the five pages in §10's
order. Models is the fifth and lands with M6, because there is nothing to show
until the catalog routes exist.

Acceptance met: an admin sees the area, a non-admin sees no entry and is sent
home rather than to a forbidden screen, and the server refuses either way.

Three things worth recording:

- **Every page states what it does not show.** The spaces page says members and
  capacity, not work; the account detail says spaces are listed by name and role
  only. This is not decoration — an operator who assumes otherwise eventually
  reports a missing feature that is actually the boundary.
- **There is no link from a space into its contents.** A link that 403s reads
  as a bug rather than as a boundary, so the page does not offer one.
- **The destructive actions say what they will do before they do it.**
  Disabling names every credential it stops, says sessions are revoked and
  queued work will not start, and says it is not deletion. A login code says it
  is shown once and recoverable nowhere. Discovering either afterwards is how
  an operator learns to distrust the surface.

Browser coverage is `portal/e2e/admin.spec.ts`, which needs the test account to
hold a grant — `./make e2e` now issues one alongside the login code.

### M6. Model Catalog Read And Toggle — DONE

`GET /api/admin/llm/models` and the enable/disable routes, reusing the audit
actions `model_admin.go` already writes. `add` stays a command.

Acceptance met: a model can be retired from Portal, the catalog response
contains no key, and the trail does not distinguish a CLI toggle from an API
one except by actor — a command names the binary, the route names a person.

One addition the sketch did not have: **the response says which aliases point
at each model**, and the page marks a model with none. Enabled and unreachable
is the most common reason an operator's model "does not work", and nothing else
in the product reports it. Aliases themselves stay configuration — changing one
is a deploy rather than a click, for the same reason §7.2 gives for the rest of
the configuration.

Quota tier assignment, if it is wanted, comes after M6 with its own store
method and its own audit action. It is the one item in §7.1 that was a feature
rather than a window onto existing state, and nothing has asked for it yet.

## 14. Frontend Plan

1. **Gate and shell.** `getAdminMe`, the `admin` route segment, and a nav entry
   rendered only on success. A 403 is an expected answer, not an error toast —
   the same convention `features/audit/api.ts` already documents.
2. **Overview.** Status cards from `GET /api/admin/system`, with a failed
   readiness check named and unexplained, matching what `/readyz` does and for
   the same reason.
3. **Accounts.** A searchable list, a detail panel, and confirm dialogs on
   disable and on session revocation that say what will happen to sessions and
   to pending runs. A login code is shown once, with that stated before it is
   generated rather than after.
4. **Teams.** Metadata table with the scope line from §10.
5. **Audit.** The cross-team trail reusing `features/audit/describe.ts`, which
   already renders an unrecognised action verbatim — the property that keeps a
   Portal older than its server from hiding events.
6. **Models.** Catalog table with enable/disable and the `add` command shown as
   copyable text.

## 15. Validation

Backend:

```sh
go test ./internal/server/handlers ./internal/infra/db ./internal/service/audit ./internal/bootstrap
```

Frontend:

```sh
cd portal && npm run build && npm test
```

Full:

```sh
./make check ci
```

Manual scenarios, each of which is a claim in this document:

1. Grant, list, and revoke from the command line against an empty deployment.
2. A granted user reaches `/admin`; a team owner without a grant does not.
3. A system admin calling a team content route without membership is refused.
4. Revoking a grant ends the admin's access on the next request, not at
   token expiry.
5. The API refuses to revoke the last grant; the command allows it and says so.
6. Disabling an account: an in-flight request fails, refresh fails, the
   password login says `account_disabled`, a webhook key is refused, pending
   runs are cancelled, a running one finishes.
7. Every step above appears in the cross-team audit trail with the right actor.
8. `buildmax-server user create` now appears in the trail as a system actor.
9. No admin response contains an API key, a hash, or a token.

## 16. Risks

- **The grant becomes a shortcut.** The first time a handler is hard to
  authorize, someone will reach for the grant inside a team check. §11 item 4
  is the test that fails, and it should keep the comment explaining why.
- **The admin API grows into an infrastructure console.** The proposal's
  non-goal is right: BuildMax should report its own state, not become a
  replacement for `kubectl`. Every addition should be checkable against §2's
  question list.
- **Metadata creep toward content.** "Just the issue titles" is metadata by one
  reading and content by another. The rule: if a team member wrote it or an
  agent produced it, it is content.
- **Bootstrapping through a config value.** It will be proposed again, because
  it is convenient in a Helm chart. §6 is the answer, and the cost is a second
  authority the trail cannot describe.
- **A best-effort audit write on a grant.** The weakest point in this design
  (§9). It is inherited rather than introduced: the best-effort policy itself
  is settled, and what should be raised again is the residue of team-governance
  question 2 — whether a grant is the action that earns a transactional
  record.
- **Disable is not delete.** Nothing here deletes an account or its data, and
  an operator who reads "disable" as "remove" will be wrong. Deletion belongs
  with team-governance question 10, which has no answer today.

## 17. Open Questions

1. Does a `system_observer` role have a real caller? The column exists (§5.1);
   the role should not, until someone needs to give a person status and audit
   without account control.
2. ~~Should `disabled_at` also block a personal team's existing task runs from
   being retried by a *teammate*?~~ **Decided: no**, by the principle §8
   already commits to for a running run — the team owns the work, and the
   departing user is not the party harmed by losing it. Disabling withdraws
   that account's ability to ask for work; it does not quarantine the backlog
   of a team whose remaining members are in good standing. The scheduler guard
   keys on the run's own `created_by`, so a rerun by someone else is a new run
   by a non-disabled actor and dispatches. What is genuinely unsettled sits a
   level down and is a team-model question rather than an administration one:
   nothing stops an owner adding members to a *personal* team, which is the
   only reason this case is reachable.
3. Does the admin API need rate limiting before it needs anything else? Login
   is unthrottled today ([deploy/authentication.md](../deploy/authentication.md)),
   and account enumeration through `GET /api/admin/users?q=` is a much smaller
   problem than that one — but it is a new surface with a search parameter.
4. What does an admin see for a deployment with no database? Every store is
   nil-able by design and the routes will answer 503; whether Portal should
   show the area at all in that state is unresolved.
5. When OIDC lands, does a grant follow the account or the identity? If an IdP
   subject is re-linked to a new user row, an unfollowed grant is a lockout and
   a followed one is a privilege transfer nobody approved.
6. Should a run stopped by a disable be distinguishable from one that failed?
   Today both are `FAILED` and only `error_message` separates them, so a Portal
   filter or a metric counts them together. The fix is a run-lifecycle change
   rather than an administration one — see §8.
7. Should an account's webhook keys survive a disable? They do today: the key
   is refused while the account is off and works again when it is back on,
   which keeps an operator from having to reissue keys to whatever is calling
   the webhook. If a leaver's integrations should die permanently, that is a
   different action from disabling.
8. Should model aliases become editable through the API? They are the last
   piece of the model story still requiring a deploy, and the argument against
   §7.2 makes — one replica's view of the world, or none — applies to them
   exactly. A config store would answer this and the configuration-write
   question together, and neither should be answered alone.
