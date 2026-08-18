# System Administration

> **Audience:** operators, contributors, and security reviewers · **Status:**
> proposal — under discussion

Related: [roadmap](../ROADMAP.md) P3 and P4; [enterprise deployment
design](../design/enterprise-deployment.md); [team governance
design](../design/team-governance.md); [enterprise identity and access
proposal](enterprise-identity-and-access.md); [deployment
authentication](../deploy/authentication.md); and [support matrix](../start/support.md).

## Problem And Current Context

BuildMax has team-scoped collaboration roles, but no deployment-scoped
administrator identity. `owner`, `admin`, and `member` are memberships in one
Team: they deliberately govern that team's people and shared automation, not
the Server, other teams, or a private deployment as a whole.

That leaves the operator functions outside the product:

- Accounts are created, given a password, and issued a recovery code with
  `buildmax-server user ...`, which runs with database credentials.
- Model catalog administration is a Server-side command, and deployment
  configuration is changed through `server.yaml`, Kubernetes, or the database
  rather than a deliberate Server/Portal boundary.
- Portal audit is owner-only and team-scoped. It cannot answer a deployment
  operator's question across teams.
- Server, worker, and Kubernetes logs are collected outside Portal. Run traces
  and artifacts are team-scoped on purpose, so an operator cannot inspect them
  through a documented, authorization-checked support path.
- Sessions can be renewed and logged out by their holder, but no system surface
  can list accounts, disable access, or revoke a user's sessions.

This is acceptable for a small trusted alpha deployment, but it prevents a
private operator from running the system without direct database, Kubernetes,
or credential access. It also encourages treating a Team owner as a global
administrator, which would collapse the product's ownership boundary.

## Goals

- Define a **System Administrator** as a deployment-scoped principal, separate
  from every Team role and never inferred from membership in a Team.
- Give authorized System Administrators a Server API and a clearly separated
  Portal administration area for the first operational jobs:
  - account lifecycle and access recovery;
  - deployment health, version, migration, worker, and safe operational-log
    status;
  - approved model catalog and deployment-level model policy visibility;
  - cross-team metadata, quota, and audit investigation;
  - explicit, safe-to-display system configuration state.
- Make every privileged action attributable, auditable, revocable, and covered
  by end-to-end authorization tests.
- Preserve Team as the resource and collaboration boundary, and preserve direct
  local CLI/Desktop use without a Server.
- Establish a bootstrap and lockout-recovery path that does not require a
  permanently privileged Portal account or an undocumented database edit.

## Non-Goals

- Making every Team owner or Team admin a System Administrator.
- A full organization hierarchy, arbitrary custom roles, SCIM, SAML, or OIDC.
  The identity proposal remains the place to decide those integrations.
- Exposing raw secrets, provider credentials, JWT material, passwords, or
  unredacted environment variables in Portal.
- Giving a System Administrator silent default access to team prompts, tool
  output, artifacts, files, or run traces. Cross-team content access, if ever
  needed for support, requires its own explicit, audited design.
- Replacing Kubernetes, database, object-store, or identity-provider consoles;
  BuildMax should report its own state and operate its own resources, not become
  a general infrastructure console.
- Building a generic log-search or SIEM product in the first slice.

## Options And Trade-Offs

| Option | Strength | Main concern |
|---|---|---|
| Keep operator work in CLI, database, and cluster tools | No new authorization model | Makes normal operations depend on broad infrastructure credentials and gives Portal users no accountable path |
| Treat Team owners as global administrators | Reuses an existing role | Violates Team isolation and creates accidental cross-team authority |
| One configured static admin email | Simple bootstrap | Poor rotation, revocation, multi-admin support, and audit semantics; unsuitable as the ongoing model |
| Persist a deployment-scoped System Administrator grant | Separates authority cleanly, supports multiple admins, revocation, and audit | Requires a new authorization boundary, bootstrap path, Server API, and Portal area |

The likely direction is the final option. A static bootstrap configuration or
operator command may seed the first grant, but the durable authority should be
a persisted, auditable grant rather than a special email address or Team role.

## Proposed First Slice

### Authority Model

- Introduce one initial global role, `system_admin`, attached to a user but not
  to a Team. Avoid a permissive `superuser` flag whose scope is never written
  down.
