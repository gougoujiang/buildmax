# Entity Identity And Relational Keys

> **Audience:** contributors and database reviewers · **Status:** proposal — under discussion
>
> **Opened:** 2026-08-22

Related: [../ROADMAP.md](../ROADMAP.md) Beta gate,
[data model](../contribute/architecture/data-model.md),
[store architecture](../contribute/architecture/store.md), and
[ID helper architecture](../contribute/architecture/util.md).

## Problem

BuildMax gives most server entities two identifiers:

- an auto-increment `bigint unsigned` primary key named `id`; and
- a public `varchar(64)` identifier such as `t_...` or `tm_...`.

The numeric key is internal, but it is not the relational key. Foreign
references, joins, authorization filters, quota aggregation, and most
secondary indexes use the public string. The schema therefore pays for two
identifiers on each public entity without receiving the main benefit of the
numeric one: compact and unambiguous relationships.

The type prefix does not compensate for that cost. Routes, JSON fields, log
attributes, and SQL columns already name the entity type, and the application
does not validate or dispatch on the prefix. Prefixes are useful when a bare
identifier reaches a person, but they are currently a convention rather than
a type-safety boundary. The prefix registry has also drifted as new entities
were added and old artifact concepts changed.

BuildMax is in Alpha and has no identifier compatibility commitment. This is
the least expensive point at which to decide whether the current format is a
durable contract or an implementation that should be replaced before Beta.

## Goals

- Make ordinary entity relationships use compact database-native keys.
- Keep public identifiers opaque, non-enumerable, URL-safe, and independent of
  database topology.
- Give each identifier one role: relational identity inside MySQL or durable
  identity outside it.
- Keep database primary keys out of API responses, URLs, JWT claims, logs,
  traces, workflow definitions, and object-storage paths.
- Define which string identifiers are intentionally not relational keys.
- Use the Alpha window for one coherent schema change rather than permanent
  dual reads or mixed identifier formats.

## Non-Goals

- Making an identifier an authorization credential. Every resource lookup
  still enforces team membership and returns the same not-found/forbidden
  behavior regardless of identifier entropy.
- Ordering by a public identifier. Durable ordering continues to use
  `created_at` plus the internal primary key where a tie-breaker is required.
- Replacing provider IDs, tool-call IDs, session IDs, idempotency keys, or
  other identifiers owned outside the server entity model.
- Hiding raw IDs in Portal. Displaying names and offering a copy affordance is
  a separate presentation change.
- Quietly changing the repository's no-database-foreign-key invariant. Whether
  to add constraints is a separate explicit decision below.

## Current Model

The representative shape is:

```text
task
  id               bigint unsigned primary key
  task_id          varchar(64) unique       # public
  conversation_id  varchar(64) index        # public conversation ID
  team_id          varchar(64) index        # public team ID

task_run
  id               bigint unsigned primary key
  task_run_id      varchar(64) unique       # public
  task_id          varchar(64) index        # public task ID
```

This has four consequences:

1. Every relationship repeats an opaque string and indexes it again.
2. MySQL compares identifiers under the database's string collation unless
   each column overrides it. The database creator selects `utf8mb4` but does
   not give opaque identifiers an ASCII binary collation.
3. The core models carry both `ID uint` and a public field such as
   `TaskID string`, even though the numeric value is a persistence detail.
4. Public identity is coupled to relation columns, raw SQL joins, and schema
   documentation, making a presentation-level format decision expensive to
   revisit.

The cost is concentrated in tables that grow with activity rather than with
administrative configuration: `conversation_message`, `task_run`,
`task_run_artifact`, `llm_call`, workflow step runs, and audit events.

## Options

| Option | Strength | Main concern |
|---|---|---|
| Keep prefixed strings as relational keys | No implementation work; bare IDs remain recognizable | Retains duplicate identity, wide relation indexes, collation semantics, and prefix-registry maintenance |
| Remove prefixes but keep string relationships | Simpler external appearance | Changes presentation without fixing the relational model |
| Keep numeric primary keys, add a short random public ID, and make strict relationships numeric | Uses the primary keys already present; `bigint` joins are compact; external identity is short and non-enumerable | Repository queries must translate at the storage boundary when callers supply or need a related public ID |
| Use a time-ordered `binary(16)` public value as the primary and relational key | One identifier, no secondary public-ID lookup, distributed generation | Doubles the clustered key size versus `bigint`; the key is copied into every InnoDB secondary index; requires a binary/text codec throughout persistence |
| Expose auto-increment IDs directly | Smallest schema and shortest joins | Enumerable, reveals insertion order, couples external contracts to one database, and is awkward for import or replication |

