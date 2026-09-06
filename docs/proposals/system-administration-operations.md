# System Administration Operations

> **Audience:** contributors, operators, and security reviewers · **Status:** proposal — under discussion
>
> **Opened:** 2026-09-05

Related: [system administration design](../design/system-administration.md),
[space governance](../design/space-governance.md),
[space membership lifecycle](../design/space-membership-lifecycle.md),
[enterprise identity and access](enterprise-identity-and-access.md), and the
[roadmap](../ROADMAP.md).

## Contents

- [1. Problem](#1-problem)
- [2. Current Evidence](#2-current-evidence)
- [3. Decision Boundary](#3-decision-boundary)
- [4. Goals](#4-goals)
- [5. Non-Goals](#5-non-goals)
- [6. Correctness Prerequisites](#6-correctness-prerequisites)
- [7. Options](#7-options)
- [8. Recommended Product Shape](#8-recommended-product-shape)
- [9. Delivery Plan](#9-delivery-plan)
- [10. API And Domain Changes](#10-api-and-domain-changes)
- [11. Authorization, Privacy, And Audit](#11-authorization-privacy-and-audit)
- [12. Validation](#12-validation)
- [13. Rollout And Documentation](#13-rollout-and-documentation)
- [14. Open Questions](#14-open-questions)
- [15. Evidence Needed For A Decision](#15-evidence-needed-for-a-decision)
- [16. Likely Destination If Accepted](#16-likely-destination-if-accepted)

## 1. Problem

BuildMax already has deployment-scoped system administration. The accepted
[system administration design](../design/system-administration.md) is marked
implemented, `internal/server/handlers/admin` registers the administration API,
and Portal has an Administration area. The missing product is therefore not a
new superuser or a second authorization system.

The problem is that the existing capability does not yet form a complete,
discoverable operator journey:

- Administration appears only in the signed-in user's menu and only after
  `GET /api/admin/me` confirms a grant. An operator who has not bootstrapped a
  grant sees no hint that the area exists.
- The Server can list, grant, and revoke `system_admin`, but Portal has no
  interface for those routes.
- Account and Space lists fetch one fixed page of 50 records and expose no way
  to continue.
- The Server returns a redacted effective configuration, but Portal displays
  only its warnings.
- Account sessions can be counted and revoked only as a whole. An operator
  cannot identify or revoke one device.
- Space quota and usage are visible, but quota tiers cannot be assigned after
  creation.
- The deployment overview reports coarse status, not enough runtime metadata
  to distinguish an idle deployment from a blocked queue or a lost worker.
- Audit search supports time bounds in the API, while Portal exposes raw Space,
  actor, and action identifiers only.
- Operator documentation still says the Portal administration area is being
  built, although it exists.

The command line is a strength, not legacy surface to remove. It is suitable
for automation, repeatable deployment, initial bootstrap, and recovery when the
web application is unavailable. Portal serves a different caller: a human
operator handling routine work interactively. Today those two callers receive
different sets of outcomes even when the underlying service and API already
exist. That difference is the central gap this proposal closes.

These gaps make an implemented subsystem look absent and still send operators
back to the command line or database for ordinary work. Expanding the surface
also raises the value of the grant path, so correctness issues in that path
must be fixed before the UI makes it easier to use.

## 2. Current Evidence

### 2.1 Backend

`internal/server/handlers/admin/handler.go` registers the live source of truth
under `/api/admin/*`. The current routes cover:

| Area | Existing behavior |
|---|---|
| Authority | Read the caller's grant; list, grant, and revoke system roles |
| Accounts | Search, inspect, create, issue a login code, disable, enable, and revoke all sessions |
| Deployment | Read health, build/version facts, schema migrations, redacted configuration, and TaskRun counts |
| Spaces | Search Space metadata; inspect members, roles, quota tier, and aggregate usage |
| Models | List catalog entries and enable or disable one |
| Plugins | List and publish catalog entries, list and publish releases, archive, restore, and yank |
| Audit | Search the deployment-wide trail and export CSV or JSONL |

The system grant is checked by `access.Guard.SystemAdmin` on every request.
Revocation therefore takes effect on the next request rather than at token
expiry. The separate space authorization path does not consult a system grant,
and the authorization matrix proves that an administrator without Space
membership cannot read Space content.

The first grant and lockout recovery are intentionally command-line operations:

```text
buildmax-server user create <email>
buildmax-server admin grant <email>
buildmax-server user login-code <email>
```

There is no `system_admins` configuration setting. The database grant is the
only authority source, and the command that creates the first grant can recover
a deployment with none.

### 2.2 Portal

`portal/src/pages/admin/AdminSettings.tsx` exposes six sections:

1. Overview
2. Accounts
3. Spaces
4. Models
5. Plugins
6. Audit

The pages perform real reads and some mutations. Account creation, login-code
issuance, disablement, session revocation, model state changes, and plugin
catalog changes are already usable. `portal/e2e/admin.spec.ts` proves route
wiring, reload behavior, and deployment-only audit search, but it does not
exercise the destructive operator journeys.

### 2.3 Known Documentation Drift

`docs/deploy/authentication.md` still says the Portal area is being built and
later says session revocation requires direct database access. The code is the
fact: the Portal area and the administrator's revoke-all route exist. The
operator documentation is stale and should be corrected with the first
accepted implementation slice.

## 3. Decision Boundary

This proposal preserves the central decision in the existing design:

> A System Administrator operates the deployment. A Space membership authorizes
> access to the Space's contents.

A system grant may authorize account lifecycle, deployment health, capacity,
catalog state, and metadata-only operational actions. It must not authorize
reading prompts, conversation messages, Issue text, generated output, files,
artifacts, or run traces belonging to a Space whose member list does not contain
the administrator.

The System Administrator is not the top of a role hierarchy. It is a separate
axis from Space `owner`, `admin`, and `member`. A person who holds both receives
the union of two independently checked authorities; the system grant must never
be passed into `SpaceAction` or used as a fallback when Space authorization fails.

## 4. Goals

- Make the existing administration capability visible and understandable to
  the people who hold it.
- Establish outcome parity between the operator command line and Portal for
  routine administration: if an authenticated administrator can safely perform
  an operation, Portal should expose it.
- Preserve command-line operations as a stable automation surface rather than
  replacing them with browser-only workflows.
- Let the first administrator manage later administrators without returning to
  the machine that holds database credentials.
- Complete joiner, access-recovery, session, leaver, and quota-assignment
  journeys in Portal.
- Preserve an audited, recoverable bootstrap path that does not depend on the
  running Server or an external identity provider.
- Provide enough metadata to answer whether BuildMax itself is healthy and
  whether work is progressing, without turning Portal into a cluster console.
- Make every new authority edge safe under concurrent requests and prove that
  safety against MySQL.
- Keep secrets and Space-authored or Agent-produced content out of every admin
  response.
- Keep the API useful independently of Portal and keep handlers as adapters over
  authoritative services.

## 5. Non-Goals

- A universal superuser that bypasses Space membership.
- Custom roles, arbitrary permissions, or per-resource ACLs in this slice.
- OIDC, SAML, SCIM, MFA, and service accounts. The
  [enterprise identity proposal](enterprise-identity-and-access.md) owns those.
- Account hard deletion or Space deletion. Both require data-ownership,
  retention, and audit decisions first.
- Editing `server.yaml` from Portal. It is process-start configuration and has
  no shared multi-replica write target.
- A raw log viewer. Logs can contain endpoints, prompts, and credentials and
  remain the deployment observability system's responsibility.
- Kubernetes node, Pod, or infrastructure lifecycle management.
- Cross-Space support access. If needed, it requires a separate design for Space
  consent, expiry, revocation, redaction, and audit.
- Bulk destructive account or authority actions in the first delivery.
- Pixel-for-pixel or flag-for-field duplication between CLI and Portal. The
  required parity is the safe operator outcome, not identical interaction.

## 6. Correctness Prerequisites

The following are implementation defects or incomplete invariants, not product
options. They should be fixed before adding a Portal grant-management surface.

### 6.1 Enforce One Live Grant In The Database

`systemGrantRow` currently declares a unique index over
`(user_id, role, revoked_at)`, while `revoked_at` is `NULL` for a live grant.
MySQL treats values containing `NULL` as distinct for unique-index purposes.
The index therefore permits more than one row with the same user, role, and
`NULL` revocation time, contrary to the code comment.

`GrantSystemRole` first checks for an existing live row and then inserts outside
a transaction. Two concurrent grants can both pass the check, and the current
index does not guarantee that one loses.

Recommended correction:

- add a nullable live marker whose fixed non-`NULL` value denotes an active
  grant and whose `NULL` value denotes a historical grant;
- place the unique index on `(user_id, role, live_marker)`;
- clear the marker in the same update that sets `revoked_at`;
- keep every historical row;
- translate the duplicate-key result to `ErrSystemGrantExists`;
- prove concurrent grants against MySQL, not only a mock store.

An equivalent generated column is acceptable if it is expressed by the row
schema, works with the supported MySQL version, and is covered by migration and
schema tests.

### 6.2 Make Last-Holder Protection Atomic

`systemadmin.Service.Revoke` currently counts active grants and revokes in two
separate store calls. Two administrators can concurrently see two holders and
both revoke, leaving the deployment with none.

The store must expose one atomic operation that means:

> Revoke this role if doing so leaves at least one effective holder; otherwise
> return the last-holder refusal.

The implementation should serialize mutations for one system role inside a
database transaction. The operator command keeps a distinct force-capable path
because it is the recovery mechanism and already possesses database authority.

### 6.3 Count Effective Administrators

An unrevoked grant on a disabled account cannot authorize a request because
`Guard.ActiveUser` refuses the account first. Last-holder protection must
therefore count effective holders, defined as:

```text
unrevoked system_admin grant AND user.disabled_at IS NULL
```

The same invariant applies when disabling an account that holds
`system_admin`. Disabling may proceed only when another effective holder will
remain. The existing refusal to disable oneself remains useful but is not a
substitute for this invariant.

Granting a role to a disabled account should be refused with an actionable
conflict response. The operator can enable the account first.

### 6.4 Decide Transactional Audit For Authority Changes

Grant and revoke audit writes are currently best-effort and happen after the
authority mutation. The existing design identifies this as its weakest point:
the one event an investigation most needs may be the one that was dropped.

The recommended decision for this narrow class is to commit the grant mutation
and its audit event in one database transaction. This does not change the
general fail-open audit policy for ordinary product actions. It creates an
explicit stronger contract for changes to deployment authority.

The domain-facing mutation input should carry the actor and desired transition,
while GORM and transaction details remain in `internal/infra/db`.

## 7. Options

### Option A: Leave Administration As It Is

Keep Portal as a thin view and require CLI or database access for the remaining
operations.

Advantages:

- no new security-sensitive surface;
- no additional operational state;
- lowest implementation cost.

Costs:

- existing Server capabilities remain undiscoverable and partly unreachable
  from Portal;
- ordinary administration continues to require infrastructure access;
- account, session, quota, and runtime journeys remain incomplete;
- the product continues to appear to lack system administration.

### Option B: Complete The Existing Boundary Incrementally

Keep one `system_admin` role, close correctness gaps, expose existing routes,
then add narrowly scoped session, catalog, quota, and runtime metadata
operations. Every routine operator outcome receives both an automation-friendly
CLI/API path and an interactive Portal path unless an explicit security or
availability reason prevents it.

Advantages:

- reuses the grant, audit, API package, and Portal area already shipped;
- preserves the Space content boundary;
- breaks into reviewable changes with independent acceptance criteria;
- aligns with roadmap R3 account and Space operations and the wider operational
  trust milestone.

Costs:

- requires MySQL concurrency work before the most visible UI work;
- some runtime operations depend on the R1 single- versus multi-instance
  decision;
- one broad role remains powerful over account lifecycle.

### Option C: Introduce A Full Administrative RBAC Platform Now

Add observer, support, identity, catalog, quota, and runtime roles with granular
permissions before expanding the surface.

Advantages:

- can model larger enterprise operations and separation of duties;
- reduces the authority held by any one account when configured carefully.

Costs:

- invents roles without known callers;
- multiplies authorization and test-matrix states before the core operator
  journeys work;
- risks becoming a generic policy platform ahead of current roadmap priorities;
- creates migration and UX complexity with little deployment evidence.

### Recommendation

Choose Option B. Treat an observer role, support access, and administrative
separation of duties as later evidence-driven decisions. The `role` column
already leaves room for another role without forcing one into this slice.

## 8. Recommended Product Shape

Portal should present one deployment administration area, separate from Space
settings, with the following information architecture:

| Section | Operator question | Scope |
|---|---|---|
| Overview | Is BuildMax healthy and is work moving? | Deployment status and operational metadata |
| Administrators | Who can operate this deployment? | Active and historical system grants |
| Accounts | Who can sign in and where are they signed in? | Account lifecycle and session metadata |
| Spaces | Which Spaces exist and what capacity do they have? | Membership, quota tier, and aggregate usage only |
| Models | Which model upstreams may callers use? | Redacted catalog state and safe operational checks |
| Plugins | What may this deployment publish and install? | Catalog and release metadata |
| Audit | Who changed what and when? | Structured metadata events and exports |

For a confirmed administrator, Administration should be a first-level sidebar
destination rather than an item hidden inside the user menu. The server remains
the authority: hiding or showing navigation is presentation only.

The overview should show the caller's grant source and time so the user can
distinguish Space ownership from deployment authority. Every Space-oriented page
must continue to state that it shows metadata, not contents.

### 8.1 Operator Surface Contract

Operator surfaces are split by the authority a caller can present, not by
feature. Two are direct-authority break-glass; two are authenticated peers over
one Admin API:

| Surface | Primary use | Authentication | Availability |
|---|---|---|---|
| `buildmax-server` | Break-glass: bootstrap the first account and grant, recover a locked-out or zero-administrator deployment, mint a run token, and run the Server | Direct access to Server configuration, the database, and the deployment signing key | Works without Portal and without a healthy public Server |
| Admin API (`/api/admin/*`) | Stable programmatic contract for routine administration | User session plus a live system grant | Requires the Server |
| `buildmax admin` | Scriptable, automation-friendly routine administration | The same Admin API, reusing the `buildmax` client login and Server-address configuration | Requires the Server |
| Portal | Discoverable, guided routine administration | The same Admin API | Requires the Server and Portal |

`buildmax admin` and Portal are peer clients of one Admin API; neither reaches
the database. `buildmax-server` is not a routine administration surface: it
keeps only the operations that must sit next to the database or the signing
key — creating the first authority, recovering when no administrator can log in,
and minting a run token — plus running the Server itself. Every other operator
outcome moves onto the authenticated Admin API and is reached identically from a
script (`buildmax admin`) or a browser (Portal).

One domain service owns each mutation. The `buildmax admin` client and the HTTP
handlers are both thin adapters that reach that service through the Admin API,
while `buildmax-server` reaches the same services directly as a system actor.
Validation, state transitions, invariants, and audit vocabulary are shared
across all three.

The parity requirement is explicit:

| Operator outcome | CLI today | Portal today | Proposed Portal outcome |
|---|---|---|---|
| Create an account | `buildmax-server user create` | Available | Keep and improve the guided flow |
| Let a user claim or recover an account | `buildmax-server user login-code` | Available | Keep, with one-time display and delivery guidance |
| Set another person's password | `buildmax-server user set-password` | Not available | Do not copy; issuing a login code lets the person choose their own password and is the safer equivalent |
| List administrators | `buildmax-server admin list` | Not available | Add active and historical grant views |
| Grant an administrator | `buildmax-server admin grant` | Not available | Add account selection, confirmation, and immediate audit feedback |
| Revoke an administrator | `buildmax-server admin revoke` | Not available | Add ordinary revoke; keep final-holder force recovery CLI-only |
| List models | `buildmax-server model list` | Available | Keep the richer catalog view |
| Enable or disable a model | `buildmax-server model enable/disable` | Available | Keep |
| Add a model | `buildmax-server model add` | Not available | Add a write-only credential form after credential storage and transport are hardened |
| Create a plugin catalog entry | Admin API | Not available | Add the existing API operation to Portal |
| Publish a plugin release | `buildmax plugin publish` and Admin API | Not available | Upload a prepared archive through the existing streaming API |
| Mint a diagnostic run token | `buildmax-server run-token` | Not available | Keep CLI-only; exposing a bearer credential is not a routine management outcome |
| Change process-start configuration | Edit `server.yaml` and restart | Read-only warnings | Keep read-only until a shared dynamic configuration store exists |

The table records today's surfaces and the proposed Portal outcome. Under the
split above, every non-exception row is also delivered on `buildmax admin`, so
each routine outcome reaches three-way parity: Admin API, `buildmax admin`, and
Portal. The exception rows below are exactly the outcomes that stay on
`buildmax-server` because they precede or bypass the authority the Admin API
requires.

Exceptions must be narrow and explained at the point where an authenticated
surface would otherwise offer an action:

- the first grant and recovery from zero administrators cannot depend on an
  already authenticated administrator;
- final-holder force revocation is deliberately a database-authorized operator
  action;
- a diagnostic run token is a credential for direct worker-route diagnosis,
  not a normal Portal workflow;
- process-start configuration cannot truthfully be edited through one Server
  replica.

Model creation is no longer excluded merely because it carries a credential.
Portal can accept write-only secrets safely only after the model credential is
encrypted at rest, request bodies are excluded from application and proxy logs,
TLS is required at the deployment boundary, errors never echo the value, the
response omits it, and the browser clears it immediately after submission. If
those conditions are not met, `model add` remains visibly marked as unavailable
rather than silently delegated to a command snippet.

## 9. Delivery Plan

The `buildmax admin` client is not a separate phase. Each phase that adds an
Admin API capability adds its `buildmax admin` subcommand in the same slice, so
the automation surface never lags Portal. One discrete restructuring, sized with
Phase 1, trims `buildmax-server` to its break-glass set — first grant, recovery
login-code, final-holder force revoke, run-token, and running the Server — and
moves every routine `user`, `model`, and `admin` operation onto the authenticated
Admin API reached by `buildmax admin`. Because the repository is Alpha, this
replaces the old command placement rather than aliasing it.

### Phase 0: Grant Integrity And Recovery

Scope:

- correct the live-grant uniqueness constraint;
- make grant, revoke, and last-effective-holder enforcement atomic;
- protect account disablement with the same invariant;
- make authority audit transactional;
- add MySQL concurrency and recovery tests;
- retain and exercise the force-capable operator command.

Acceptance:

- 20 concurrent grants create exactly one live row;
- two concurrent administrators cannot both revoke the last two effective
  grants through the API;
- a disabled grantee does not satisfy the last-holder invariant;
- disabling or revoking cannot leave zero effective administrators through the
  API;
- the CLI can still deliberately revoke the final grant and grant it again;
- every committed grant transition has its matching audit event.

### Phase 1: Administrators And Discoverability

Scope:

- add an Administrators Portal section;
- expose existing grant list, create, and revoke routes through the Portal API
  client;
- list active grants by default and allow viewing revoked history;
- select an existing enabled account by email rather than requiring a user ID;
- show grant actor, time, state, and target account;
- add role actions to account detail;
- move Administration to first-level navigation for confirmed holders;
- show the caller's own grant on Overview;
- render the redacted effective configuration in a collapsible read-only view;
- correct stale operator documentation.

Acceptance:

- after the CLI creates the first administrator, all later administrator
  lifecycle actions can be completed through Portal;
- Portal explains and displays the last-holder refusal;
- a revoked administrator loses the area on their next request;
- a non-administrator sees no navigation and receives 403 from every admin
  route;
- no rendered or serialized response contains credential material.

### Phase 2: Account And Session Operations

Scope:

- replace the fixed account page with cursor or explicit offset pagination;
- filter by enabled state, system role, password state, last-login range, and
  platform;
- make account detail a stable address that survives reload;
- retain separate create-account and issue-login-code calls and audit events,
  but present them as a guided joiner flow;
- list live sessions with session ID, platform, creation time, last rotation,
  expiry, and revocation state;
- revoke one session or all sessions;
- present a leaver flow that explains disablement, session revocation, webhook
  refusal, and the effect on queued work;
- rate-limit login and sensitive administration mutations.

Acceptance:

- every account remains reachable when more than 50 exist;
- an operator can identify and revoke one device without disturbing another;
- a newly created account cannot sign in until a separate credential action;
- a disabled account is refused through password, login code, refresh token,
  existing access token checks, and webhook keys;
- login and administrative abuse limits are deterministic and tested.

Hard deletion remains outside this phase.

### Phase 3: Catalog Management Parity

Status: the model line is delivered — provider credentials are encrypted at rest
under the deployment key-encryption boundary, `POST /api/admin/llm/models`
accepts a model over the admin API with a write-only credential, and Portal has
a model-creation form. The plugin line (the plugin-entry and release-publication
items below) is deferred by decision: plugin catalog management stays on the
command line for now, so those items are not built.

Scope:

- encrypt managed-model provider credentials at rest using the deployment's
  existing key-encryption boundary before accepting them from Portal;
- add `POST /api/admin/llm/models` over the existing `llmcatalog.Service`;
- add a model-creation form covering the fields supported by
  `buildmax-server model add`;
- treat `api_key` as write-only: password-style input, no response field, no
  persisted browser state, no audit detail, and no error echo;
- verify that HTTP middleware, documented reverse-proxy configuration, traces,
  and error reporting do not record request bodies;
- expose the existing plugin-entry creation API in Portal;
- expose the existing streaming plugin-release publication API through an
  archive file upload with progress and size-limit feedback;
- retain the CLI path for directory packaging, scripting, and bulk publication;
- show a clear Portal explanation for the remaining CLI-only recovery and
  process-configuration operations.

Acceptance:

- an administrator can add the same valid model through CLI or Portal and the
  resulting catalog row has the same non-secret semantics;
- provider credentials are encrypted at rest and never returned by a read;
- a known test credential appears in no response, log, trace, audit event,
  rendered DOM after completion, or browser storage;
- an administrator can create a plugin entry and upload a prepared release
  archive through Portal;
- the release is inspected, digested, stored, and audited by the same service
  used by the command-line publisher;
- CLI automation continues to work unchanged.

Portal does not need to pack an arbitrary local directory. The browser accepts
a prepared archive, while the CLI remains the natural surface for turning a
working directory into that archive and publishing it in one command.

### Phase 4: Space Capacity And Quota Assignment

Scope:

- list quota tiers through the administration API;
- assign an existing tier to a Space;
- paginate and filter Spaces by personal/shared kind, owner, tier, and quota
  pressure;
- show runs, tokens, and storage against their respective limits;
- record `space.quota_tier_changed` with actor, Space, old tier, and new tier;
- remove the duplicate user-level quota tier if it has no remaining caller,
  leaving the Space as the authoritative enforcement boundary.

Acceptance:

- every Space remains reachable when more than 50 exist;
- an unknown tier is refused;
- the next quota check observes the newly assigned tier;
- concurrent assignments have a deterministic final value and complete audit
  history;
- the response contains no Space-authored or Agent-produced field.

Portal should assign existing tiers in this phase. Creating and editing tier
definitions is deferred until there is evidence that source-controlled or
seeded tiers are insufficient.

### Phase 5: Runtime Operations

This phase begins only after roadmap R1 decides whether the supported Server
topology is one replica or introduces shared coordination. A global-looking
dashboard assembled from one process's memory would be actively misleading.

Scope:

- report process start time, build identity, and current Server time;
- report TaskRuns by status and age, including oldest pending age;
- report counts of cancellation requests, stale-run candidates, and recent
  terminal outcomes;
- report worker heartbeat freshness and execution modes using persisted run
  metadata rather than an invented persistent worker entity;
- report database, object storage, and model-gateway health through bounded,
  redacted probes;
- report the number of Spaces near or above each quota dimension;
- group failures by a safe error class, never by raw error text.

Acceptance:

- the dashboard distinguishes no work, queued work, active work, and work that
  appears stuck;
- all reported values have deployment-wide semantics under the supported
  topology;
- one failed dependency does not make the entire status response unavailable;
- no prompt, trace, tool output, artifact metadata supplied by a member, raw
  error, DSN, endpoint credential, or provider key is returned.

Global dispatch pause, force-cancel, or cross-Space retry are not implicit parts
of this phase. Each changes user work and requires an explicit authority and
multi-instance consistency decision.

### Phase 6: Enterprise Identity Follow-On

After the enterprise identity proposal is accepted, administration may gain:

- OIDC connection state and callback diagnostics;
- SCIM provisioning and deprovisioning status;
- identity-link inspection without exposing provider tokens;
- administrator MFA or step-up authentication for destructive actions;
- a real `system_observer` role if an operations-only caller is identified;
- service-account lifecycle if unattended callers need a supported credential.

The operator command remains available across identity-provider outages and
must be included in every lockout exercise.

## 10. API And Domain Changes

### 10.1 Reused APIs

Phase 1 should use the existing authority routes rather than introduce aliases:

```text
GET    /api/admin/me
GET    /api/admin/grants?include_revoked=true
POST   /api/admin/grants
DELETE /api/admin/grants/{user_id}
```

The existing account, system, configuration, Space, model, plugin, and audit
routes remain the base of their corresponding pages.

### 10.2 Proposed Account Queries And Session APIs

The exact query representation is an implementation detail, but the public
capability should cover:

```text
GET    /api/admin/users?status=&system_role=&has_password=&platform=&cursor=
GET    /api/admin/users/{user_id}/sessions
DELETE /api/admin/users/{user_id}/sessions/{session_id}
DELETE /api/admin/users/{user_id}/sessions
```

The session response must never contain a refresh-token hash or plaintext. Its
purpose is to identify a login chain by safe metadata. `RefreshTokenStore`
needs a list method returning a domain session projection; GORM rows remain
inside `internal/infra/db`.

### 10.3 Proposed Quota APIs

```text
GET /api/admin/quota-tiers
PUT /api/admin/spaces/{space_id}/quota-tier
```

`internal/core/quota.TierStore` currently reads one tier only. It needs a list
operation.
The Space store needs a quota-tier assignment operation, while validation and
audit ownership belong in `internal/service/quota`. The handler should not
coordinate raw stores directly.

### 10.4 Proposed Catalog APIs

Model creation should use the same input validation and creation service as the
command line:

```text
POST /api/admin/llm/models
```

The request may contain a provider credential; the response must not. The model
store must replace the current plaintext credential column with an encrypted
representation before this route is enabled. Only the managed gateway's
credential reader may decrypt it.

Plugin entry creation and release publication already have server routes:

```text
POST /api/admin/plugins
POST /api/admin/plugins/{plugin_name}/releases
```

Portal should call those routes rather than add browser-specific aliases. The
release body remains a streaming archive. Provenance supplied by a browser
upload must identify itself as such rather than claiming Git metadata the
browser did not verify.

### 10.5 Proposed Runtime Read Model

Runtime status should be assembled from narrow readers owned by the domains
that already own the facts:

- `internal/core/task` for persisted TaskRun status and age aggregates;
- readiness probes supplied by bootstrap;
- `internal/core/quota` for aggregate capacity pressure;
- configuration for immutable deployment facts and its redacted projection.

Do not create a generic `AdminStore` exposing the complete database. The admin
handler package's narrow configuration is part of the privacy boundary.

### 10.6 Service Ownership

Transport handlers should authenticate, parse, call one authoritative service,
and serialize a dedicated response type.

- `internal/service/systemadmin` owns grant lifecycle and last-holder rules.
- `internal/service/identity` owns account and session lifecycle.
- `internal/service/quota` owns tier validation, assignment, and usage rules.
- `internal/service/llmcatalog` owns model catalog mutations.
- `internal/service/plugin` owns plugin catalog mutations.

Operator commands and HTTP handlers should delegate to the same service for the
same state transition. Their authority differs — database-holding system actor
versus signed-in administrator — but their business procedure must not drift.
Today this delegation is uneven: `admin grant`/`revoke` and `model add` already
call their owning service, while the `user` lifecycle commands and `model
list`/`enable`/`disable` still reach `internal/core` primitives or the store
directly and record audit inline. Converging every operator command onto its
owning service is part of this proposal's work, not a precondition it assumes.

The two CLIs reach these services differently on purpose. `buildmax-server`
does not call the public HTTP API: bootstrap and recovery must still work when
that API is unavailable, so it invokes the shared service methods directly as a
system actor. `buildmax admin` is the opposite — a pure Admin API client that
authenticates as the signed-in administrator and carries no database or
service-layer access of its own, exactly like Portal. Behavioral parity across
all three comes from the single owning service, not from either CLI duplicating
its rules.

## 11. Authorization, Privacy, And Audit

### 11.1 Route Authorization

Every route registered by the admin package must appear in the system
authorization matrix. For each route, tests drive:

1. an effective System Administrator;
2. a Space owner without a system grant;
3. an ordinary user;
4. a user whose grant was revoked;
5. a user whose account was disabled;
6. an anonymous caller;
7. a grant-store failure.

Expected behavior remains 401 for no valid identity, 403 for a valid but
unauthorized identity, and denial on store failure.

### 11.2 Response Boundary

Dedicated response structs remain mandatory. Admin routes must not serialize a
database row or a general internal model merely because it is convenient.

Automated assertions should reject fields or values associated with:

- passwords and password hashes;
- login and refresh tokens;
- API keys and secret values;
- raw configuration credentials;
- prompts, messages, instructions, and generated output;
- artifact storage keys or file contents;
- run trace content and raw error messages.

Write-only model credentials need value-based leak tests across the complete
request path. Merely checking response field names is insufficient: a decoder,
validator, logger, or error wrapper can leak the submitted value without naming
the field `api_key`.

Space names, account emails, membership roles, quotas, aggregate usage, run
statuses, timestamps, and opaque public identifiers remain acceptable
administrative metadata.

### 11.3 Audit Vocabulary

Existing permanent action strings remain unchanged. Proposed additions are
introduced only with their callers:

| Action | Target | Detail |
|---|---|---|
| `auth.session_revoked` | one session | platform or empty; never a token |
| `space.quota_tier_changed` | Space | old and new tier names in a bounded structured form |

The existing `user.sessions_revoked` continues to mean revoke-all. Runtime
reads do not need one event per dashboard request. Bulk audit exports remain
audited because they extract the evidence trail itself.

If dispatch pause or force-cancel is accepted later, each needs a distinct
action and must name whether its scope was deployment, Space, or run.

### 11.4 Sensitive Action Presentation

Portal must state the result before asking for confirmation:

- revoking a role removes administration but leaves login sessions intact;
- disabling an account refuses every account credential and revokes stored
  sessions, but deletes no data;
- a login code is displayed once and cannot be recovered;
- revoking a session does not invalidate an already issued access token unless
  the account is disabled;
- changing a quota affects later admission and does not terminate work already
  running.

## 12. Validation

### 12.1 Unit And Handler Tests

- service table tests for every successful and refused transition;
- handler tests for parsing, status codes, pagination, and response shapes;
- authorization-matrix coverage for every registered route;
- secret and Space-content response assertions;
- pure Portal tests for filtering, status labels, quota pressure, and audit
  descriptions.

### 12.2 MySQL Tests

The following require `./make test mysql`; mocks cannot prove them:

- concurrent grant uniqueness;
- concurrent last-holder revocation;
- disablement racing with revoke or grant;
- authority mutation and audit atomicity;
- quota assignment persistence and concurrent updates;
- session listing and single-session revocation queries.

### 12.3 Portal End-To-End Tests

Browser coverage should prove complete journeys rather than page existence:

1. the granted account sees first-level Administration navigation;
2. an ungranted account does not and is redirected from `#/admin`;
3. an administrator grants and revokes a second administrator;
4. Portal reports the last-effective-holder refusal;
5. an operator creates an account and issues a one-time login code;
6. an operator disables and re-enables the account;
7. one session is revoked while a second remains;
8. account and Space pagination works past 50 records;
9. a quota-tier change appears in usage and audit;
10. audit time filters and export use the visible filter state.
11. a model is added through Portal without its credential appearing after
    submission;
12. a plugin catalog entry and prepared release archive are published through
    Portal.

### 12.4 Boundary And Deployment Tests

- A System Administrator without Space membership receives the same refusal as
  any other non-member on every Space-content route.
- A metadata-only runtime page works for local-process and Kubernetes Job
  worker modes.
- Multi-instance runtime claims are tested only after the supported topology is
  decided and assembled.
- Bootstrap, revoke-last with the CLI, and re-grant recovery are exercised
  without direct SQL.

### 12.5 Handoff Checks

Run the narrow scope while iterating and the relevant repository checks before
handoff:

```bash
./make test ./internal/server/handlers/admin ./internal/service/systemadmin ./internal/service/identity ./internal/service/quota
./make test mysql
./make check portal
./make check docs
git diff --check
```

Portal-, worker-, or deployment-shaped runtime changes also need the
proportionate Compose or kind evidence described in
`docs/contribute/testing.md`.

## 13. Rollout And Documentation

The repository is Alpha and has no compatibility obligation for an incorrect
stored shape. The grant schema correction should therefore replace the wrong
constraint everywhere rather than layer a compatibility workaround over it.

Suggested implementation order:

1. grant schema and concurrency correctness;
2. Administrators Portal page and discoverability, the `buildmax admin` client,
   and the `buildmax-server` break-glass trim;
3. account pagination and session lifecycle;
4. model and plugin catalog parity, after credential hardening;
5. Space quota assignment;
6. runtime operations after the topology decision;
7. enterprise identity as a separate accepted plan.

Each routine capability from step 3 onward ships its Admin API route, its
`buildmax admin` subcommand, and its Portal surface together.

Each user-visible slice needs one changelog entry. Update:

- `docs/design/system-administration.md` with accepted durable decisions and
  shipped status;
- `docs/deploy/authentication.md` with current bootstrap, Portal, session, and
  recovery behavior;
- `docs/contribute/architecture/portal.md` for the resulting Portal surface;
- `docs/contribute/architecture/data-model.md` for grant, session, or quota
  schema changes;
- `internal/server/static/openapi.json` for the exact live route surface;
- `docs/ROADMAP.md` only after the work is accepted and prioritized.

Do not retain this proposal after the direction is accepted or rejected. Move
durable rationale into the existing design record and let git history preserve
the discussion.

## 14. Open Questions

1. Should a grant to a disabled account be refused, as recommended, or stored
   as dormant authority that becomes effective on enablement? Dormant authority
   is harder for an operator to see and reason about.
2. Should grant and revoke be the first actions whose audit writes are
   transactional, or should BuildMax keep one best-effort policy even for
   authority changes?
3. Should account and Space lists retain offset paging for consistency with the
   existing API or move to cursor paging before deployments become large?
4. Which session metadata is sufficiently useful without becoming a device
   fingerprinting surface? Platform and timestamps exist; IP address and user
   agent do not.
5. Should the existing deployment key-encryption key encrypt managed-model
   credentials directly, or should the model catalog reference a new
   deployment-scoped secret resource?
6. Are quota-tier definitions deployment configuration or mutable Server data?
   This proposal recommends assigning existing tiers first and deferring tier
   editing.
7. What runtime aggregates are actionable to an operator without revealing
   Space content?
8. Does any known operator need read-only deployment status strongly enough to
   justify `system_observer`, or should one role remain until a caller exists?
9. Should a System Administrator be allowed to stop a clearly runaway run
   without Space membership? If yes, what metadata may they inspect first, how
   is the Space informed, and how does it interact with worker confirmation?
10. Which destructive actions require recent re-authentication or MFA after the
    enterprise identity direction is chosen?

## 15. Evidence Needed For A Decision

- Operator interviews or deployment reports showing which current CLI or
  database operations interrupt ordinary administration.
- A reproduced MySQL concurrency test for duplicate live grants and
  last-holder revocation.
- A browser walkthrough with more than 50 accounts and Spaces.
- A joiner, forgotten-password, leaver, and second-administrator exercise by an
  operator who did not implement the feature.
- A completed CLI/API/Portal outcome matrix showing that every routine operator
  action is present on both automation and human surfaces or has one documented
  exception.
- A model-creation leak test covering response bodies, logs, traces, audit
  events, DOM state, and browser storage.
- A runtime incident or drill demonstrating which metadata would have shortened
  diagnosis without requiring raw logs or Space content.
- A threat review of session metadata, transactional authority audit, and any
  proposed operational mutation.
- A topology decision from roadmap R1 before accepting deployment-wide runtime
  semantics.

## 16. Likely Destination If Accepted

Acceptance should not create a parallel System Administration design. Instead:

- add grant correctness and the chosen operator-surface decisions to
  `docs/design/system-administration.md`;
- place the agreed sequencing in `docs/ROADMAP.md`, with the early phases
  naturally supporting R3 account and Space operations;
- create focused implementation Issues or pull requests for each phase;
- keep enterprise identity decisions in their own proposal and later design;
- delete this proposal once its durable decisions have moved.
