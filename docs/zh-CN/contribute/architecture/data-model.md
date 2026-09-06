# Data Model

> **翻译说明：** 本文是[英文原文](../../../contribute/architecture/data-model.md)的简体中文派生翻译。**同步依据：** 英文原文 SHA-256 `2f755407bdfc4e8bba79bfd79206ed50640391559852d944e74b1086317db686`。**同步状态：** 与该版本一致。若中英文存在语义冲突，以英文原文为准。
> **简体中文：** [阅读中文镜像](data-model.md)
> **Audience:** contributors · **Status:** current

The full relational schema of the BuildMax server database: every table, every
column, and the rules for changing them. Read this before touching anything
under `internal/infra/db`.

For the layering around persistence — which package owns contracts versus the
implementation — see [store.md](store.md). For why the entities are shaped this
way, see [../../../design/product-vision.md](../../../design/product-vision.md) and
[../../../design/space-governance.md](../../../design/space-governance.md).

## Where The Schema Lives

There is no `.sql` file describing the current schema. The source of truth is
the set of unexported `xxxRow` structs in `internal/infra/db`, and their GORM
tags. `New` in `internal/infra/db/store.go` calls `AutoMigrate` over all 31 of
them at server startup, so the running database is whatever those structs say.

The CLI and Desktop surfaces do not use this database at all. Sessions, traces,
and settings are files under `<BUILDMAX_HOME>`; see
[session.md](session.md). Everything below exists only in a server deployment.

## Conventions

These hold for every table. They are not repeated in the per-table sections.

**Two identifiers, one role each.** `id` is an auto-increment
`bigint unsigned` primary key. It is the relational key: every reference in
this document joins on it, and it never leaves `internal/infra/db`. `public_id`
is the handle every boundary sees — an API path, a JWT claim, a log line, an
object key — and it stores 96 bits of crypto-random data as its canonical text
form: 20 lowercase base32 characters (`ivyoh5qcfu6ypfkhyedq`) in a
`char(20) CHARACTER SET ascii COLLATE ascii_bin` column, written in the tables
below as `char(20) ascii_bin`. Storing the text keeps a direct `SELECT`
readable; `ascii_bin` keeps comparison memcmp, and the store writes only the
canonical lowercase form. A read rooted at a handle resolves it once through
the unique index and is numeric after that. Why, and which tables have a
handle at all, is in
[../../../design/entity-identity.md](../../../design/entity-identity.md) — the
storage form is its §17 amendment.

**Not every row has a handle.** A join row, a revision, and a catalog record
are addressed by something else: `space_member` by its pair, `agent_revision`
and `workflow_revision` by parent plus revision number, `plugin` by name, and
`plugin_release` by name plus version. Those tables have no `public_id`.

**Some references stay text.** A column ending in `_id` is a `bigint unsigned`
reference unless it is polymorphic, externally owned, or a value rather than a
reference — an audit actor whose type column admits an operator, an assignee
that may be a person or an agent or a workflow, a provider's tool-call ID, an
agent session that names a file. Each one is called out in its table below, and
the full list with its reasons is in `internal/architecture`, where a test
fails when a reference is added as text without one.

**Session IDs are not handles.** `task.session_id` and `task_run.session_id`
are `varchar(36)` UUIDs pointing at a session file under the run's
`BUILDMAX_HOME` rather than at any table. `user_refresh_token.session_id` is a
different thing again: an `as_`-prefixed login chain, carried as a claim in
every access token issued under it.

**No database-level foreign keys.** No row struct declares a GORM relation, so
`AutoMigrate` emits no `FOREIGN KEY` constraints, and a test in
`internal/architecture` fails when one does. Every reference described in
this document is a plain indexed column that application code is responsible
for keeping consistent. Deleting a parent row does not cascade, and a numeric
reference must not be read as implying that it would. That is decided rather
than deferred: [entity identity](../../../design/entity-identity.md) §8 reviewed
the store's deletion semantics — no hard delete removes a referenced parent —
and leaves the constraints to the change that first ships a real deletion
feature, where the order has to be written down anyway.

**Timestamps are `DATETIME(6)` columns**, never `TIMESTAMP`, never an integer,
and never `DATE` unless the value genuinely has no time of day. In Go they are
`time.Time`, and on the wire RFC 3339. Columns tagged `autoCreateTime` /
`autoUpdateTime` are filled by GORM; the rest are set explicitly with
`time.Now().UTC()`. A nullable timestamp is a `*time.Time`, and its absence is
meaningful — `ended_at IS NULL` means still running, not unknown, and there are
no sentinel zeros. Every connection speaks UTC: `db.New` forces `loc=UTC` and a
`time_zone` of `+00:00` on whatever DSN it is given, so a `DATETIME` written by
the server and one read by an operator's shell are the same instant. Durations,
quotas, and counts are not instants and stay `bigint`, with the unit in the
name. The reasoning is in
[timestamp representation](../../../design/timestamp-representation.md).

**Nullability is narrower than the Go type suggests.** `AutoMigrate` only emits
`NOT NULL` where a tag says so. A non-pointer Go field without `not null` maps
to a nullable column that application code never actually writes `NULL` into —
it writes the zero value. The `Null` column below reports the *database*
constraint; treat "yes" on a non-pointer field as "nullable in DDL, empty
string or 0 in practice".

**Names.** Table names are singular. Columns and JSON fields are `snake_case`.
Enumerated values are stored as strings, not integers, and the constants live in
the domain's `internal/core/*` package — the spelling is inconsistent by table
and is called out in each section (`task_run.status` shouts, `issue.status`
does not).

## Entity Relationships

The work graph — what a user actually creates and runs:

```mermaid
erDiagram
    space ||--o{ issue : scopes
    space ||--o{ agent : scopes
    space ||--o{ conversation : scopes
    space ||--o{ workflow : owns
    space ||--o{ task : scopes

    conversation ||--o{ conversation_message : contains
    conversation ||--o{ task : "spawns (tier 1 to tier 2)"
    conversation ||--o{ workflow_run : drives

    issue ||--o{ issue : "breaks down into (2 levels)"
    issue ||--o{ issue_comment : "discussed in"
    issue ||--o{ task : "tracked by"
    issue ||--o{ workflow_run : "tracked by"

    agent ||--o{ task : executes
    agent ||--o{ workflow_step_run : "targeted by"
    agent ||--o{ agent_revision : "versioned by"
    workflow ||--o{ workflow_revision : "versioned by"

    plugin ||--o{ plugin_release : "published as"
    space ||--o{ plugin_activation : activates

    task ||--o{ task_run : "attempted as"

    space ||--o{ artifact : keeps

    workflow ||--o{ workflow_run : "instantiated as"
    workflow_run ||--o{ workflow_step_run : "expands to"
    workflow_step_run ||--o| task : "delegates to"
```

Identity, authorization, and platform tables:

```mermaid
erDiagram
    user ||--o{ space_member : "joins via"
    space ||--o{ space_member : "joins via"
    user ||--o| space : "has personal"
    space ||--o{ space_invitation : offers
    user ||--o{ space_invitation : "is invited by"
    quota_tier ||--o{ user : rates
    quota_tier ||--o{ space : rates
    user ||--o{ user_webhook_key : owns
    user ||--o{ login_code : "authenticates with"
    user ||--o{ user_refresh_token : "keeps sessions in"
    user ||--o{ system_grant : "holds deployment authority via"
    llm_model ||--o{ llm_call : serves
    task_run ||--o{ llm_call : attributes
```

Space is the authorization boundary: a request is allowed because the caller has
a `space_member` row for the resource's `space_id`. Issue is the primary
user-facing work object. Conversation owns foreground chat and may create or
project a Task. Task plus task_run is the durable Agent execution plane and its
result is authoritative without a Conversation. The current non-null relation
below is implementation debt; the target ownership and continuation model are
in [Agent execution and Task threads](../../../design/agent-execution-and-task-threads.md).

## Identity And Authorization

### `user`

One row per person. Created by an operator; self-registration is disabled by
default (see [../../deploy/authentication.md](../../../deploy/authentication.md)).

| Column | Type | Null | Notes |
|---|---|---|---|
| `id` | `bigint unsigned` | no | Auto-increment primary key, internal |
| `public_id` | `char(20) ascii_bin` | no | Public handle, unique |
| `email` | `varchar(255)` | no | Unique; the login identifier |
| `name` | `varchar(255)` | yes | Display name |
| `password_hash` | `varchar(255)` | yes | argon2id, PHC-encoded. `NULL` until a password is set |
| `password_set_at` | `datetime(6)` | yes |  |
| `quota_tier` | `varchar(64)` | yes | References `quota_tier.tier_name` |
| `last_login_at` | `datetime(6)` | yes |  |
| `last_login_platform` | `varchar(32)` | yes | Where the last login came from |
| `disabled_at` | `datetime(6)` | yes | Non-`NULL` means every credential this account holds is refused |
| `created_at` | `datetime(6)` | yes | `autoCreateTime` |

Indexes: PK `id`; unique `email`; unique `public_id`.

`disabled_at` is read on every authenticated request, which is why it is a
column on this row rather than a side table: the check has to be one
primary-key read. Disabling is not deletion — nothing is removed, and enabling
clears the column and nothing else. What each credential does about it is in
[../../../design/system-administration.md](../../../design/system-administration.md)
section 8.

`last_login_at` and `last_login_platform` are written by the login handler,
which calls `UpdateLoginMeta` once the token pair is issued. A failure there is
logged and does not fail the login: the person is signed in either way, and
losing one timestamp is not worth refusing them. They record the most recent
login only — the login audit trail is `audit_event`, which keeps every one.

`password_hash` is nullable and read only by the code that verifies a login,
through `identity.PasswordStore` rather than as a field on `identity.User`. It never
rides along on a user object, so no handler can serialize it by accident.
Nullable is also what leaves room for an account authenticated somewhere else:
an identity provider, when there is one, needs no local password to exist.

### `space`

The ownership and authorization boundary for every Portal resource.

| Column | Type | Null | Notes |
|---|---|---|---|
| `id` | `bigint unsigned` | no | Internal primary key |
| `public_id` | `char(20) ascii_bin` | no | Public handle, unique |
| `name` | `varchar(255)` | no | Display name |
| `personal_for_user_id` | `bigint unsigned` | yes | Set on a user's personal space; unique, so a user has at most one |
| `quota_tier` | `varchar(64)` | yes | References `quota_tier.tier_name` |
| `plugin_curation` | `varchar(16)` | no | Default `'open'`; `open` or `curated`, see `plugin_activation` |
| `agent_instructions` | `text` | yes | Space-level instructions appended to every background Agent run; empty means no layer |
| `agent_instructions_revision` | `bigint` | no | Advances whenever `agent_instructions` changes; starts at 0 |
| `default_sandbox_network_tier` | `varchar(64)` | yes | Tier an agent that declares no network tier inherits; empty means none, see `agent` |
| `default_sandbox_filesystem_tier` | `varchar(64)` | yes | Filesystem counterpart of `default_sandbox_network_tier` |
| `created_by` | `bigint unsigned` | no | `user.id` |
| `created_at` | `datetime(6)` | yes | `autoCreateTime` |
| `updated_at` | `datetime(6)` | yes | `autoUpdateTime` |

Indexes: PK `id`; unique `personal_for_user_id`; unique `public_id`.

Every user gets a personal space named `My Space`
(`space.DefaultPersonalName`). It is a real space row, not a special case in
the authorization code, which is why quota and membership work identically for
solo and shared use.

That arrangement is deliberate and load-bearing. Before spaces existed, issues,
agents, and conversations hung off `user_id`, and a Portal request resolved as
`JWT -> user_id -> store query -> ownership check`. Space replaced that in April
2026 — before the public history was squashed, so `git log` does not show the
transition — with one rule: every working resource belongs to a space, and a
solo user simply owns a space of one. The point was to make sharing a
membership change rather than a data migration.

Two consequences bind new code:

- **Do not add a user-scoped path around a space-scoped resource.** A handler
  that resolves ownership from `user_id` alone reintroduces the model Space
  replaced, and it will diverge from quota, membership, and every authorization
  check that reads `space_member`.
- **Solo users must never have to learn the concept.** The personal space is
  created for them and named for them; surfacing space selection, invitations,
  or roles on a path a single user must walk is a regression, not a feature.

### `space_member`

The membership join table, and the row every authorization check looks for.

| Column | Type | Null | Notes |
|---|---|---|---|
| `id` | `bigint unsigned` | no | Internal primary key |
| `space_id` | `bigint unsigned` | no | `space.id` |
| `user_id` | `bigint unsigned` | no | `user.id` |
| `role` | `varchar(32)` | no | `owner`, `admin`, or `member` |
| `created_at` | `datetime(6)` | yes | `autoCreateTime` |

Indexes: PK `id`; unique `uq_space_member_space_user` on (`space_id`, `user_id`).