## Likely Direction

Use the existing numeric primary key for strict database relationships, and
give externally addressable entities a separate unprefixed `public_id`.

```sql
CREATE TABLE task (
    id              BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    public_id       BINARY(12) NOT NULL,
    conversation_id BIGINT UNSIGNED NOT NULL,
    team_id         BIGINT UNSIGNED NOT NULL,
    issue_id        BIGINT UNSIGNED NULL,
    agent_id        BIGINT UNSIGNED NULL,
    last_run_id     BIGINT UNSIGNED NULL,
    PRIMARY KEY (id),
    UNIQUE KEY uq_task_public_id (public_id),
    KEY idx_task_conversation (conversation_id),
    KEY idx_task_team (team_id)
);

CREATE TABLE task_run (
    id          BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    public_id   BINARY(12) NOT NULL,
    task_id     BIGINT UNSIGNED NOT NULL,
    retry_of_id BIGINT UNSIGNED NULL,
    PRIMARY KEY (id),
    UNIQUE KEY uq_task_run_public_id (public_id),
    KEY idx_task_run_task (task_id)
);
```

The names distinguish the two roles consistently:

| Name | Meaning |
|---|---|
| `id` | This row's internal numeric primary key |
| `public_id` | This row's opaque external handle, only when one is needed |
| `<entity>_id` | Numeric relation to that entity inside the database |
| `<concept>_public_id` | Deliberate stored external or polymorphic handle; uncommon and named explicitly |

The example omits `FOREIGN KEY` clauses intentionally. Numeric references and
database-enforced foreign keys are separable decisions.

### Public ID Format

The likely public format is 96 bits of cryptographically secure random data,
stored as the raw 12 bytes in MySQL and encoded without padding as 16
base64url characters at every external boundary:

```text
Z2x9pQ7mK4vN8cRa
```

The text form uses the RFC 4648 URL-safe alphabet (`A-Z`, `a-z`, `0-9`, `-`,
and `_`). It is case-sensitive. Parsers accept exactly one canonical form: 16
characters, no padding, decoding to exactly 12 bytes. MySQL compares the raw
`BINARY(12)` value, so text collation never affects identity.

At one billion generated values in one table, the birthday-bound collision
probability is approximately `6.3e-12`. A database uniqueness constraint is
still the final collision guard; a create path that observes a conflict on the
public-ID unique index generates a new value and retries within a small fixed
limit, while every other duplicate-key error is returned unchanged. With one
billion live values, a uniformly random guess names any one of them with
probability approximately `1.3e-20`. Public-ID opacity is defense in depth,
not a replacement for team authorization or indistinguishable not-found
responses.

This deliberately trades the standard UUID text ecosystem for a shorter user
and operator-facing handle. It is not UUIDv7, does not contain a timestamp,
and reveals neither insertion order nor an approximate resource count. The
internal auto-increment key supplies index locality; the random public ID is a
single secondary unique index rather than the clustered key copied into every
secondary index.

Public IDs are unique per entity table. BuildMax has no global resource
resolver, and a route or JSON field supplies the type context. A cross-table
registry would add a write bottleneck for no authorization or collision
benefit.

### Boundary Rule

Public IDs cross process and persistence-system boundaries. Numeric IDs do
not.

| Boundary | Representation |
|---|---|
| HTTP request and response | Public ID |
| Portal route and client state | Public ID |
| JWT and worker-run claim | Public ID |
| Logs, traces, and tool output | Public ID |
| Object-storage key | Public ID |
| Workflow definition JSON | Public ID |
| Repository implementation and SQL relationships | Numeric ID |
| Core model | Public ID; no storage primary key |