- Create a dedicated Server authorization helper and route group. Team-scoped
  handlers must continue to require team membership; a system grant is not a
  substitute for a team path parameter or a normal user request.
- Require an existing System Administrator for ordinary grants and revocations.
  Bootstrap the first grant through an explicit operator-only command or
  configuration contract, with a documented recovery procedure.
- Record grant, revocation, login, account-action, configuration-action, and
  support-access events in the append-only audit store. An audit write remains
  fail-open only where the underlying action would otherwise be unsafe to
  block; that trade-off must be documented per action.

### Server Administration API

The initial API should be narrow and versioned around BuildMax resources:

| Area | First capability | Explicit limit |
|---|---|---|
| Accounts | List, inspect, create, issue recovery code, disable/enable, and revoke refresh sessions | Never return password hashes, login codes, or active token values |
| System status | Server version, schema version, readiness dependencies, worker launch configuration, and aggregate run state | Not a replacement for cluster metrics or a shell |
| Models and quota | View and manage the operator model catalog, deployment aliases, and team quota assignments | Do not expose provider credentials or claim strict spending control before reservations exist |
| Audit | Search system-wide audit metadata with team and actor filters | No prompt, output, secret, or raw request-body retention |
| Logs and diagnostics | View bounded, redacted Server/Worker diagnostic summaries and link to a team-authorized run when applicable | No unbounded raw logs or implicit access to files, artifacts, traces, or tool output |
| Configuration | Show a redacted effective configuration and validation status | Secrets are presence/status only; configuration writes may remain deployment-managed initially |

Account disablement must have defined runtime semantics: deny new login and
managed calls immediately, revoke refresh sessions, and state how long an
already issued access token can remain usable. A system role cannot be removed
in a way that strands the deployment without the documented recovery path.

### Portal Administration Surface

Portal should add a separate `/admin` area, not another Team settings tab. It
should be visible only after Server authorization confirms `system_admin`.
The first navigation should favour operational questions over CRUD breadth:

1. System health and deployment diagnostics.
2. Accounts and access recovery.
3. Teams, aggregate usage, and quota status.
4. Models and policy status.
5. System audit trail.

Each privileged page must explain scope and retain a stable link to the event
or resource that produced an alert. A user who is not a System Administrator
should receive neither navigation nor data; hiding a button is not enforcement.

## Questions To Resolve

- Does the first deployment role need only `system_admin`, or are a read-only
  `system_observer` and a support role already required by real operators?
- Where is the first grant stored and how is it recovered when all admins are
  removed: a signed bootstrap command, a server configuration value, or a
  short-lived break-glass credential?
- What should account disablement do to pending and running task runs, webhook
  keys, and already-issued access tokens?
- Which logs are safe and useful to expose in Portal, and should raw logs stay
  exclusively in the deployment's existing observability system?
- Is cross-team run-trace or artifact access ever needed for support? If so,
  what request, approval, redaction, expiry, and audit model protects team
  data?
- Which deployment settings can safely become mutable through BuildMax, and
  which must remain source-controlled Kubernetes or `server.yaml` changes?
- How will a future OIDC/SCIM mapping provision and deprovision System
  Administrators without weakening the bootstrap recovery path?

## Evidence Needed For A Decision

- A threat model distinguishing System Administrator, Team owner, user,
  worker, and infrastructure-operator authority, including a compromised admin
  session and an attempted cross-team data read.
- An end-to-end authorization matrix that proves a Team owner cannot call an
  admin route, a System Administrator cannot bypass team content authorization,
  and an unauthenticated request sees neither data nor existence clues.
- A lockout and recovery exercise that creates, revokes, and restores an admin
  grant without direct database modification.
- A representative incident investigation showing which redacted status, log,
  audit, and run metadata the operator actually needed.
- Feedback from private deployment operators on account management, log
  visibility, and the smallest useful division of administrator duties.

## Likely Destination If Accepted

An accepted decision should add a deployment-administration design under
`docs/design/`, update P3/P4 sequencing in [ROADMAP.md](../ROADMAP.md), and
create bounded implementation Issues for the global grant model, bootstrap and
recovery, admin API authorization, Portal administration surface, audit events,
and diagnostic/log redaction. The proposal should then be deleted rather than
kept as parallel roadmap text.