Roles are `space.RoleOwner` / `RoleAdmin` / `RoleMember`. The column
is `NOT NULL` but accepts the empty string, and `core/space.EffectiveRole` reads
such a row as a member: the row is what says somebody belongs to the space, and
member is the least the three roles can mean. Nothing writes one — the space
service defaults an unset role before storing it — so that reading exists for
rows a release before the default may have left behind. Space
approvals are planned but not implemented. The audit trail is implemented in
`audit_event`; neither approval nor audit state belongs in this membership row.

### `space_invitation`

A pending offer of space membership against an account that already exists.
See [space membership lifecycle](../../../design/space-membership-lifecycle.md).

| Column | Type | Null | Notes |
|---|---|---|---|
| `id` | `bigint unsigned` | no | Internal primary key |
| `public_id` | `char(20) ascii_bin` | no | Public handle, unique |
| `space_id` | `bigint unsigned` | no | `space.id` |
| `user_id` | `bigint unsigned` | no | `user.id` of the invited account |
| `role` | `varchar(32)` | no | `member` or `admin`; never `owner` — see `SetMemberRole` |
| `invited_by` | `bigint unsigned` | no | `user.id` of the sender |
| `expires_at` | `datetime(6)` | no | Three days from creation by default (`space.InvitationTTLDefault`) |
| `accepted_at` | `datetime(6)` | yes | Non-`NULL` means claimed; mutually exclusive with `revoked_at` |
| `revoked_at` | `datetime(6)` | yes | Non-`NULL` means withdrawn before acceptance |
| `created_at` | `datetime(6)` | yes | `autoCreateTime` |

Indexes: PK `id`; unique `public_id`; index `space_id`; index `user_id`.

There is no `status` column. `accepted_at`, `revoked_at`, and `expires_at`
are the whole state, the same shape `user.disabled_at` and
`system_grant.revoked_at` already use for "off until proven otherwise".
`corespace.Invitation.Pending` reads all three together.

The row never carries a code or a code hash: unlike `login_code`, a space
invitation targets an account that can already authenticate on its own, so
there is nothing to issue or deliver. Accepting one
(`AcceptInvitation`) is atomic with creating the resulting `space_member`
row — an invitation marked accepted with no membership to show for it would
be evidence of a bug no caller could act on.

### `system_grant`

One deployment-scoped authority held by one user, attached to no space. This is
the only table in the schema that grants anything outside a Space, and it grants
operation of the deployment rather than access to its contents — see
[../../../design/system-administration.md](../../../design/system-administration.md).

| Column | Type | Null | Notes |
|---|---|---|---|
| `id` | `bigint unsigned` | no | Internal primary key |
| `public_id` | `char(20) ascii_bin` | no | Public handle, unique |
| `user_id` | `bigint unsigned` | no | `user.id` |
| `role` | `varchar(32)` | no | `system_admin` is the only value this build accepts |
| `granted_by` | `varchar(64)` | no | Opaque: a user's handle, or `buildmax-server` when the operator command made the grant — the same string the matching audit event carries |
| `granted_at` | `datetime(6)` | no |  |
| `revoked_at` | `datetime(6)` | yes | `NULL` while the grant is in force |

Indexes: PK `id`; index `granted_at`; unique `idx_system_grant_live` on
(`user_id`, `role`, `revoked_at`); index `user_id`; unique `public_id`.

Nothing deletes from this table. Revoking sets `revoked_at`, so the row stays
as the record that the authority existed and when it ended. The unique index
includes `revoked_at` on purpose: MySQL treats `NULL`s in a unique index as
distinct, which leaves at most one live grant per (user, role) while allowing
any number of retired ones alongside it.

`role` is a column rather than a boolean so a second deployment role can be
added without a migration. Only roles `model.ValidSystemRole` accepts are
stored, so the column cannot become a way to invent authority.

### `login_code`

Single-use email login codes. Rows are consumed, not deleted on use.

| Column | Type | Null | Notes |
|---|---|---|---|
| `id` | `bigint unsigned` | no | Internal primary key |
| `code_hash` | `varchar(128)` | no | Hash of the emailed code, unique — the plaintext is never stored |
| `user_id` | `bigint unsigned` | no | `user.id` |
| `expires_at` | `datetime(6)` | no | default TTL is one hour (`model.LoginCodeTTLDefault`) |
| `used_at` | `datetime(6)` | yes | Non-`NULL` means already redeemed; a second attempt fails |
| `created_at` | `datetime(6)` | yes | `autoCreateTime` |

Indexes: PK `id`; unique `code_hash`; index `expires_at`; index `user_id`.

### `user_refresh_token`

The stored half of a login. Signing in returns a signed access token, which the
server keeps no record of, plus a refresh token, which is a row here. That split
is what makes a session revocable: the credential that lives for weeks is the
one the server can retire.

Each row belongs to a `session_id` — one login chain. Every exchange spends the
presented token and issues a new one in the same session, so revoking a session
retires the chain however many times it has been renewed.

| Column | Type | Null | Notes |
|---|---|---|---|
| `id` | `bigint unsigned` | no | Internal primary key |
| `token_hash` | `varchar(128)` | no | Hash of the token, unique — the plaintext is returned once and never stored |
| `user_id` | `bigint unsigned` | no | `user.id` |
| `session_id` | `varchar(64)` | no | `as_` prefix; one login chain, preserved across every rotation |
| `platform` | `varchar(32)` | yes | Which surface logged in — a label for the reader, not enforced |
| `expires_at` | `datetime(6)` | no | default TTL is 30 days (`model.RefreshTokenTTLDefault`) |
| `used_at` | `datetime(6)` | yes | Non-`NULL` means already exchanged |
| `revoked_at` | `datetime(6)` | yes | Non-`NULL` means retired by a logout or a reuse report |
| `replaced_by` | `varchar(128)` | yes | Hash of the token issued in exchange; lets an operator walk a chain back to its login |
| `created_at` | `datetime(6)` | yes | `autoCreateTime` |

Indexes: PK `id`; index `expires_at`; index `session_id`; unique `token_hash`;
index `user_id`.

A token presented after it was already exchanged means two holders, so the
store revokes the whole session rather than guessing which one is legitimate.
The exception is a short grace window after an exchange, which exists because
the CLI and Desktop share one credentials file between processes and refreshing
twice at once is normal there. Rows are swept once they expire; revoked rows are
kept until then, so a reuse report still has a chain to inspect.

### `user_webhook_key`

API keys for the inbound webhook surface documented in
[../../reference/webhook.md](../../../reference/webhook.md).

| Column | Type | Null | Notes |
|---|---|---|---|
| `id` | `bigint unsigned` | no | Internal primary key |
| `public_id` | `char(20) ascii_bin` | no | Public handle, unique |
| `user_id` | `bigint unsigned` | no | `user.id` — keys are user-scoped, not space-scoped |
| `key_hash` | `varchar(128)` | no | Unique; the secret is shown once at creation and never again |
| `name` | `varchar(255)` | yes | Human label |
| `created_at` | `datetime(6)` | yes | `autoCreateTime` |

Indexes: PK `id`; unique `key_hash`; index `user_id`; unique `public_id`.

### `quota_tier`

Rate limits, referenced by name from `user.quota_tier` and `space.quota_tier`.
This is the one table whose primary key is not `id`.

| Column | Type | Null | Notes |
|---|---|---|---|
| `tier_name` | `varchar(64)` | no | Primary key |
| `max_runs_per_period` | `bigint` | no | Task runs allowed per window |
| `max_tokens_per_period` | `bigint` | no | Prompt plus completion tokens per window |
| `period_days` | `bigint` | no | Window length |

Indexes: PK `tier_name`.

`SeedDefaultQuotaTiers` inserts `free_trial` (10 runs, 100,000 tokens, 30 days)
and `pro` (1,000 runs, 10,000,000 tokens, 30 days) at startup, but only when
the table is empty — an operator who edits a tier will not have it overwritten
on restart.

There is deliberately **no usage table**. `SpaceUsageInWindow` aggregates on
read: it counts `task_run` rows joined to `task` by space, sums their
`prompt_tokens` and `completion_tokens`, and adds the title-generation tokens
recorded on tasks created in the same window. Metering therefore has no second
write path that can drift out of sync with the runs themselves.

### `audit_event`

Governance evidence: that an action happened and who performed it. Append-only —
there is no update or delete path in `internal/infra/db/audit.go`, because a
record that can be edited is not evidence.

| Column | Type | Null | Notes |
|---|---|---|---|
| `id` | `bigint unsigned` | no | Internal primary key |
| `public_id` | `char(20) ascii_bin` | no | Public handle, unique |
| `space_id` | `bigint unsigned` | yes | Empty for actions with no space, such as a login |
| `created_at` | `datetime(6)` | no |  |
| `actor_type` | `varchar(16)` | no | `user`, `worker`, or `system` |
| `actor_id` | `varchar(64)` | no | User ID, or a process name for `system` |
| `action` | `varchar(64)` | no | `user.login`, `user.logout`, `user.password_set`, `auth.refresh_reuse`, `space.member_added`, `llm_model.created`, `access.denied`, … |
| `target_type` / `target_id` | `varchar(32)` / `varchar(64)` | yes | What the action was performed on. Opaque: the type admits a permission name and a model name as well as a row |
| `detail` | `varchar(255)` | yes | A short non-sensitive note — a role name, a model name |

Indexes: PK `id`; index `action`; index `actor_id`; index `idx_audit_space_time`
on (`space_id`, `created_at`); unique `public_id`.

Action strings are persisted and therefore permanent: renaming one rewrites
history for every reader that filters on it. They are declared in
`internal/core/audit/audit.go`.

**No prompts, generated content, tool output, or credentials.** This table
answers a governance question; run diagnostics belong to the durable run trace
and per-call accounting to `llm_call`. Recording the same fact in two places
would give it two retention policies and two chances to disagree.

Writes go through `internal/service/audit`, which logs a failed insert rather
than failing the action that caused it. That is a deliberate trade with a real
cost: the table records what happened while the database was reachable, not
every action that occurred. A deployment needing the stronger property has to
make the write part of the same transaction as the action.

### `schema_migration`

One row per applied migration. It is the record of what has been done to a
database, and the reason each migration runs at most once.

| Column | Type | Null | Notes |
|---|---|---|---|
| `id` | `varchar(191)` | no | Primary key; the migration's permanent identifier. 191 is MySQL's longest indexable `varchar` under `utf8mb4` |
| `applied_at` | `datetime(6)` | no |  |

Indexes: PK `id`.

Rows are never deleted. A missing row means that migration runs again, which is
what makes recovery from a crash mid-migration work and what makes deleting a
row a way to corrupt a database.