The store decodes a supplied public ID into 12 bytes and resolves it at its
boundary. A lookup rooted at a public resource selects its numeric key once
and uses numeric relationships for the rest of the operation. List and detail
queries join the parent table when they must return a related public ID. Hot
aggregation queries, including quota usage, remain entirely numeric after the
root team has been resolved.

The core packages should not grow a second internal-ID API to avoid one lookup.
That would move a MySQL implementation detail above `internal/infra/db` and
make every mock and alternative store reproduce it. The row structs own both
representations; domain models own only the public one.

### API Contract

The HTTP resource topology does not change. Existing paths keep their semantic
parameters:

```text
GET /api/teams/{team_id}/tasks/{task_id}
GET /api/teams/{team_id}/task-runs/{task_run_id}/trace
GET /api/admin/users/{user_id}
```

The parameter value changes from a prefixed ID to the canonical 16-character
public form. The route registration tree remains the source of truth:
[top-level routes](../../internal/server/handlers/routes.go),
[team routes](../../internal/server/handlers/team/handler.go),
[work routes](../../internal/server/handlers/work/handler.go), and
[admin routes](../../internal/server/handlers/admin/handler.go). This document
does not copy their full list.

`public_id` is a database name, not an API field. A resource represents its
own public handle as `id`; relationships keep semantic names such as
`team_id`, `task_id`, and `agent_id`:

```json
{
  "id": "Q8m2xP4vN7kL3aBc",
  "team_id": "Z2x9pQ7mK4vN8cRa",
  "conversation_id": "F7n4wK2pR9mX6cDe"
}
```

Every ID remains a JSON string. Neither an API request nor a response exposes
the numeric database key. Create responses that return a related handle under
a semantic name such as `conversation_id` or `task_run_id` may keep that name;
the proposal does not require unrelated response-shape churn.

One shared codec validates IDs from path parameters, query parameters, and
JSON bodies. A malformed value returns `400 Bad Request`. A canonical value
that names no row returns `404 Not Found`; a row outside the caller's team also
returns `404` so a valid public ID does not become a cross-team existence
oracle. A team-scoped lookup resolves the team's public ID, then constrains the
resource lookup by both its public ID and the numeric team key.

The main HTTP surfaces affected are admin user/team/model operations, team
membership and agents, issues and comments, workflows and runs,
conversations, tasks and task runs, artifact/trace/LLM-call reads, webhook-key
management, and the team WebSocket. Endpoint names and nesting stay unchanged.
Revision path parameters remain integers; agent and workflow revisions are
identified by parent plus revision number and no longer return a redundant
revision public ID.

Access-token `sub` and worker-run claims `sub`, `tid`, `rid`, and `kid` continue
to carry public IDs. Worker routes compare the canonical `rid` with the route's
`task_run_id`. The Alpha cutover invalidates old access tokens, refresh
sessions, worker tokens, Portal links, and bookmarks instead of accepting both
formats. `/api/webhook` continues to authenticate with the webhook secret, not
the webhook key's public ID.

## Table Impact

The current `AutoMigrate` list contains 25 tables. The proposed split affects
23 of them: 17 externally named entity tables keep or receive `public_id`, six
internal/history tables use numeric relations without `public_id`, and two
natural-key infrastructure tables keep their current identity.

### Externally Named Entities

| Table | Public identity and numeric relationships |
|---|---|
| `user` | Replace `user_id` with `public_id`; no entity parent |
| `team` | Replace `team_id`; make personal user and creator numeric user relations |
| `issue` | Replace `issue_id`; make user, team, parent issue, and creator numeric; keep the typed assignee opaque |
| `issue_comment` | Replace `issue_comment_id`; make issue and source task/run numeric; keep the typed author opaque |
| `agent` | Replace `agent_id`; make user and team numeric |
| `conversation` | Replace `conversation_id`; make user, team, and creator numeric |
| `conversation_message` | Replace message ID; make conversation numeric; keep provider tool-call IDs opaque |
| `task` | Replace `task_id`; make conversation, team, issue, creator, latest run, and agent numeric |
| `task_run` | Replace `task_run_id`; make task, retry source, and a user canceller numeric; keep the typed creator opaque |
| `workflow` | Replace `workflow_id`; make team and creator numeric |
| `workflow_run` | Replace `workflow_run_id`; make workflow, issue, conversation, and creator numeric |
| `workflow_step_run` | Replace step-run ID; make workflow run, target agent, task, and task run numeric |
| `user_webhook_key` | Replace `key_id`; make owner user numeric; the credential hash is unchanged |
| `llm_model` | Replace `llm_model_id`; no entity parent |
| `llm_call` | Replace `llm_call_id`; make team, user, task, and task run numeric; catalog target semantics remain open |
| `audit_event` | Replace `audit_event_id`; make the team scope numeric; keep typed actor and target handles opaque |
| `system_grant` | Replace `system_grant_id`; make the holder numeric; keep `granted_by` opaque because the operator is not a user row |

`workflow_step_run` keeps a public ID even though it has no top-level detail
route: issue outputs persist it as a source correlation handle. `system_grant`
keeps one because the admin API and operator CLI return the grant record as
durable authority history.

### Internal And History Tables

| Table | Identity decision |
|---|---|
| `team_member` | No public ID; team and user become numeric |
| `agent_revision` | Remove `agent_revision_id`; identify by numeric agent plus revision number; creator becomes numeric |
| `workflow_revision` | Remove `workflow_revision_id`; identify by numeric workflow plus revision number; creator becomes numeric |
| `task_run_artifact` | No public ID; task run becomes numeric and remains paired with relative path |
| `login_code` | No public ID; user becomes numeric; code hash remains the lookup key |
| `user_refresh_token` | No row public ID; user becomes numeric; token hash and login-chain session ID retain their authentication formats |

### Unchanged Natural-Key Tables

| Table | Reason |
|---|---|
| `quota_tier` | `tier_name` is a configuration-owned natural key, not an entity handle |
| `schema_migration` | `id` is the migration's permanent authored name |

## Which References Become Numeric

A column becomes numeric when it points to exactly one server entity type and
the application treats the relationship as part of the current data model.

| Area | Numeric relationships |
|---|---|
| Identity and team | Team membership to team/user; personal team to user; login code, refresh token, webhook key, and system grant to user |
| Issues | Issue to team/user/parent issue; comment to issue and, where retained as live relations, source task/run |
| Agents and workflows | Agent to team/user; revision to parent; workflow to team; run to workflow/issue/conversation; step run to workflow run/agent/task/run |
| Conversations and work | Conversation to team/user; message to conversation; task to conversation/team/issue/agent/latest run; run and artifact row to task/run |
| Managed LLM | Call to team/user/task/run and, if catalog deletion semantics permit it, managed model |
| Governance | Audit event to team for team-scoped filtering, provided export can still produce the team's public ID |

This is not a mechanical conversion of every column ending in `_id`.

## References That Stay Opaque

Some identifiers are values or polymorphic correlation handles rather than
ordinary relational keys:

- `audit_event.actor_id` and `target_id`, because their type is carried in a
  separate column and one numeric value cannot identify rows from several
  tables;
- creator/canceller fields whose accompanying type permits `system`, `worker`,
  or `webhook`, until the actor model is normalized;
- agent session IDs and trace run IDs, which identify files rather than rows;
- provider tool-call IDs, provider response IDs, client idempotency keys, and
  upstream model identifiers;
- step IDs inside workflow definitions;
- stable catalog aliases and configuration-owned target IDs;
- secrets, token hashes, and login-chain identifiers, whose formats belong to
  their authentication contracts.

Historical data alone is not a reason to keep a string. A nullable numeric
reference can continue to record a deleted row's former key when database
foreign keys are absent. Polymorphism, external ownership, and boundary
stability are the reasons to retain opaque text.

## Public IDs Are Not Required On Every Table

A row earns a `public_id` only when another process, a user-visible artifact,
or a durable external reference must name it independently. Join tables and
implementation-only rows do not receive one by convention.

Agent and workflow revision rows lose their public IDs because callers already
address them by parent plus revision number. Workflow step runs retain a public
ID because issue outputs persist it as a source correlation handle. Internal
artifact rows, login codes, refresh-token rotations, and membership rows have
no independent external handle.

## Database Foreign Keys

The current architecture deliberately declares no GORM relationships or
database foreign-key constraints. This proposal does not silently reverse that
decision.