The table also tells a binary that the database is ahead of it: an ID here that
the binary does not know is a migration from a later release. See
[Forward Only, One Release Back](#forward-only-one-release-back).

## Work Objects

### `issue`

The primary user-facing work object.

| Column | Type | Null | Notes |
|---|---|---|---|
| `id` | `bigint unsigned` | no | Internal primary key |
| `public_id` | `char(20) ascii_bin` | no | Public handle, unique |
| `user_id` | `bigint unsigned` | no | Owning user |
| `space_id` | `bigint unsigned` | yes | Owning space; the authorization key |
| `parent_issue_id` | `bigint unsigned` | yes | `issue.id` of the parent; `NULL` for a top-level issue |
| `title` | `varchar(255)` | no | |
| `description` | `text` | no | |
| `status` | `varchar(32)` | no | `todo`, `in_progress`, `done` |
| `assignee_kind` | `varchar(32)` | yes | `person`, `agent`, or `workflow` |
| `assignee_id` | `varchar(64)` | yes | Interpreted according to `assignee_kind`: a `user_id`, `agent_id`, or `workflow_id` |
| `created_by` | `bigint unsigned` | no | `user.id` |
| `version` | `bigint unsigned` | no | Optimistic-concurrency token, starts at 1 |
| `created_at` | `datetime(6)` | yes | `autoCreateTime` |
| `updated_at` | `datetime(6)` | yes | `autoUpdateTime` |

Indexes: PK `id`; index `parent_issue_id`; index `idx_issue_space_updated` on
(`space_id`, `updated_at`); index `user_id`; unique `public_id`.

`version` makes every update conditional. An update carries the version it was
built from, the store writes with `WHERE public_id = ? AND version = ?` and sets
`version = version + 1`, and a caller whose version no longer matches gets
`coreissue.ErrVersionConflict` — a 409 — instead of overwriting a change it
never read. There is no unconditional update path: an update with no version
fails, because the zero value matches no row. `updated_at` was not reused for
this; it is a display and ordering value whose exact round trip through RFC 3339
is not something a correctness check should rest on.

The `assignee_kind` / `assignee_id` pair is a polymorphic reference — no index
or constraint ties it to a specific table, so validation lives in
`internal/service/issue`.

`parent_issue_id` is a self-reference forming an adjacency list, and the
hierarchy is capped at **two levels**: a parent must itself have
`parent_issue_id IS NULL`. Nothing in the schema enforces that — the invariants
live in `internal/service/issue`, which also rejects a parent in another space, a
self-parent, and giving a parent to an issue that already has children. Progress
(`child_count`, `done_child_count`) is computed per response with a grouped
query and never stored. See
`internal/service/issue` for the authoritative validation.

### `issue_comment`

One statement about an issue, addressed to people.

| Column | Type | Null | Notes |
|---|---|---|---|
| `id` | `bigint unsigned` | no | Internal primary key |
| `public_id` | `char(20) ascii_bin` | no | Public handle, unique |
| `issue_id` | `bigint unsigned` | no | `issue.id` |
| `author_kind` | `varchar(16)` | no | `user`, `agent`, `local_agent`, or `system` |
| `author_id` | `varchar(64)` | no | `user_id` or `agent_id`; the reporting person for `local_agent`; empty for `system` |
| `body` | `text` | no | Markdown source, stored raw; capped at 16 KiB by the service |
| `source_task_id` | `bigint unsigned` | yes | Set on an `agent` comment; never on a `local_agent` one, which names no run |
| `source_task_run_id` | `bigint unsigned` | yes | Set on an `agent` comment; never on a `local_agent` one, which names no run |
| `created_at` | `datetime(6)` | yes | `autoCreateTime` |
| `edited_at` | `datetime(6)` | yes | `NULL` until the body is changed |

Indexes: PK `id`; index `idx_issue_comment_issue_created` on (`issue_id`,
`created_at`); unique `public_id`.

Ordering is by `created_at`, then `id` — a thread reads oldest first, and a
public handle is random rather than time-ordered.

The row carries **no `space_id`**. A comment's space is its issue's space, and
every handler already loads the issue to authorize; denormalizing the
authorization key would give it a second place to be wrong. This follows
`conversation_message`, which resolves its space through its conversation.

Deletion is hard — there is no `deleted_at` and no tombstone. Editing is
restricted to the person who wrote the comment; an agent or system comment is
the record of what a run reported and is editable by nobody, though a space owner
may delete one.

### `agent`

A stored agent definition: a name plus system instructions that a task can run
under.

| Column | Type | Null | Notes |
|---|---|---|---|
| `id` | `bigint unsigned` | no | Internal primary key |
| `public_id` | `char(20) ascii_bin` | no | Public handle, unique |
| `user_id` | `bigint unsigned` | no | Owning user |
| `space_id` | `bigint unsigned` | yes | Owning space |
| `name` | `varchar(255)` | no | |
| `description` | `text` | yes | Shown in pickers |
| `instructions` | `text` | yes | Appended to the system prompt for runs using this agent |
| `plugins` | `text` | yes | JSON array of catalog plugin names this agent loads |
| `sandbox_network_tier` | `varchar(64)` | yes | `none`, `registries`, or `open`; empty inherits the space default, then the surface baseline |
| `sandbox_filesystem_tier` | `varchar(64)` | yes | `workspace`, `workspace_plus_shared_read`, or `workspace_plus_external_write`; same fallback as the network tier |
| `revision` | `bigint` | no | Number of the `agent_revision` row holding this content; starts at 1 |
| `deleted_at` | `datetime(6)` | yes | Set when the agent was deleted; the row stays |
| `created_at` | `datetime(6)` | yes | `autoCreateTime` |

Indexes: PK `id`; index `deleted_at`; index `space_id`; index `user_id`; unique
`public_id`.

Deletion is a stamp on `deleted_at`, not a `DELETE`. Tasks, workflow step runs,
and revisions all name an agent by ID, so removing the row turned every one of
those into a dangling reference and broke any workflow run still in flight at
its next step. Reads split accordingly: `GetAgent` and the list queries see only
live agents, so nothing can start new work with a deleted one, while
`GetAgentIncludingDeleted` resolves what an existing record refers to. Deleting
an agent a `published` workflow still names is refused with `409` — that
workflow could still be run, and the failure would otherwise surface at the next
run rather than at the delete. Draft and archived workflows do not block it,
because neither can start a run and publishing revalidates its agents.

`plugins` names catalog plugins, never releases: the version and digest come
from the space's `plugin_activation` row, so moving a plugin to a new release
stays one edit in one place. Nothing is inherited from the space's activations —
an agent that names none loads none — and the list is stored trimmed,
deduplicated, and sorted, so reordering the same set does not append a revision.
It is a JSON column rather than a join table because nothing queries inside it:
the selection is written and read whole, and "which agents name this plugin" is
a scan of one space's agents.

There is no undelete route. The row exists so references resolve, not as a
recycle bin.

These are server-side agent records. They are distinct from the workspace
subagents defined as Markdown files under `.buildmax/`; see
[help/skills-and-subagents.md](../../../../help/skills-and-subagents.md).

### `agent_revision`

One recorded version of an agent definition. Rows are appended, never updated or
deleted.

| Column | Type | Null | Notes |
|---|---|---|---|
| `id` | `bigint unsigned` | no | Internal primary key |
| `agent_id` | `bigint unsigned` | no | `agent.id` |
| `revision` | `bigint` | no | 1 for the first recorded content, then one higher per change |
| `name` | `varchar(255)` | no | |
| `description` | `text` | yes | |
| `instructions` | `text` | yes | |
| `plugins` | `text` | yes | JSON array; the selection this revision recorded |
| `sandbox_network_tier` | `varchar(64)` | yes | The tier this revision recorded |
| `sandbox_filesystem_tier` | `varchar(64)` | yes | The tier this revision recorded |
| `created_by` | `bigint unsigned` | no | The user who wrote this revision, not necessarily the agent's owner |
| `created_at` | `datetime(6)` | yes | `autoCreateTime` |

Indexes: PK `id`; unique `idx_agent_revision` on (`agent_id`, `revision`).

The revision is written in the same transaction as the agent row it describes,
and the unique (`agent_id`, `revision`) index makes a concurrent second write
fail rather than record two definitions under one number. An update that changes
nothing appends no revision. Restoring an earlier revision is an ordinary
update: it appends a new revision holding the old content rather than rewinding
to it.

Revisions outlive the agent's use: a deleted agent keeps its history, which is
what a past run's provenance points at. The revision routes serve live agents
only, so reading a deleted agent's history means querying the table.

Agents and workflows that existed before revision history was added were given a
revision 1 by migration `0003_seed_first_agent_and_workflow_revision`. That row
is an approximation: its author is whoever created the record and its timestamp
is when the content last moved, neither of which necessarily identifies the edit
that produced the content it holds.

### `conversation`

The independent foreground chat and optional Agent-task orchestrator. It owns
its messages, not the Tasks it may start or display.

| Column | Type | Null | Notes |
|---|---|---|---|
| `id` | `bigint unsigned` | no | Internal primary key |
| `public_id` | `char(20) ascii_bin` | no | Public handle, unique |
| `user_id` | `bigint unsigned` | no | Owning user |
| `space_id` | `bigint unsigned` | yes | Owning space |
| `channel` | `varchar(32)` | no | `portal`, `telegram`, `cron`, `webhook`, or a synthetic `workflow` / `issue_agent` |
| `title` | `varchar(256)` | yes | Generated from the first turn |
| `created_by` | `bigint unsigned` | no | `user.id` |
| `created_at` | `datetime(6)` | yes | `autoCreateTime` |

Indexes: PK `id`; index `idx_conversation_space_created` on (`space_id`,
`created_at`); index `idx_conversation_user_created` on (`user_id`,
`created_at`); unique `public_id`.

Transport channel constants are in
`internal/service/conversation/channel/types.go`. `system` exists as a constant
but is not in `ValidChannels`, so it cannot be supplied by a caller.

A workflow step and an issue agent run each create a Task directly, with
`task.space_id` as owner and no `conversation_id`; neither creates a
conversation for Task to hang on. See
[agent execution and Task threads](../../../design/agent-execution-and-task-threads.md).

### `conversation_message`

One message in a Tier 1 conversation, including tool traffic.

| Column | Type | Null | Notes |
|---|---|---|---|
| `id` | `bigint unsigned` | no | Internal primary key |
| `public_id` | `char(20) ascii_bin` | no | Public handle, unique |
| `conversation_id` | `bigint unsigned` | no | `conversation.id` |
| `role` | `varchar(16)` | no | LLM message role |
| `content` | `text` | no | |
| `channel` | `varchar(32)` | yes | Overrides the conversation channel for this message |
| `tool_call_id` | `varchar(64)` | yes | Set on a tool result, linking it to the call that produced it |
| `tool_calls` | `text` | yes | JSON array of tool calls on an assistant message; the Go field is `ToolCallsJSON` but the column is `tool_calls` |
| `provider_state` | `text` | yes | Opaque reasoning state on an assistant message, stored and replayed but never read here; the Go field is `ProviderStateJSON` |
| `parts` | `mediumtext` | yes | Non-text content on the message, such as an image a tool returned; `content` stays the text describing it. The Go field is `PartsJSON` |
| `created_at` | `datetime(6)` | yes | `autoCreateTime` |

Indexes: PK `id`; index `idx_conversation_message_conversation` on
(`conversation_id`, `created_at`); unique `public_id`.

Ordering is by `created_at`, then `id`. Prefixed IDs are random, not
time-ordered, so never sort by `conversation_message_id`.

`provider_state` holds what a protocol produced and requires back unchanged —
Anthropic thinking blocks, OpenAI Responses reasoning items. A Tier 1 turn
resumes from these rows, so without it a second turn would send the upstream a
conversation it rejects. A row written before the column existed, or one holding
something that no longer parses, replays as a message without state rather than
failing the turn. See
[design/llm-provider-adapters.md](../../../design/llm-provider-adapters.md).

## Background Execution

Task plus task_run is durable Agent execution. TaskRun owns the result;
Conversation, Issue, and Workflow views may project it through explicit
optional relations.

### `task`

The durable unit of background work. One task, many attempts.

| Column | Type | Null | Notes |
|---|---|---|---|
| `id` | `bigint unsigned` | no | Internal primary key |
| `public_id` | `char(20) ascii_bin` | no | Public handle, unique |
| `conversation_id` | `bigint unsigned` | yes | Optional origin/projection relation; a direct Agent, Issue, or Workflow task has none |
| `space_id` | `bigint unsigned` | no | Owning space, authoritative for every Task operation |
| `issue_id` | `bigint unsigned` | yes | The issue this task advances, if any |
| `status` | `varchar(32)` | no | `PENDING`, `SCHEDULED`, `RUNNING`, `SUCCEEDED`, `FAILED`, `CANCELED` |
| `input` | `text` | no | The prompt |
| `title` | `varchar(256)` | yes | LLM-generated |
| `title_prompt_tokens` | `bigint` | yes | Tokens spent generating the title — counted against quota |
| `title_completion_tokens` | `bigint` | yes | Same |
| `output` | `text` | yes | Result of the latest successful run |
| `created_by` | `bigint unsigned` | no | `user.id` |
| `created_at` | `datetime(6)` | yes | `autoCreateTime` |
| `started_at` | `datetime(6)` | yes | First run start |
| `ended_at` | `datetime(6)` | yes | Terminal-status time |
| `error_message` | `text` | yes | |
| `session_id` | `varchar(36)` | yes | UUID of the agent session file, not a table reference |
| `last_run_id` | `bigint unsigned` | yes | `task_run.id` of the most recent attempt |
| `agent_id` | `bigint unsigned` | yes | `agent.id` this task runs as |
| `workspace_head_checkpoint_id` | `bigint unsigned` | yes | `workspace_checkpoint.id` accepted as the Task's recoverable workspace; its seed, then each successful result. Null until the first run commits one |
| `plugin_environment_head_id` | `bigint unsigned` | yes | Immutable Plugin environment the next Continue uses; null for a Task that installs nothing autonomously |

Indexes: PK `id`; index `agent_id`; index `conversation_id`; index `issue_id`;
index `last_run_id`; index `workspace_head_checkpoint_id`; index
`plugin_environment_head_id`; index `idx_task_space_created` on (`space_id`,
`created_at`); unique `public_id`.

Status values are `task.RunStatus` — uppercase, and shared with `task_run`.

### `task_run`

One execution attempt. This is the row quota and token accounting read.

| Column | Type | Null | Notes |
|---|---|---|---|
| `id` | `bigint unsigned` | no | Internal primary key |
| `public_id` | `char(20) ascii_bin` | no | Public handle, unique |
| `task_id` | `bigint unsigned` | no | `task.id` |
| `previous_task_run_id` | `bigint unsigned` | yes | Immutable predecessor in the Task's linear run history; the worker restores this run's session bundle |
| `input` | `text` | no | Prompt for this attempt; a rerun may differ from the task's |
| `created_by` | `varchar(64)` | yes | `user.id`, empty for system-triggered runs |
| `created_by_type` | `varchar(32)` | yes | `user`, `webhook`, or `system` |
| `trigger_source` | `varchar(64)` | yes | `task_create`, `task_rerun`, `portal_conversation`, `portal_task_create`, `portal_task_rerun`, `issue_agent_run`, `workflow_step`, `webhook` |
| `status` | `varchar(32)` | no | Same `task.RunStatus` values as `task` |
| `output` | `text` | yes | |
| `error_message` | `text` | yes | |
| `started_at` | `datetime(6)` | yes | |
| `ended_at` | `datetime(6)` | yes | `NULL` while running |
| `session_id` | `varchar(36)` | yes | UUID of this run's session file |
| `worker_type` | `varchar(32)` | yes | `local_process` or `k8s_job`; see below for when it is written |
| `k8s_job_name` | `varchar(128)` | yes | The Job that ran this run; `NULL` under the local runner |
| `k8s_job_created_at` | `datetime(6)` | yes | When that Job was created; `NULL` under the local runner |
| `prompt_tokens` | `bigint` | yes | Quota input |
| `completion_tokens` | `bigint` | yes | Quota input |
| `trace_path` | `varchar(512)` | yes | This run's durable trace inside run-global storage, e.g. `traces/<session>/rt_….jsonl`; `NULL` when none was written |
| `cancel_requested_at` | `datetime(6)` | yes | When someone asked this run to stop; `NULL` when nobody has |
| `cancel_requested_by` | `bigint unsigned` | yes | `user.id` of whoever asked |
| `retry_of_task_run_id` | `bigint unsigned` | yes | The run this one repeats; `NULL` for a run that carries its own instructions |
| `source_message_id` | `bigint unsigned` | yes | `conversation_message.id` this run was asked for in; `NULL` when no message asked for it |
| `agent_revision` | `int` | yes | Which revision of `task.agent_id` this run was served; `NULL` for a run with no agent or one that never reached a worker |
| `space_agent_instructions_revision` | `int` | yes | Which revision of the owning space's Space-level instructions this run was served; `0` records no configured text, `NULL` means no provenance |
| `plugin_pins` | `text` | yes | JSON array of `{plugin_name, version, digest}`: the releases this run was given |
| `sandbox_network_tier` | `varchar(64)` | yes | The tier resolved on the first poll -- agent declaration, then space default, then the surface baseline; `NULL` until a worker claims the run |
| `sandbox_filesystem_tier` | `varchar(64)` | yes | The tier resolved on the first poll, same fallback as `sandbox_network_tier` |
| `last_seen_at` | `datetime(6)` | yes | When this run's worker last polled its own route; `NULL` until a worker claims the run |
| `idempotency_key` | `varchar(128)` | yes | Caller's dedup key for a Continue request; `NULL` for a run created without one — a retry, a workflow step, an issue agent run, or an older client |
| `workspace_base_checkpoint_id` | `bigint unsigned` | yes | `workspace_checkpoint.id` this run was authorized to read and modify, fixed before execution |
| `workspace_result_checkpoint_id` | `bigint unsigned` | yes | Successful result checkpoint this run committed |
| `workspace_partial_checkpoint_id` | `bigint unsigned` | yes | Partial checkpoint captured after failure, cancellation, or interruption; never the Task head |
| `workspace_restore_status` | `varchar(32)` | yes | `not_requested`, `pending`, `restored`, or `failed` |
| `workspace_restore_error` | `text` | yes | Bounded operator-facing reason a restore failed |
| `workspace_checkpoint_status` | `varchar(32)` | yes | `not_requested`, `pending`, `committed`, or `failed` |
| `workspace_checkpoint_error` | `text` | yes | Bounded operator-facing reason a capture or commit failed |
| `plugin_environment_base_id` | `bigint unsigned` | yes | Immutable Plugin set materialized for this run |
| `plugin_environment_result_id` | `bigint unsigned` | yes | New Plugin set a committed autonomous install requested; takes effect on the next TaskRun boundary |
| `plugin_environment_status` | `varchar(32)` | yes | `unchanged`, `pending`, `committed`, or `failed` |
| `plugin_environment_error` | `text` | yes | Bounded installation or materialization reason |
| `created_at` | `datetime(6)` | yes | `autoCreateTime` |

Indexes: PK `id`; index `cancel_requested_at`; index `created_by`; index
`last_seen_at`; index `previous_task_run_id`; index `retry_of_task_run_id`; index `source_message_id`; index
`idx_task_run_task_created` on (`task_id`, `created_at`); unique `public_id`;
unique `idx_task_run_idempotency` on (`task_id`, `idempotency_key`).

A repeated `POST .../tasks/{task_id}/runs` naming the same idempotency key on
the same task returns the run the first call created rather than starting a
second one — while that run is still active and after it has finished. `NULL`
is not a duplicate of `NULL` in a MySQL unique index, so every run created
without a key coexists with every other one. `CreateTaskRun` takes a locking
read on the task row before checking this and the active-run count below, so
two concurrent callers for one task cannot both see a clean slate and both
insert; see [agent execution and Task threads §12](../../../design/agent-execution-and-task-threads.md#12-failure-recovery-and-concurrency).

`agent_revision` is not a reference to `agent_revision.id`: a revision is
addressed by its agent plus its number, and the task already holds the agent. It
is written when a worker asks for its run, and the first write wins — instructions
are resolved per dispatch so an edit takes effect on the next run, and the record
exists so an edit during a run cannot rewrite what that run was given.

`space_agent_instructions_revision` follows the same first-write-wins rule. The
worker receives the owning space's current Space instructions as a separate
system-prompt layer before the selected Agent's instructions; editing the Space
changes the next run, not one already executing.

`plugin_pins` is written at that same moment and under the same rule, because it
answers the same question about the same run. The server resolves the space's
`plugin_activation` rows against the agent's selection and sends a finished list;
a worker never reads activations itself. Resolving at claim time rather than at
dispatch is safe because an activation names an exact version and digest — the
pin, not the timing, is what stops a release published in between from changing
what a run loads. The trace carries the same inventory, but a trace is fail-open
and lives in run-global storage, so this column is the queryable fact and what a
retry reads. Empty for a run whose agent named no plugin, for a run with no
agent, and for one that never reached a worker.

`source_message_id` is what a person actually said; `input` is what Tier 1
decided to send a worker. They are different texts and keeping both is the
point: a constraint missing from `input` is either one the model dropped or one
the user never gave, and nothing else in the schema can tell those apart. Each
run records its own — a task's first run names the message that created it, a
continuation names the message that asked for it. It is `NULL` for every origin
that is not a message: a workflow step, an issue agent run, a retry, and a task
created straight from the API. A handle that does not resolve leaves the column
`NULL` rather than refusing the run; losing provenance is better than refusing
work someone asked for.

`retry_of_task_run_id` points at the run a retry repeats, one link per row: a
retry of a retry names the run it repeated, not the first of the chain. It is
not a foreign key, and the run it names is never modified — a retry is a new
attempt, not a rewrite of the record that explains why one was needed. The
matching `trigger_source` is `task_retry`.

`previous_task_run_id` is different from retry lineage: it names the run that
was current immediately before this run was admitted, whether the new run is a
Continue or a Retry. It is written in the same transaction that advances
`task.last_run_id` and never changes afterwards. Workers use this immutable
value to restore the prior session bundle; reading the Task projection would
only return the current run after admission.

`cancel_requested_at` is a request, not a status: a run a worker already holds
stays `RUNNING` until that worker reports `CANCELED`, because nothing else can
end another process's agent loop. The worker sees the request by polling its own
run route, and `StaleRunReaper` finishes runs whose worker never confirms — the
same backstop that closes abandoned ones. Both columns are written once; a
second cancel does not overwrite who asked first.

`last_seen_at` is how the server tells a worker that died from one that is
slow. It is written on `GET /api/worker/task-runs/{id}` and nowhere else: a
worker polls that route every few seconds for the whole time its run is
`RUNNING`, so the signal already existed and only had to be recorded. The
streaming route deliberately does not stamp — it fires many times a second — and
neither does the terminal `PATCH`, which would move the timestamp past the
moment the work stopped. `StaleRunReaper` reads it to fail `RUNNING` runs that
have gone silent, minutes after the fact rather than at `worker.run_timeout`.
`NULL` is never reaped for silence: no signal was ever recorded, so there is
nothing to have gone quiet.

`worker_type`, `k8s_job_name`, and `k8s_job_created_at` are written by
`Scheduler.dispatch` through `UpdateTaskRunWorkerInfo`, once the runner returns
without error. When that happens differs by runner, and the difference matters
before reading these columns:

- **`k8s_job`** returns as soon as the Job is created, so all three are written
  at dispatch and are readable for the whole run.
- **`local_process`** blocks for the entire run, so `worker_type` is written
  only after the worker exits, and only when it exits cleanly — a spawn or exit
  failure takes the failure path, which records the error instead. The two
  Kubernetes columns stay `NULL`.

Nothing reads any of them yet. `k8s_job_name` is what a future sweep would use
to ask Kubernetes why a worker disappeared — `OOMKilled` and `Evicted` are the
same silence from the server's side — but that would cover the `k8s_job` runner
only, which is why `last_seen_at` rather than Job status is what the stale-run
reaper watches.

`trace_path` is written by the worker on the terminal PATCH, on failure as well
as success. It is stored rather than derived because the trace's file name is
the agent run id, which is generated inside the run and appears nowhere else.
The value is the same key `uploadTaskGlobal` uploads the file under, so it
resolves directly against run-global storage — a test in
`internal/agentapp/taskrun` couples the two computations so they cannot drift.

The scheduler claims work by polling for the oldest pending run
(`GetNextPendingTaskRun`); GORM's logger is configured to swallow
`ErrRecordNotFound` so an idle server does not log a miss every poll.

### `workspace_checkpoint`

An immutable, complete representation of a Task's `workspace/` at one boundary —
its seed, a successful result, or a partial. See
[task workspace checkpoints](../../../design/task-workspace-checkpoints.md) §9.1.
The payload lives in object storage; this row is its metadata.

| Column | Type | Null | Notes |
|---|---|---|---|
| `id` | `bigint unsigned` | no | Internal primary key |
| `public_id` | `char(20) ascii_bin` | no | Public handle, unique |
| `space_id` | `bigint unsigned` | no | Authorization owner |
| `task_id` | `bigint unsigned` | no | Workspace owner |
| `source_task_run_id` | `bigint unsigned` | no | The run that captured it |
| `kind` | `varchar(32)` | no | `seed`, `successful`, or `partial` |
| `base_checkpoint_id` | `bigint unsigned` | yes | Lineage predecessor |
| `payload_format` | `varchar(32)` | no | Initially `tar.zst.v1` |
| `payload_sha256` | `char(64) ascii_bin` | no | Digest of the stored bytes |
| `storage_key` | `varchar(1024)` | no | Backend-relative key; never serialized to domain JSON, a worker response, a log, or a trace |
| `size_bytes` | `bigint` | no | Stored payload bytes |
| `uncompressed_bytes` | `bigint` | no | Sum of regular-file sizes |
| `entry_count` | `bigint` | no | Regular files, directories, and symlinks |
| `created_at` | `datetime(6)` | no | UTC commit time |

Uniqueness is `(source_task_run_id, kind)`: a run has at most one seed, one
successful, and one partial. `payload_sha256` is not unique — separate rows can
name the same immutable payload with separate provenance. `storage_key` follows
the `artifact` rule: it is infrastructure data absent from every serialized
surface.

Indexes: PK `id`; unique `public_id`; unique `(source_task_run_id, kind)`;
index `space_id`; index `base_checkpoint_id`; index
`idx_workspace_checkpoint_task_created` on (`task_id`, `created_at`).

### `artifact`

One durable file the space owns, with one immutable content object. Content
lives in object storage under a key this table records and no API returns.

The name is reused: the `artifact` table dropped by migration 0001 was a task
run's child structure. This one is a first-class object whose producer is
recorded as provenance, so migration 0001 now checks for `artifact_item` and for
a legacy `task_run_id` column before touching either table. See
[../../../design/unified-artifacts.md](../../../design/unified-artifacts.md).

| Column | Type | Null | Notes |
|---|---|---|---|
| `id` | `bigint unsigned` | no | Internal primary key |
| `public_id` | `char(20) ascii_bin` | no | Public handle, unique |
| `space_id` | `bigint unsigned` | no | Owning space; the authorization boundary |
| `filename` | `varchar(512)` | no | One path element; directories are stripped |
| `media_type` | `varchar(255)` | yes | Derived from the extension, never from the uploader |
| `size_bytes` | `bigint` | no | Measured while streaming |
| `sha256` | `varchar(64)` | no | Digest of what was stored; not a dedup key |
| `storage_key` | `varchar(1024)` | no | Object key. Never serialized anywhere |
| `created_by_type` | `varchar(32)` | no | `user`, `agent`, `worker`, or `system` |
| `created_by_id` | `varchar(64)` | yes | Empty for automated work |
| `source_type` | `varchar(32)` | no | `agent`, `task_run`, `user_upload`, `system` |
| `source_id` | `varchar(64)` | yes | The producing operation |
| `title` | `varchar(255)` | yes | Display label |
| `deleted_at` | `datetime(6)` | yes | Tombstone; set means hidden and unreadable |
| `expires_at` | `datetime(6)` | yes | Retention hook |
| `created_at` | `datetime(6)` | yes |  |

Indexes: PK `id`; index `deleted_at`; index `expires_at`; index `source_id`;
index `idx_artifact_space_created` on (`space_id`, `created_at`); unique
`public_id`.

There is deliberately no free-form metadata column. Durable metadata is where
prompts, file contents, and credentials leak in when a column will take
anything, so a new product behavior earns a named column instead.

Deletion is a tombstone rather than a row removal: it must take effect at the
authorization boundary immediately, while reclaiming the object is retention's
job and may be slower than the request that asked for it.

## Workflows

Workflows are space-scoped reusable linear plans. A run expands the stored
definition into one step run per step, and each agent step delegates to a task.

### `workflow`

| Column | Type | Null | Notes |
|---|---|---|---|
| `id` | `bigint unsigned` | no | Internal primary key |
| `public_id` | `char(20) ascii_bin` | no | Public handle, unique |
| `space_id` | `bigint unsigned` | no | Owning space — required, unlike most tables |
| `name` | `varchar(255)` | no | |
| `description` | `text` | no | |
| `definition` | `longtext` | no | JSON step list; `longtext`, not `text`, because plans can be large |
| `status` | `varchar(32)` | no | `draft` (default), `published`, `archived` |
| `revision` | `bigint` | no | Number of the `workflow_revision` row holding this content; starts at 1 |
| `created_by` | `bigint unsigned` | no | `user.id` |
| `created_at` | `datetime(6)` | yes | `autoCreateTime` |
| `updated_at` | `datetime(6)` | yes | `autoUpdateTime` |

Indexes: PK `id`; index `space_id`; unique `public_id`.

`definition` is opaque to the database. Editing a published workflow does not
retroactively change runs already expanded from it.

### `workflow_revision`

One recorded version of a workflow. Rows are appended, never updated or deleted.
The rules are the same as for [`agent_revision`](#agent_revision), including the
seeded first revision for workflows that predate the table.

| Column | Type | Null | Notes |
|---|---|---|---|
| `id` | `bigint unsigned` | no | Internal primary key |
| `workflow_id` | `bigint unsigned` | no | `workflow.id` |
| `revision` | `bigint` | no | 1 for the first recorded content, then one higher per change |
| `name` | `varchar(255)` | no | |
| `description` | `text` | no | |
| `definition` | `longtext` | no | |
| `status` | `varchar(32)` | no | The lifecycle state this revision was written with |
| `created_by` | `bigint unsigned` | no | `user.id` |
| `created_at` | `datetime(6)` | yes | `autoCreateTime` |

Indexes: PK `id`; unique `idx_workflow_revision` on (`workflow_id`, `revision`).

`status` is recorded because publishing is what lets a workflow run, so the
record of who published which definition belongs in history. It is not restored:
restoring an old revision writes back its name, description, and definition and
leaves the current lifecycle state alone, so restoring the content of a draft
revision cannot unpublish a workflow spaces are running.

### `workflow_run`

| Column | Type | Null | Notes |
|---|---|---|---|
| `id` | `bigint unsigned` | no | Internal primary key |
| `public_id` | `char(20) ascii_bin` | no | Public handle, unique |
| `workflow_id` | `bigint unsigned` | no | `workflow.id` |
| `workflow_revision` | `bigint` | no | The revision this run expanded; 0 for runs started before workflows recorded revisions |
| `issue_id` | `bigint unsigned` | yes | Issue this run advances |
| `status` | `varchar(32)` | no | `pending`, `running`, `succeeded`, `failed`, `canceled` — lowercase, unlike `task` |
| `created_by` | `bigint unsigned` | no | `user.id` |
| `created_at` | `datetime(6)` | yes | `autoCreateTime` |
| `started_at` | `datetime(6)` | yes | |
| `ended_at` | `datetime(6)` | yes | |
| `error_message` | `text` | yes | |

Indexes: PK `id`; index `issue_id`; index
`idx_workflow_run_workflow_created` on (`workflow_id`, `created_at`); unique
`public_id`.

Each step run creates a Space-owned Task directly (`task.space_id`, no
`conversation_id`); a run's progress is read from its steps' `task_id` /
`task_run_id`, not from a Conversation.

### `workflow_step_run`

One step of one workflow run. The bridge between the workflow engine and Tier 2.

| Column | Type | Null | Notes |
|---|---|---|---|
| `id` | `bigint unsigned` | no | Internal primary key |
| `public_id` | `char(20) ascii_bin` | no | Public handle, unique. The Go field is `StepRunID` |
| `workflow_run_id` | `bigint unsigned` | no | `workflow_run.id` |
| `step_id` | `varchar(128)` | no | Step identifier authored in the workflow definition, not a reference to a row |
| `step_index` | `bigint` | no | Position in the linear plan; the execution order |
| `step_type` | `varchar(32)` | no | `agent_task` |
| `target_agent_id` | `bigint unsigned` | yes | `agent.id` to run the step as |
| `agent_name` | `varchar(255)` | no | Agent name captured when the run started; empty on rows written before step runs snapshotted their agent |
| `agent_description` | `text` | no | Agent description captured when the run started |
| `agent_instructions` | `longtext` | no | Agent instructions captured when the run started |
| `agent_revision` | `bigint` | no | The `agent_revision.revision` the snapshot came from; 0 when it predates revisions |
| `prompt` | `text` | no | Rendered prompt for this step |
| `status` | `varchar(32)` | no | `pending`, `running`, `succeeded`, `failed`, `canceled`, `blocked` |
| `task_id` | `bigint unsigned` | yes | The Tier 2 task this step created |
| `task_run_id` | `bigint unsigned` | yes | The specific attempt |
| `output_summary` | `text` | yes | First 500 runes of the step output, for display; it is not passed to the next step |
| `error_message` | `text` | yes | |
| `created_at` | `datetime(6)` | yes | `autoCreateTime` |
| `started_at` | `datetime(6)` | yes | |
| `ended_at` | `datetime(6)` | yes | |

Indexes: PK `id`; index `idx_step_run_run_index` on (`workflow_run_id`,
`step_index`); index `target_agent_id`; index `task_id`; index `task_run_id`;
unique `public_id`.

The three `agent_*` columns pin the agent definition for the whole run. Steps are
dispatched one at a time as the previous task run reaches a terminal state, so
without them an edit to the agent between two steps would change what the later
step sends to the model.

`blocked` has no counterpart in `workflow_run.status`. When a step fails, the run
is marked `failed` and every later `pending` step becomes `blocked`.

`canceled` is written when the step's task run is canceled. It stops the run the
way a failure does — later steps are blocked, the run ends — and the run is
marked `canceled` rather than `failed`, because nothing went wrong.

## Managed Inference

These two tables back the LLM gateway. Read
[../../../design/llm-gateway.md](../../../design/llm-gateway.md) before changing
either.

### `llm_model`

The model catalog. Edited with `buildmax-server model add|list|enable|disable`
on the machine that holds the database credentials.

| Column | Type | Null | Notes |
|---|---|---|---|
| `id` | `bigint unsigned` | no | Internal primary key |
| `public_id` | `char(20) ascii_bin` | no | Public handle, unique |
| `name` | `varchar(128)` | no | Operator-facing catalog name, unique |
| `provider_type` | `varchar(32)` | no | Wire protocol: `openai_compatible`, `openai`, or `anthropic` |
| `api_url` | `varchar(512)` | no | Upstream base URL |
| `api_key` | `varchar(512)` | no | **Provider credential in plaintext** — see below |
| `model` | `varchar(128)` | no | Upstream model identifier |
| `context_window` | `bigint` | no | Default `0`, meaning unspecified |
| `call_timeout` | `bigint` | no | Seconds; default `0`, meaning unspecified |
| `max_tokens` | `bigint` | no | Cap on one response; default `0`, meaning the client default |
| `reasoning` | `varchar(16)` | no | Effort level: empty or `off`, `low`, `medium`, `high` |
| `cache_mode` | `varchar(16)` | no | Default `''`; prompt-cache policy: `auto`, `off`, `force` |
| `cache_ttl` | `varchar(16)` | no | Default `''`; prompt-cache retention: `provider_default`, `5m`, `1h` |
| `currency` | `varchar(8)` | no | Default `''`; ISO 4217 code the rates below are quoted in. Empty means unpriced |
| `input_per_mtok` | `bigint` | no | Nano-currency-units per million fresh prompt tokens |
| `cache_read_per_mtok` | `bigint` | no | Per million cached prompt tokens read |
| `cache_write_per_mtok` | `bigint` | no | Per million prompt tokens written to cache |
| `output_per_mtok` | `bigint` | no | Per million generated tokens |
| `vision` | `tinyint(1)` | no | Default `false`; the upstream accepts image input |
| `capabilities` | `varchar(255)` | yes | Comma-separated: `text_chat`, `tool_calls`, `streaming_text`, `usage_reporting` |
| `enabled` | `tinyint(1)` | no | Default `true` |
| `created_at` | `datetime(6)` | yes | `autoCreateTime`, indexed for listing order |
| `updated_at` | `datetime(6)` | yes | `autoUpdateTime` |

An empty `cache_mode` means nobody chose, and takes the default policy. An
operator who wants caching off writes `cache_mode = off`.

Indexes: PK `id`; index `created_at`; unique `name`; unique `public_id`.

`api_key` is read by exactly one query — the one that constructs a provider
client — and never appears in a listing, an API response, or an error message.
It is nonetheless stored in plaintext, so **database backups carry provider
credentials** and must be handled accordingly. See [../../../SECURITY.md](../../../../SECURITY.md).

`capabilities` is a comma-separated list rather than a join table: the set is
small, closed, and only ever read whole.

Every enabled row is callable by every user of the deployment: a space is a
collaboration boundary, not a model authorization boundary. A client names a
model by its `name`, which is unique across the deployment; `server.yaml`
`llm.default_model` names the one a caller that names none gets, and a name
there that matches no row stops the server at startup.

### `llm_call`

One managed inference call. The metering and debugging record.

| Column | Type | Null | Notes |
|---|---|---|---|
| `id` | `bigint unsigned` | no | Internal primary key |
| `public_id` | `char(20) ascii_bin` | no | Public handle, unique |
| `client_call_id` | `varchar(128)` | yes | Caller's idempotency key; part of the composite unique index |
| `user_id` | `bigint unsigned` | yes | Who the call is attributed to; leads the composite unique index |
| `task_run_id` | `bigint unsigned` | yes | Attributes the call to a Tier 2 run |
| `surface` | `varchar(32)` | yes | `server`, `cli`, `desktop`, `worker` |
| `session_id` | `varchar(64)` | yes | |
| `task_id` | `bigint unsigned` | yes | |
| `model` | `varchar(128)` | yes | The catalog name the caller asked for |
| `target_id` | `varchar(64)` | no | Catalog entry the name resolved to — `llm_model.id` |
| `provider_type` | `varchar(32)` | no | Denormalized from the catalog at call time |
| `upstream_model` | `varchar(128)` | no | Denormalized from the catalog at call time |
| `streaming` | `tinyint(1)` | no | Default `false` |
| `accepted_at` | `datetime(6)` | no | When the gateway accepted the request; indexed |
| `upstream_started_at` | `datetime(6)` | yes | |
| `first_delta_at` | `datetime(6)` | yes | Time to first token, on streaming calls |
| `completed_at` | `datetime(6)` | yes | |
| `status` | `varchar(16)` | no | `ACCEPTED`, `SUCCEEDED`, `FAILED`, `CANCELED`; indexed |
| `error_class` | `varchar(64)` | yes | Stable BuildMax error code, not the upstream message |
| `attempts` | `bigint` | no | Default `0` |
| `prompt_tokens` | `bigint` | yes | |
| `completion_tokens` | `bigint` | yes | |
| `total_tokens` | `bigint` | yes | |
| `cache_read_tokens` | `bigint` | yes | Part of the prompt served from the provider's cache |
| `cache_write_tokens` | `bigint` | yes | Part of the prompt written into it |
| `currency` | `varchar(8)` | yes | The rate snapshot's currency; empty when the model was unpriced |
| `rate_input_per_mtok` | `bigint` | yes | Fresh-input rate in force when the call was accepted |
| `rate_cache_read_per_mtok` | `bigint` | yes | Cache-read rate in force then |
| `rate_cache_write_per_mtok` | `bigint` | yes | Cache-write rate in force then |
| `rate_output_per_mtok` | `bigint` | yes | Output rate in force then |
| `usage_source` | `varchar(16)` | yes | `reported`, `estimated`, or `unavailable` |

Indexes: PK `id`; index `accepted_at`; unique `idx_llm_call_client` on
(`user_id`, `client_call_id`); index `status`; index `task_id`; index
`task_run_id`; unique `public_id`.

A call is attributed to a person, not a space: a foreground CLI or Desktop call
belongs to no space, and a run's space is reached through `task_run_id`. The
composite unique index leads with `user_id`, which both scopes idempotency per
caller and serves per-user lookups — so there is deliberately no second index on
`user_id` alone. See
[../../../design/client-modes.md](../../../design/client-modes.md) section 9.

The cache counts **break `prompt_tokens` down rather than adding to it**. A
spend report that summed all three would count the same tokens twice.

`provider_type` and `upstream_model` are copied onto the row rather than joined
from `llm_model`, so a completed call still describes what actually ran after
the catalog entry is edited or deleted.

The rate columns are copied for the same reason, and one more: a model gets
repriced, and a spend report recomputed from today's rates would restate an
invoice that has already been paid. They are written when the call is accepted
and never updated. A row whose `currency` is empty was run against an unpriced
model, or predates these columns; either way its cost is unknown, which is not
the same fact as a call that cost nothing.

Amounts are nano-currency-units — one currency unit is 1e9 of them — held as
integers because a float would round a published price before anything read it
and drift a few hundred calls into a figure someone compares against a bill.

Note that `llm_call` is *not* what quota reads. Quota aggregates `task_run`
tokens; `llm_call` records gateway traffic including calls with no task behind
them. The two will not agree, by design.

## Plugin Catalog

These two tables back the private Marketplace. Read
[../../../design/plugin-marketplace.md](../../../design/plugin-marketplace.md) before
changing either.

The catalog belongs to the deployment, not to a space: neither table carries a
`space_id`, which is what lets a System Administrator manage company
capabilities without reaching into any space's prompts, files, or traces.

### `plugin`

One catalog entry — the stable identity releases are published under.

| Column | Type | Null | Notes |
|---|---|---|---|
| `id` | `bigint unsigned` | no | Internal primary key |
| `name` | `varchar(128)` | no | The manifest name, unique; every route addresses the plugin by it |
| `display_name` | `varchar(255)` | no | Default `''` |
| `description` | `varchar(1024)` | no | Default `''` |
| `archived_at` | `datetime(6)` | yes | Non-`NULL` hides the entry and refuses new releases |
| `created_by` | `bigint unsigned` | no | `user.id` |
| `created_at` | `datetime(6)` | yes | `autoCreateTime`, indexed for listing order |
| `updated_at` | `datetime(6)` | yes | `autoUpdateTime` |

Indexes: PK `id`; index `archived_at`; index `created_at`; unique `name`.

Archiving never deletes. A copy someone already installed keeps working, and
the record still explains where that copy came from.

### `plugin_release`

One immutable published version.

| Column | Type | Null | Notes |
|---|---|---|---|
| `id` | `bigint unsigned` | no | Internal primary key |
| `plugin_id` | `bigint unsigned` | no | `plugin.id`, indexed |
| `plugin_name` | `varchar(128)` | no | Denormalised so a release reads without a join |
| `version` | `varchar(64)` | no | Semantic version from the packed manifest |
| `min_buildmax_version` | `varchar(64)` | no | Default `''`; default install selection filters on it |
| `digest` | `varchar(128)` | no | `sha256:<hex>`, calculated by the server over the stored bytes |
| `object_key` | `varchar(512)` | no | Where the package bytes live in the object store |
| `size_bytes` | `bigint` | no | Default `0` |
| `inspection` | `text` | yes | JSON: the sanitized capability report |
| `source` | `text` | yes | JSON: the publisher's claim about the checkout the bytes came from |
| `published_by` | `bigint unsigned` | no | `user.id` |
| `published_at` | `datetime(6)` | yes | `autoCreateTime`, indexed for listing order |
| `yanked_at` | `datetime(6)` | yes | Non-`NULL` withdraws it from default selection |
| `yanked_by` | `bigint unsigned` | yes | Default `''` |
| `yanked_reason` | `varchar(512)` | no | Default `''` |

Indexes: PK `id`; index `digest`; index `plugin_id`; index `published_at`;
index `yanked_at`; unique `ux_plugin_release_version` on (`plugin_name`,
`version`).

The unique index over (`plugin_name`, `version`) is what makes a version
immutable, and it is the guard rather than a preceding read: two publishes
racing would both pass a check and only one can pass the constraint.
Publishing an existing version returns `409` even for identical bytes, because
a release is what someone reviewed and what someone else downloaded.

`inspection` and `source` are JSON documents rather than columns because
nothing queries inside them: they are written once and read whole, and giving
each field a column would freeze the report's shape into the schema. Neither
carries command arguments, header values, environment values, prompt text, or
file contents — see design §8. `source` is client-reported and cannot be
verified, so it is presented as a claim rather than as proof.

Package bytes are not in either table. They sit behind the plugin package
storage interface, so a query that lists or inspects releases cannot carry one.

### `plugin_activation`

One space's pinned use of one catalog plugin. The catalog belongs to the
deployment; an activation belongs to a space, which is why this is a separate
table rather than a column on `plugin_release`.

| Column | Type | Null | Notes |
|---|---|---|---|
| `id` | `bigint unsigned` | no | Internal primary key |
| `public_id` | `char(20) ascii_bin` | no | Public handle, unique |
| `space_id` | `bigint unsigned` | no | `space.id` |
| `plugin_name` | `varchar(128)` | no | Catalog identity, as on `plugin_release` |
| `version` | `varchar(64)` | no | The pinned release |
| `digest` | `varchar(128)` | no | The pinned release's digest |
| `enabled` | `boolean` | no | Default `true`; `false` suspends without losing the pin |
| `origin` | `varchar(16)` | no | Default `'curated'`; `curated` or `automatic` |
| `activated_by` | `bigint unsigned` | no | `user.id` |
| `activated_at` | `datetime(6)` | yes | `autoCreateTime`, indexed for listing order |
| `updated_by` | `bigint unsigned` | no | `user.id` of the last change |
| `updated_at` | `datetime(6)` | yes | `autoUpdateTime` |

Indexes: PK `id`; unique `uq_plugin_activation_public_id`; index
`activated_at`; unique `ux_plugin_activation_space_plugin` on (`space_id`,
`plugin_name`).

The unique index over (`space_id`, `plugin_name`) is what makes an activation
one row per pair rather than a history, which is why suspension is the
`enabled` flag: the pin survives it, and a suspended activation still explains
why a run failed. Moving to another release updates `version` and `digest` in
place; the trail of who moved what lives in the audit events, not here.

`version` and `digest` together are the pin, and nothing advances them on its
own. A release published after this row was written cannot change what a run
loads until a person moves it.

`origin` records which of the two ways the row appeared. `curated` is a space
admin activating deliberately; `automatic` is the row created because an agent
named the plugin in a space whose `space.plugin_curation` is `open`. Both are
real pins with the same digest and the same audit event, and `activated_by`
names a person either way. See
[../../../design/plugin-space-distribution.md](../../../design/plugin-space-distribution.md)
§4.1.

## Changing The Schema

The server creates the database named by `database.name` when the server does
not have it, then `AutoMigrate` fills it. That runs only after the connection
failed for that reason, so an existing deployment never issues the statement,
and an account without `CREATE` rights gets an error naming the statement to run
by hand.

`AutoMigrate` runs on every server start and is the whole migration story for
additive changes. Anything it cannot express — a backfill, a drop, a rename —
is an entry in the ordered `migrations` list, recorded in `schema_migration` so
it runs at most once per database. `AutoMigrate` is **additive only**: it creates missing tables, adds
missing columns, and adds missing indexes. It does not drop a column, rename a
column, narrow a type, or change a primary key.

**Adding a column or an index.** Edit the `xxxRow` struct and the matching
struct in that domain's `internal/core/*` package, plus the `toX` / `toXRow`
mapping functions. Add the field to any handler DTO that should expose it.
Nothing else is required — the next server start adds it. Give it a type tag; an untagged `string` becomes
`longtext`.

**Adding a table.** Add the row struct with a `TableName()` method returning a
singular name, register it in the `AutoMigrate` call in `store.go`, define the
repository interface in that domain's `internal/core/*` package, and implement
it in `internal/infra/db`. Decide whether the row needs a handle: give it a
`public_id` — `char(20) CHARACTER SET ascii COLLATE ascii_bin` — with a
`uq_<table>_public_id` unique index when another
process must name it, and nothing when a parent plus a natural key already
addresses it. References to other tables are `bigint unsigned`; the tests in
`internal/architecture` fail otherwise. Then add it to this document.

**Removing, renaming, or retyping.** `AutoMigrate` will not do it, so it needs
an entry in the `migrations` list in `internal/infra/db/migration.go`. Each
entry has a permanent ID and an `Apply` function, runs in list order, and is
recorded in the `schema_migration` table so it executes at most once per
database.

Three rules govern that list:

- **Append only.** Existing IDs and their order are permanent. Renaming an ID
  makes that migration run a second time on every deployed database; reordering
  changes what an upgraded database gets relative to a fresh one.
  `TestMigrationIDsAreStable` fails on either.
- **`Apply` must tolerate re-running.** A crash between applying a change and
  recording it leaves the migration pending, and the next start retries it.
  Probe `information_schema` first and return `nil` when there is nothing to do.
- **Copy before dropping.** Move the data in the same `Apply` that removes its
  old home, so a half-applied migration never loses rows.

**Do not** add a third automatic mechanism, and do not reach for a migration
framework for an additive change `AutoMigrate` already handles.

### Forward Only, One Release Back

The schema moves forward only. There are no down migrations, and `Migration`
has no `Down` field to write one in.

What is supported is rolling the **binary** back one release. Schema version N
must keep serving code from release N-1, which puts one requirement on every
change:

> Do not remove or rename anything in the same release that stops using it.

A removal takes two releases. In the first, the code stops reading and writing
the column or table but the schema keeps it. In the second, a migration drops
it. Between the two, either release's binary runs against either schema.

That is a discipline, not a mechanism. The forward-only half is structural —
there is no `Down` to write — but nothing fails a build when a change removes a
column in one release, and during alpha it may be removed in one deliberately.
BuildMax is alpha and owes no migration path: when a stored shape is wrong, the
fix is to correct it everywhere at once rather than carry the wrong one for a
release. What that spends is the binary rollback, and what it owes in exchange
is a changelog entry saying so.

Take the two-release path by default — it is nearly free, and it is what lets a
bad upgrade be undone by redeploying the previous image. Spend the rollback only
when keeping the wrong shape costs more than losing it, and say in the entry
which one you did.

A binary that starts against a database carrying migrations it does not know
logs a warning and continues, because that is the N-1 promise working: a server
one release behind a migrated database is supposed to keep serving. A server
several releases behind has no such promise, and that log line is the only
signal an operator gets that they are in that position.

Rolling a database *back* is not supported at all. Recovery from a bad upgrade
is a restore from backup, and the deployment documentation says so rather than
implying an undo exists.

**After any schema change**, update this document in the same commit, and check
whether [store.md](store.md) or the design record for the subsystem also needs
a change. Run `./make test` — the store tests in `internal/infra/db` use an
isolated database and will catch a mapping that no longer round-trips.

## 工作对象

### `issue`

主要的用户工作对象。

| 列 | 类型 | 可空 | 说明 |
|---|---|---|---|
| `id` | `bigint unsigned` | 否 | 内部主键 |
| `public_id` | `char(20) ascii_bin` | 否 | 公开句柄，唯一 |
| `user_id` | `bigint unsigned` | 否 | 所属用户 |
| `space_id` | `bigint unsigned` | 是 | 所属 Space；授权键 |
| `parent_issue_id` | `bigint unsigned` | 是 | 父项的 `issue.id`；顶层 Issue 为 `NULL` |
| `title` | `varchar(255)` | 否 | |
| `description` | `text` | 否 | |
| `status` | `varchar(32)` | 否 | `todo`、`in_progress`、`done` |
| `assignee_kind` | `varchar(32)` | 是 | `person`、`agent` 或 `workflow` |
| `assignee_id` | `varchar(64)` | 是 | 根据 `assignee_kind` 解释为 `user_id`、`agent_id` 或 `workflow_id` |
| `created_by` | `bigint unsigned` | 否 | `user.id` |
| `version` | `bigint unsigned` | 否 | 乐观并发控制令牌，从 1 开始 |
| `created_at` | `datetime(6)` | 是 | `autoCreateTime` |
| `updated_at` | `datetime(6)` | 是 | `autoUpdateTime` |

索引：主键 `id`；索引 `parent_issue_id`；(`space_id`, `updated_at`) 上的索引 `idx_issue_space_updated`；索引 `user_id`；唯一索引 `public_id`。

`version` 使每次更新都带条件。更新携带其所依据的版本，store 用 `WHERE public_id = ? AND version = ?` 写入并设置 `version = version + 1`；版本不再匹配的调用者收到 `coreissue.ErrVersionConflict`，即 409，而不是覆盖自己未读过的变更。没有无条件更新路径：不带版本的更新会失败，因为零值不匹配任何行。这里没有复用 `updated_at`；它用于展示和排序，正确性检查不应依赖它经过 RFC 3339 后仍能精确往返。

`assignee_kind` / `assignee_id` 对是多态引用，没有索引或约束将其绑定到特定表，因此验证位于 `internal/service/issue`。

`parent_issue_id` 是构成邻接表的自引用，层级最多为**两层**：父项自身必须满足 `parent_issue_id IS NULL`。模式不强制这一点；不变量位于 `internal/service/issue`，它还拒绝不同 Space 的父项、自身作为父项，以及为已有子项的 Issue 设置父项。进度（`child_count`、`done_child_count`）通过分组查询为每个响应计算，从不存储。权威验证见 `internal/service/issue`。

### `issue_comment`

一条面向人的 Issue 评论。

| 列 | 类型 | 可空 | 说明 |
|---|---|---|---|
| `id` | `bigint unsigned` | 否 | 内部主键 |
| `public_id` | `char(20) ascii_bin` | 否 | 公开句柄，唯一 |
| `issue_id` | `bigint unsigned` | 否 | `issue.id` |
| `author_kind` | `varchar(16)` | 否 | `user`、`agent`、`local_agent` 或 `system` |
| `author_id` | `varchar(64)` | 否 | `user_id` 或 `agent_id`；`local_agent` 时为报告人；`system` 时为空 |
| `body` | `text` | 否 | 原样存储的 Markdown 源码；服务限制为 16 KiB |
| `source_task_id` | `bigint unsigned` | 是 | 在 `agent` 评论上设置；`local_agent` 评论不指向运行，绝不设置 |
| `source_task_run_id` | `bigint unsigned` | 是 | 在 `agent` 评论上设置；`local_agent` 评论不指向运行，绝不设置 |
| `created_at` | `datetime(6)` | 是 | `autoCreateTime` |
| `edited_at` | `datetime(6)` | 是 | 正文修改前为 `NULL` |

索引：主键 `id`；(`issue_id`, `created_at`) 上的索引 `idx_issue_comment_issue_created`；唯一索引 `public_id`。

先按 `created_at`、再按 `id` 排序——评论串从最早内容读起，公开句柄是随机的，不按时间排列。

此行**没有 `space_id`**。评论的 Space 就是其 Issue 的 Space，每个处理器本来就会加载 Issue 进行授权；反规范化授权键会多出一个可能出错的位置。这与 `conversation_message` 通过 Conversation 解析 Space 的方式一致。

删除是硬删除，没有 `deleted_at`，也没有墓碑标记。只有评论作者本人可以编辑；Agent 或系统评论是运行报告的记录，任何人都不能编辑，不过 Space owner 可以删除。

### `agent`

存储的 Agent 定义：Task 可以使用的一组名称和系统指令。

| 列 | 类型 | 可空 | 说明 |
|---|---|---|---|
| `id` | `bigint unsigned` | 否 | 内部主键 |
| `public_id` | `char(20) ascii_bin` | 否 | 公开句柄，唯一 |
| `user_id` | `bigint unsigned` | 否 | 所属用户 |
| `space_id` | `bigint unsigned` | 是 | 所属 Space |
| `name` | `varchar(255)` | 否 | |
| `description` | `text` | 是 | 在选择器中显示 |
| `instructions` | `text` | 是 | 使用此 Agent 的运行将其追加到系统提示词 |
| `plugins` | `text` | 是 | 此 Agent 加载的目录插件名称的 JSON 数组 |
| `sandbox_network_tier` | `varchar(64)` | 是 | `none`、`registries` 或 `open`；为空则继承 Space 默认值，再回退到界面基线 |
| `sandbox_filesystem_tier` | `varchar(64)` | 是 | `workspace`、`workspace_plus_shared_read` 或 `workspace_plus_external_write`；与网络级别使用相同回退规则 |
| `revision` | `bigint` | 否 | 保存此内容的 `agent_revision` 行的修订号；从 1 开始 |
| `deleted_at` | `datetime(6)` | 是 | Agent 删除时设置；行保留 |
| `created_at` | `datetime(6)` | 是 | `autoCreateTime` |

索引：主键 `id`；索引 `deleted_at`；索引 `space_id`；索引 `user_id`；唯一索引 `public_id`。

删除只写入 `deleted_at`，不执行 `DELETE`。Task、WorkflowStepRun 和修订都通过 ID 引用 Agent，移除行会让它们全部变成悬空引用，并让仍在进行的 WorkflowRun 在下一步失败。因此读取分为两类：`GetAgent` 和列表查询只查看存活 Agent，避免用已删除 Agent 启动新工作；`GetAgentIncludingDeleted` 则解析已有记录的引用。若 `published` Workflow 仍引用某 Agent，删除会被拒绝并返回 `409`——该 Workflow 仍可运行，否则错误会在下次运行时才暴露，而不是在删除时暴露。草稿和归档 Workflow 不阻止删除，因为两者都不能启动运行，而且发布时会重新验证 Agent。

`plugins` 指定目录插件，而不指定发布版本：版本和摘要来自 Space 的 `plugin_activation` 行，因此将插件切换到新版本始终只需修改一处。不从 Space 的激活项隐式继承任何插件——未指定插件的 Agent 不加载插件——列表存储前去除首尾空白、去重并排序，因此重排同一集合不会追加修订。采用 JSON 列而非关联表，是因为不查询其内部：选择整体写入、整体读取，“哪些 Agent 指定了此插件”通过扫描一个 Space 的 Agent 得出。

没有撤销删除的路由。保留行是为了让引用可解析，不是将其用作回收站。

这些是服务端 Agent 记录，与 `.buildmax/` 下通过 Markdown 文件定义的工作区 subagent 不同；见 [help/skills-and-subagents.md](../../../../help/skills-and-subagents.md)。

### `agent_revision`

Agent 定义的一次版本记录。行仅追加，从不更新或删除。

| 列 | 类型 | 可空 | 说明 |
|---|---|---|---|
| `id` | `bigint unsigned` | 否 | 内部主键 |
| `agent_id` | `bigint unsigned` | 否 | `agent.id` |
| `revision` | `bigint` | 否 | 首次记录内容为 1，此后每次变更加一 |
| `name` | `varchar(255)` | 否 | |
| `description` | `text` | 是 | |
| `instructions` | `text` | 是 | |
| `plugins` | `text` | 是 | JSON 数组；此修订记录的选择 |
| `sandbox_network_tier` | `varchar(64)` | 是 | 此修订记录的级别 |
| `sandbox_filesystem_tier` | `varchar(64)` | 是 | 此修订记录的级别 |
| `created_by` | `bigint unsigned` | 否 | 写入此修订的用户，不一定是 Agent owner |
| `created_at` | `datetime(6)` | 是 | `autoCreateTime` |

索引：主键 `id`；(`agent_id`, `revision`) 上的唯一索引 `idx_agent_revision`。

修订与其描述的 Agent 行在同一事务中写入，唯一的 (`agent_id`, `revision`) 索引让并发的第二次写入失败，而不是将两个定义记在同一修订号下。没有实际变化的更新不追加修订。恢复早期修订是普通更新：追加一条包含旧内容的新修订，而非把修订号倒退回去。

修订的生命周期长于 Agent 的使用期：删除的 Agent 保留历史，过去运行的来源信息正是指向这些历史。修订路由只服务存活 Agent，因此读取已删除 Agent 的历史需要查询表。

修订历史引入前已经存在的 Agent 和 Workflow，由迁移 `0003_seed_first_agent_and_workflow_revision` 补充修订 1。这一行是近似记录：作者是记录创建者，时间戳是内容上次变动的时间，两者未必能标识产生所存内容的那次编辑。

### `conversation`

独立的前台聊天，以及可选的 Agent Task 编排器。它拥有自己的消息，而不拥有可能启动或展示的 Task。

| 列 | 类型 | 可空 | 说明 |
|---|---|---|---|
| `id` | `bigint unsigned` | 否 | 内部主键 |
| `public_id` | `char(20) ascii_bin` | 否 | 公开句柄，唯一 |
| `user_id` | `bigint unsigned` | 否 | 所属用户 |
| `space_id` | `bigint unsigned` | 是 | 所属 Space |
| `channel` | `varchar(32)` | 否 | `portal`、`telegram`、`cron`、`webhook`，或合成的 `workflow` / `issue_agent` |
| `title` | `varchar(256)` | 是 | 根据第一轮生成 |
| `created_by` | `bigint unsigned` | 否 | `user.id` |
| `created_at` | `datetime(6)` | 是 | `autoCreateTime` |

索引：主键 `id`；(`space_id`, `created_at`) 上的索引 `idx_conversation_space_created`；(`user_id`, `created_at`) 上的索引 `idx_conversation_user_created`；唯一索引 `public_id`。

传输渠道常量位于 `internal/service/conversation/channel/types.go`。`system` 常量存在，但不在 `ValidChannels` 中，因此调用者不能传入它。

Workflow 步骤和 Issue Agent 运行都直接创建 Task，以 `task.space_id` 作为所有权依据，不设 `conversation_id`；两者均不会创建 Conversation 来挂载 Task。见 [Agent 执行与 Task 线程](../../../design/agent-execution-and-task-threads.md)。

### `conversation_message`

Tier 1 Conversation 中的一条消息，包括工具交互。

| 列 | 类型 | 可空 | 说明 |
|---|---|---|---|
| `id` | `bigint unsigned` | 否 | 内部主键 |
| `public_id` | `char(20) ascii_bin` | 否 | 公开句柄，唯一 |
| `conversation_id` | `bigint unsigned` | 否 | `conversation.id` |
| `role` | `varchar(16)` | 否 | LLM 消息角色 |
| `content` | `text` | 否 | |
| `channel` | `varchar(32)` | 是 | 为此消息覆盖 Conversation 的渠道 |
| `tool_call_id` | `varchar(64)` | 是 | 在工具结果上设置，将其关联到产生它的调用 |
| `tool_calls` | `text` | 是 | assistant 消息上的工具调用 JSON 数组；Go 字段为 `ToolCallsJSON`，列名为 `tool_calls` |
| `provider_state` | `text` | 是 | assistant 消息上的不透明推理状态，存储并重放，但此处从不读取其内容；Go 字段为 `ProviderStateJSON` |
| `parts` | `mediumtext` | 是 | 消息上的非文本内容，例如工具返回的图像；`content` 仍保存描述它的文本。Go 字段为 `PartsJSON` |
| `created_at` | `datetime(6)` | 是 | `autoCreateTime` |

索引：主键 `id`；(`conversation_id`, `created_at`) 上的索引 `idx_conversation_message_conversation`；唯一索引 `public_id`。

先按 `created_at`、再按 `id` 排序。带前缀的 ID 是随机的，不按时间排列，因此绝不能按 `conversation_message_id` 排序。

`provider_state` 保存协议产生并要求原样返还的内容，例如 Anthropic thinking block、OpenAI Responses reasoning item。Tier 1 轮次从这些行恢复，因此没有它，第二轮就会向上游发送会被拒绝的 Conversation。列出现前写入的行，或者内容已无法解析的行，重放为不带状态的消息，而不是使该轮失败。见 [design/llm-provider-adapters.md](../../../design/llm-provider-adapters.md)。

## 后台执行

Task 加 task_run 构成持久的 Agent 执行。TaskRun 拥有结果；Conversation、Issue 和 Workflow 视图可以通过显式的可选关系投影结果。

### `task`

后台工作的持久单元。一个 Task，多次尝试。

| 列 | 类型 | 可空 | 说明 |
|---|---|---|---|
| `id` | `bigint unsigned` | 否 | 内部主键 |
| `public_id` | `char(20) ascii_bin` | 否 | 公开句柄，唯一 |
| `conversation_id` | `bigint unsigned` | 是 | 可选的来源/投影关系；直接创建的 Agent、Issue 或 Workflow Task 没有此关系 |
| `space_id` | `bigint unsigned` | 否 | 所属 Space，是每项 Task 操作的权威依据 |
| `issue_id` | `bigint unsigned` | 是 | 此 Task 推进的 Issue（如果有） |
| `status` | `varchar(32)` | 否 | `PENDING`、`SCHEDULED`、`RUNNING`、`SUCCEEDED`、`FAILED`、`CANCELED` |
| `input` | `text` | 否 | 提示词 |
| `title` | `varchar(256)` | 是 | 由 LLM 生成 |
| `title_prompt_tokens` | `bigint` | 是 | 生成标题消耗的 token，计入配额 |
| `title_completion_tokens` | `bigint` | 是 | 同上 |
| `output` | `text` | 是 | 最近一次成功运行的结果 |
| `created_by` | `bigint unsigned` | 否 | `user.id` |
| `created_at` | `datetime(6)` | 是 | `autoCreateTime` |
| `started_at` | `datetime(6)` | 是 | 首次运行开始时间 |
| `ended_at` | `datetime(6)` | 是 | 进入终态的时间 |
| `error_message` | `text` | 是 | |
| `session_id` | `varchar(36)` | 是 | Agent Session 文件的 UUID，不是表引用 |
| `last_run_id` | `bigint unsigned` | 是 | 最近一次尝试的 `task_run.id` |
| `agent_id` | `bigint unsigned` | 是 | 此 Task 以哪个 `agent.id` 运行 |
| `workspace_head_checkpoint_id` | `bigint unsigned` | 是 | 被接受为 Task 可恢复工作区的 `workspace_checkpoint.id`；先是种子，再是每次成功结果。首次运行提交前为空 |
| `plugin_environment_head_id` | `bigint unsigned` | 是 | 下一次 Continue 使用的不可变 Plugin 环境；未自主安装任何插件的 Task 为空 |

索引：主键 `id`；索引 `agent_id`；索引 `conversation_id`；索引 `issue_id`；索引 `last_run_id`；索引 `workspace_head_checkpoint_id`；索引 `plugin_environment_head_id`；(`space_id`, `created_at`) 上的索引 `idx_task_space_created`；唯一索引 `public_id`。

状态值为 `task.RunStatus`，使用大写，与 `task_run` 共享。

### `task_run`

一次执行尝试。配额和 token 计量读取此行。

| 列 | 类型 | 可空 | 说明 |
|---|---|---|---|
| `id` | `bigint unsigned` | 否 | 内部主键 |
| `public_id` | `char(20) ascii_bin` | 否 | 公开句柄，唯一 |
| `task_id` | `bigint unsigned` | 否 | `task.id` |
| `previous_task_run_id` | `bigint unsigned` | 是 | Task 线性运行历史中不可变的前驱；worker 恢复该运行的 Session 包 |
| `input` | `text` | 否 | 本次尝试的提示词；重新运行时可与 Task 的提示词不同 |
| `created_by` | `varchar(64)` | 是 | `user.id`，系统触发的运行为空 |
| `created_by_type` | `varchar(32)` | 是 | `user`、`webhook` 或 `system` |
| `trigger_source` | `varchar(64)` | 是 | `task_create`、`task_rerun`、`portal_conversation`、`portal_task_create`、`portal_task_rerun`、`issue_agent_run`、`workflow_step`、`webhook` |
| `status` | `varchar(32)` | 否 | 与 `task` 相同的 `task.RunStatus` 值 |
| `output` | `text` | 是 | |
| `error_message` | `text` | 是 | |
| `started_at` | `datetime(6)` | 是 | |
| `ended_at` | `datetime(6)` | 是 | 运行中为 `NULL` |
| `session_id` | `varchar(36)` | 是 | 本次运行的 Session 文件 UUID |
| `worker_type` | `varchar(32)` | 是 | `local_process` 或 `k8s_job`；写入时机见下文 |
| `k8s_job_name` | `varchar(128)` | 是 | 执行此次运行的 Job；本地 runner 下为 `NULL` |
| `k8s_job_created_at` | `datetime(6)` | 是 | 该 Job 的创建时间；本地 runner 下为 `NULL` |
| `prompt_tokens` | `bigint` | 是 | 配额输入 |
| `completion_tokens` | `bigint` | 是 | 配额输入 |
| `trace_path` | `varchar(512)` | 是 | 本次运行在运行级全局存储中的持久 trace，例如 `traces/<session>/rt_….jsonl`；未写入时为 `NULL` |
| `cancel_requested_at` | `datetime(6)` | 是 | 有人请求停止此次运行的时间；无人请求时为 `NULL` |
| `cancel_requested_by` | `bigint unsigned` | 是 | 请求者的 `user.id` |
| `retry_of_task_run_id` | `bigint unsigned` | 是 | 本次重复执行的运行；携带自身指令的运行为 `NULL` |
| `source_message_id` | `bigint unsigned` | 是 | 请求本次运行的 `conversation_message.id`；没有消息发起请求时为 `NULL` |
| `agent_revision` | `int` | 是 | 本次运行收到的 `task.agent_id` 修订号；没有 Agent 或从未到达 worker 的运行为 `NULL` |
| `space_agent_instructions_revision` | `int` | 是 | 本次运行收到的所属 Space 的 Space 级指令修订号；`0` 表示未配置文本，`NULL` 表示没有来源记录 |
| `plugin_pins` | `text` | 是 | `{plugin_name, version, digest}` 的 JSON 数组：本次运行获得的发布版本 |
| `sandbox_network_tier` | `varchar(64)` | 是 | 首次轮询时解析的级别，依次采用 Agent 声明、Space 默认值、界面基线；worker 认领前为 `NULL` |
| `sandbox_filesystem_tier` | `varchar(64)` | 是 | 首次轮询时解析的级别，与 `sandbox_network_tier` 使用相同回退规则 |
| `last_seen_at` | `datetime(6)` | 是 | 此次运行的 worker 最近轮询自身路由的时间；worker 认领前为 `NULL` |
| `idempotency_key` | `varchar(128)` | 是 | 调用者为 Continue 请求提供的去重键；未带键创建的运行为 `NULL`，包括重试、Workflow 步骤、Issue Agent 运行或旧客户端 |
| `workspace_base_checkpoint_id` | `bigint unsigned` | 是 | 本次运行获准读取和修改的 `workspace_checkpoint.id`，执行前固定 |
| `workspace_result_checkpoint_id` | `bigint unsigned` | 是 | 本次运行提交的成功结果检查点 |
| `workspace_partial_checkpoint_id` | `bigint unsigned` | 是 | 失败、取消或中断后捕获的部分检查点；绝不作为 Task head |
| `workspace_restore_status` | `varchar(32)` | 是 | `not_requested`、`pending`、`restored` 或 `failed` |
| `workspace_restore_error` | `text` | 是 | 面向操作员、长度受限的恢复失败原因 |
| `workspace_checkpoint_status` | `varchar(32)` | 是 | `not_requested`、`pending`、`committed` 或 `failed` |
| `workspace_checkpoint_error` | `text` | 是 | 面向操作员、长度受限的捕获或提交失败原因 |
| `plugin_environment_base_id` | `bigint unsigned` | 是 | 为本次运行物化的不可变 Plugin 集合 |
| `plugin_environment_result_id` | `bigint unsigned` | 是 | 已提交的自主安装请求的新 Plugin 集合；在下一个 TaskRun 边界生效 |
| `plugin_environment_status` | `varchar(32)` | 是 | `unchanged`、`pending`、`committed` 或 `failed` |
| `plugin_environment_error` | `text` | 是 | 长度受限的安装或物化原因 |
| `created_at` | `datetime(6)` | 是 | `autoCreateTime` |

索引：主键 `id`；索引 `cancel_requested_at`；索引 `created_by`；索引 `last_seen_at`；索引 `previous_task_run_id`；索引 `retry_of_task_run_id`；索引 `source_message_id`；(`task_id`, `created_at`) 上的索引 `idx_task_run_task_created`；唯一索引 `public_id`；(`task_id`, `idempotency_key`) 上的唯一索引 `idx_task_run_idempotency`。

对同一 Task 使用相同幂等键重复发送 `POST .../tasks/{task_id}/runs`，会返回首次调用创建的运行，而不启动第二次运行；无论原运行仍在活动还是已经结束都如此。MySQL 唯一索引中 `NULL` 不视为另一个 `NULL` 的重复，因此所有未带键创建的运行都能共存。`CreateTaskRun` 在检查这一点及下述活动运行计数前，先对 Task 行加锁读取，因此同一 Task 的两个并发调用者不能都看到空状态并各自插入；见 [Agent 执行与 Task 线程 §12](../../../design/agent-execution-and-task-threads.md#12-failure-recovery-and-concurrency)。

`agent_revision` 不是对 `agent_revision.id` 的引用：修订通过 Agent 加修订号寻址，而 Task 已持有 Agent。worker 请求其运行时写入此值，首次写入生效。指令按派发解析，使编辑在下次运行生效；保留此记录，使运行期间的编辑不能改写该运行实际收到的内容。

`space_agent_instructions_revision` 遵循相同的首次写入生效规则。worker 将所属 Space 当前的 Space 指令作为独立系统提示词层接收，放在选定 Agent 的指令之前；编辑 Space 影响下次运行，而非正在执行的运行。

`plugin_pins` 在同一时刻按同一规则写入，因为它回答的是同一运行的同类问题。服务端根据 Agent 的选择解析 Space 的 `plugin_activation` 行并发送完整列表；worker 从不自行读取激活项。在认领时而非派发时解析是安全的，因为激活项指定精确版本和摘要——阻止中途发布的新版本改变运行所加载内容的是固定引用，而非解析时机。trace 携带同样的清单，但 trace 失败时放行且存于运行级全局存储，因此此列是可查询的事实，也是重试读取的来源。Agent 未指定插件、运行没有 Agent 或从未到达 worker 时，此列为空。

`source_message_id` 对应用户实际说的话；`input` 是 Tier 1 决定发送给 worker 的内容。两者是不同文本，保留两者正是目的：`input` 中缺失的约束，可能是模型遗漏，也可能是用户从未提供，模式中其他内容无法区分。每次运行各自记录：Task 首次运行指向创建它的消息，继续执行指向提出该请求的消息。所有非消息来源都为 `NULL`，包括 Workflow 步骤、Issue Agent 运行、重试和直接通过 API 创建的 Task。句柄无法解析时将列留为 `NULL`，而不拒绝运行；丢失来源记录优于拒绝用户请求的工作。

`retry_of_task_run_id` 指向一次重试所重复的运行，每行一条链接：重试的重试指向其所重复的运行，而非链首。它不是外键，所指向的运行永不修改——重试是新尝试，不是改写解释为何需要重试的记录。对应的 `trigger_source` 为 `task_retry`。

`previous_task_run_id` 不同于重试谱系：它指定新运行获准执行前一刻的当前运行，无论新运行是 Continue 还是 Retry。它与推进 `task.last_run_id` 在同一事务中写入，此后永不改变。worker 用此不可变值恢复先前的 Session 包；若读取 Task 投影，在新运行获准后只会得到当前运行。

`cancel_requested_at` 是请求，不是状态：worker 已持有的运行保持 `RUNNING`，直到该 worker 报告 `CANCELED`，因为其他组件无法结束另一个进程的 Agent 循环。worker 通过轮询自身运行路由看到请求；若 worker 始终不确认，`StaleRunReaper` 会结束运行，这也是关闭被遗弃运行的同一兜底机制。两列都只写一次，第二次取消不会覆盖首位请求者。

`last_seen_at` 使服务端能区分 worker 已终止还是执行缓慢。它仅在 `GET /api/worker/task-runs/{id}` 上写入：运行处于 `RUNNING` 的整个期间，worker 每隔几秒轮询该路由，因此信号早已存在，只需记录下来。流式路由每秒触发多次，有意不更新时间戳；终态 `PATCH` 也不更新，否则时间戳会晚于工作停止时刻。`StaleRunReaper` 读取它，在 `RUNNING` 运行失去响应数分钟后将其标为失败，而不等待 `worker.run_timeout`。`NULL` 永不因沉默而被回收：从未记录过信号，便谈不上变得安静。

runner 无错误返回后，`Scheduler.dispatch` 通过 `UpdateTaskRunWorkerInfo` 写入 `worker_type`、`k8s_job_name` 和 `k8s_job_created_at`。这一时刻因 runner 而异，读取这些列前必须理解差异：

- **`k8s_job`** 在 Job 创建后立即返回，因此三列都在派发时写入，整个运行期间可读。
- **`local_process`** 在整个运行期间阻塞，因此 `worker_type` 只在 worker 退出后写入，且仅限正常退出；启动或退出失败会走失败路径，转而记录错误。两个 Kubernetes 列保持 `NULL`。

目前没有代码读取这些列。未来的清理扫描可用 `k8s_job_name` 向 Kubernetes 查询 worker 消失的原因——从服务端看，`OOMKilled` 和 `Evicted` 都只是沉默——但这只覆盖 `k8s_job` runner，因此过期运行回收器观察的是 `last_seen_at`，而非 Job 状态。

worker 在终态 PATCH 中写入 `trace_path`，成功和失败都写。它采用存储值而非推导值，因为 trace 文件名是 Agent run id，该 ID 在运行内部生成，不出现于其他位置。值与 `uploadTaskGlobal` 上传文件所用的键一致，因此可直接在运行级全局存储中解析；`internal/agentapp/taskrun` 的测试将两处计算绑定，避免偏离。

调度器通过轮询最早的待处理运行（`GetNextPendingTaskRun`）认领工作；GORM 日志器配置为忽略 `ErrRecordNotFound`，避免空闲服务端每次轮询都记录未找到。