Three choices remain:

| Choice | Benefit | Cost |
|---|---|---|
| Keep indexed numeric references without constraints | Preserves current deletion and migration behavior while gaining compact joins | Referential integrity remains an application responsibility |
| Add `RESTRICT`/`NO ACTION` constraints to strict ownership edges | Detects orphan writes and accidental deletion early | Requires an explicit deletion order and makes every existing cleanup/test path conform |
| Add selective constraints only to high-value roots | Limits migration surface | Creates a rule contributors must remember table by table |

The likely first implementation keeps the existing no-constraint rule. A
separate design decision may add constraints after deletion semantics and GORM
migration behavior have dedicated tests. Numeric conversion must not imply
`CASCADE`; deleting a team or task must remain an explicit domain operation.

## Alpha Transition

There should be no compatibility layer between old and new identifiers.

1. Change every row struct to use explicit `uint64` primary and relation
   fields. Do not use architecture-dependent `uint` for persisted schema.
2. Rename public columns to `public_id`, store 12 random bytes as `BINARY(12)`,
   and delete unused public identifiers.
3. Replace prefixed generation with one cryptographically secure 96-bit
   public-ID generator and a strict 16-character base64url codec.
4. Update repository SQL and mappings so numeric IDs stay inside
   `internal/infra/db`.
5. Remove internal numeric IDs from `internal/core/model` structs and expose
   one public identity per entity.
6. Update API DTOs, worker claims, object keys, tests, seed data, and docs in
   the same change.
7. Reset Alpha databases and object stores rather than ship mixed formats or a
   permanent dual-read path. If preserving a particular Alpha deployment is
   later required, write a one-off export/import tool instead of complicating
   the runtime schema.

The implementation must update
[data-model.md](../contribute/architecture/data-model.md),
[store.md](../contribute/architecture/store.md), and
[util.md](../contribute/architecture/util.md) when it lands. Until then those
documents remain the source of truth for current behavior.

## Validation

An accepted implementation is complete when:

- architecture tests reject string columns for strict entity relationships;
- architecture tests reject public entity IDs not stored as `BINARY(12)`;
- repository authorization tests still prevent cross-team lookups;
- quota, artifact, workflow, issue hierarchy, revision, and audit queries use
  numeric joins where the relationship is strict;
- API, Portal, worker, trace, and object-store tests prove that internal numeric
  IDs never cross their boundaries;
- randomness failure, duplicate retry, alphabet, exact length, canonical
  parsing, and malformed-ID tests cover the new generator and codec;
- `EXPLAIN` plans for quota aggregation and conversation/run output queries use
  the intended numeric indexes; and
- a fresh database schema contains no legacy prefixed-ID relation columns.

## Questions To Resolve

- Should creator fields be normalized into nullable numeric user references
  plus an explicit non-user actor, or remain typed opaque handles?
- Does `llm_call.target_id` describe a live catalog relationship, a historical
  snapshot, or both?
- Should the numeric-key change include database `RESTRICT` constraints, or
  should that remain a follow-up after deletion behavior is specified?
- Is an Alpha database reset acceptable for every active deployment, or does
  one deployment need an export/import bridge?

## Evidence Needed For A Decision

- Agreement that the Alpha reset has no production data compatibility burden.
- Agreement that a 96-bit random public handle and its 16-character base64url
  representation are the intended security and usability trade-off.
- A table-by-table review classifying every current `*_id` column as strict,
  polymorphic, external, or merely a value.
- Representative MySQL `EXPLAIN` output and index-size measurements for the
  current and proposed quota, membership, message, and run queries.
- A prototype of one aggregate (`task` plus `task_run` and artifacts) proving
  the storage layer can translate public IDs without leaking numeric IDs into
  service contracts or adding an N+1 query pattern.
- A deletion-semantics review before any database foreign-key constraint is
  accepted.

## Likely Destination If Accepted

The accepted rationale belongs in a durable identity/storage design record.
Implementation updates the database row structs and repository queries under
`internal/infra/db`, simplifies public-ID generation in `internal/util`, and
changes the current data-model and store architecture references in the same
pull request. The Beta schema-migration requirement then starts from the new
model rather than preserving the Alpha layout.
