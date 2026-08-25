# Repository Capability Ownership

> **Audience:** contributors · **Status:** proposal — under discussion
>
> **Opened:** 2026-08-25

Related current sources:

- [Roadmap](../ROADMAP.md)
- [Architecture overview](../contribute/architecture/overview.md)
- [Layering and import rules](../contribute/architecture/packages.md)
- [Repository layout](../contribute/repo-layout.md)
- [Server architecture](../contribute/architecture/server.md)
- [Data model](../contribute/architecture/data-model.md)
- [LLM gateway](../design/llm-gateway.md)
- [System administration](../design/system-administration.md)
- [Portal execution model](../design/portal-execution-model.md)
- [Team governance](../design/team-governance.md)
- [Unified artifacts](../design/unified-artifacts.md)

## Decision Gate

This paper is the repository-wide audit requested before any structural
refactor. It records the evidence, classifies similar concepts, proposes
canonical owners, and breaks the work into independently reviewable batches.
It deliberately changes no production package, API, database shape, or runtime
behaviour.

Implementation should start only after the target ownership and batch order are
accepted. In particular, the audit does not treat fewer packages or fewer lines
as a goal. The desired outcome is that one business fact has one owner and every
boundary translates it explicitly.

## Executive Summary

The repository already has a sound top-level dependency direction and several
well-protected canonical capabilities. The shared Agent loop, task-run state
machine, LLM-facing tool names, database row boundary, plugin inspection, and
artifact/run-output distinction should be preserved. The merge at the audit
baseline, `5d0d9e4`, also removed several obsolete wrappers and centralized
task-run transitions, so repeating that refactor would create churn rather than
clarity.

Eight ownership gaps remain material:

1. The four LLM wire-protocol values and their credential rule are defined
   independently in `config` and `service/llmgateway`.
2. Team permission knowledge is split between the HTTP guard and the team
   service.
3. Account, System Administrator, and model-catalog business actions are
   implemented separately by shell commands and HTTP handlers.
4. Login, refresh rotation, password establishment, logout, and several account
   administration rules live in transport packages instead of an identity
   service.
5. Starting assigned Issue work is partly orchestrated in HTTP handlers, with a
   second workflow-side validation of the same assignment.
6. Artifact and plugin services consume interfaces owned by the infrastructure
   implementation package, reversing the intended consumer-owned port rule.
7. `internal/core/model` is a global domain package: 22 production files span
   unrelated capabilities, and 140 production Go files in 38 directories
   import it.
8. The HTTP contract has three competing descriptions. There are 121 registered
   HTTP method patterns; four serve OpenAPI/Swagger itself, leaving 117
   contract-bearing patterns but only 40 OpenAPI operations. Some documented
   `created_at` fields are integers while the Go wire types are `time.Time`.

The recommended direction is selective, incremental consolidation. Fix the
public API contract first, move shared protocol and authorization knowledge
into pure capability packages, extract transport-owned workflows into services,
move consumer ports to their consumers, and split `core/model` one capability at
a time only after those owners exist. A big-bang vertical rewrite is not
recommended.

## Goals And Non-Goals

### Goals

- Give every durable business concept, state machine, permission rule, protocol
  vocabulary, and public contract one canonical owner.
- Keep HTTP, CLI, configuration, database, and frontend types as explicit
  translations where they have genuinely different responsibilities.
- Make application workflows callable and testable without constructing an
  HTTP request or parsing CLI arguments.
- Preserve the enforced dependency direction and the single-binary Go core.
- Make each implementation batch behaviour-preserving, reviewable, and
  independently revertible.

### Non-goals

- Collapsing packages because they are small.
- Sharing types solely because their current fields happen to match.
- Replacing domain models with GORM rows or exposing storage keys at an API
  boundary.
- Combining local process-scoped work with durable server work.
- Adding an `internal/app` layer, a Node dependency to the Go core, or type
  aliases as migration shims.
- Implementing any of the proposed moves in this audit change.

## Audit Method And Evidence

The audit covered all Go packages under `cmd/`, `internal/`, and `evaluation/`,
the three frontend package roots, route registration, the static OpenAPI
document, build entry points, architecture tests, current design records, and
the latest package-boundary merge.

The discovery pass found 807 Go files under `internal`, 12 top-level service
packages, 22 production files in `internal/core/model`, 253 total Go importers
of that package, and 140 production importers across 38 directories. Textual
search covered constants, validators, normalizers, status predicates, storage
interfaces, error types, route patterns, and duplicated command/handler verbs.
Structural comparison then followed each candidate through callers, stores,
tests, and documentation before classifying it.

The audit is static. It establishes ownership and drift from source, tests, and
documents; it does not claim runtime traffic frequency or production failure
rates. Those measurements are listed as evidence gates where they would affect
batch priority.

## Capability Map

| Capability | Canonical package today | Responsibility, important types, and rules | Main entry points | Persistence or external dependencies |
|---|---|---|---|---|
| Process assembly and configuration | `internal/bootstrap`, `internal/config` | Load files/environment, apply defaults, construct processes; `ServerConfig`, `Settings`, `ModelEntry`, environment allow-list | `cmd/buildmax*`, `cmd/buildmax-server`, `cmd/buildmax-worker`, `cmd/buildmax-desktop`, `cmd/buildmax-eval` | Files, environment, DB/open stores; bootstrap alone assembles them |
| Shared Agent execution | `internal/core/agent`, `internal/core/llm` | Canonical LLM/tool loop, events, grants, tool boundaries, notes, message/tool contracts | `agentapp`, Tier 1 conversation runtime, eval | Injected model, tools, hooks, notes, trace sinks |
| Runtime assembly | `internal/agentapp` | Resolve workspace/runtime options once, build model/tools/MCP/hooks/sandbox/trace/session resources, close partial builds | CLI, Desktop, eval, task-run worker | Config values and `infra` adapters |
| Local sessions | `internal/core/session`, `internal/infra/sessionstore` | Branchable local history, recovery, metadata, file locking; implements Agent message history | CLI/TUI and Desktop session managers | Local JSON/filesystem |
| Local background jobs | `internal/agentapp/job` | Process-scoped detached commands/subagents and monitor lifecycle | TUI/Desktop job surfaces | Memory, process runner, local session state |
| Runtime tools and extensions | `internal/tool`, `internal/core/{plugin,mcp,hook,subagent}`, matching `infra` packages | Canonical LLM tool names, plugin manifests, MCP/hook contracts, sandbox and trace adapters | Agent runtime assembly | Files, subprocesses, MCP servers, hooks, sandbox |
| Identity and user sessions | Models in `internal/core/model`; workflows mostly in `internal/server/handlers/auth` | Users, password hashes, single-use codes, rotating refresh sessions, disabled-user policy | Login/refresh/password/logout HTTP routes; operator user commands | DB stores, JWT signing, password hashing, clock/randomness |
| Deployment administration | Models in `core/model`; workflows in `bootstrap` and `server/handlers/admin` | Accounts, system grants, model catalog, audit actor provenance, last-admin policy | `buildmax-server user/admin/model`; `/api/admin/*` | DB stores, audit recorder, process authority or session authority |
| Team ownership and authorization | `internal/service/team`, `internal/server/access`, team models in `core/model` | Team/member lifecycle and role-to-action matrix | `/api/teams*`, all team-scoped guards | Team/user stores, audit |
| Tier 1 conversations | `internal/service/conversation` | Single user-facing voice, turn queueing, message history, Tier 2 delegation and result delivery | Portal message routes, webhook | Conversation/message/task stores, Agent runtime, scheduler-facing task service |
| Issues | `internal/service/issue` | Issue CRUD, hierarchy invariants, assignee validation, comment ownership/moderation | `/api/teams/{team}/issues*` | Issue, agent, workflow, user stores |
| Agent definitions and workflows | `internal/service/agent`, `internal/service/workflow` | Versioned Agent definitions, plugin selection, linear workflow definitions and workflow/step-run state machines | Team Agent routes, workflow routes, worker completion callback | Agent/workflow/task/issue stores, plugin activation service |
| Durable tasks and task runs | `internal/service/task`, task models in `core/model`, `server/scheduler`, `agentapp/taskrun` | Task creation/retry/cancel, canonical `RunStatus` transitions, scheduling, worker claim/execution/reporting | Conversation/Task routes, scheduler, worker API and binary | DB, K8s/process launcher, run token, run/persist storage |
| Artifacts, run output, and team files | `internal/service/artifact`, `internal/infra/objectstore`, work/artifact handlers | First-class immutable Artifact; separate reproducible run-output index; mutable team home files | Artifact routes and tool, run-output compatibility routes, upload/file routes | Artifact rows, task-run artifact rows, local FS or S3-compatible storage |
| Managed inference, usage, and audit | `internal/service/llmgateway`, `quota`, `audit` | Resolve allowed targets, classify provider failures, ledger calls, enforce rolling-window quotas, best-effort audit | User/worker LLM routes, usage and audit routes | Model/call/quota/audit stores, provider clients, credentials |
| Plugin marketplace and distribution | `internal/service/plugin`, `plugininspect`, `core/plugin`, `infra/pluginarchive` | Validate/publish packages, catalog lifecycle, team activation/pins, materialize one immutable runtime snapshot | Admin/catalog routes, CLI marketplace, task-run materialization | DB, object storage, archive/filesystem, HTTP client |
| HTTP server and trust boundaries | `internal/server`, handler subpackages, `authtoken`, `scheduler` | Route/middleware composition, user/team/admin/run-token boundaries, WebSocket, background scheduling | Server binary | Services, stores, object storage, JWT, K8s/process launcher |
| User surfaces and clients | `internal/interface/{cli,desktop,auth,client,pluginmgr}` | Local presentation, remote client, client credential persistence, plugin install UX | CLI/TUI/Desktop | `agentapp`, server HTTP, keychain/filesystem |
| Web frontends and evaluation | `portal`, `desktop/frontend`, `gui`, `evaluation` | Portal/domain presentation, Desktop UI, shared React presentation, black-box evaluation | npm/Wails builds and eval binary | HTTP API, Wails bindings, launched subject binary |

## What Is Already Canonical

These areas need stronger tests or documentation at most, not another owner:

- `core/model/task.go` owns `RunStatus`, terminal detection, legal transitions,
  and active statuses. `infra/db.TransitionTaskRun` applies that rule atomically
  and projects it to Task state.
- `internal/tool/names.go` owns LLM-visible tool names; hook matchers, subagents,
  and documentation consume the same strings.
- `infra/db` row structs and migrations own the physical schema. Domain/API
  types intentionally do not expose numeric row keys or GORM.
- `service/plugininspect` is a reusable capability reached by publishing, CLI
  validation, installation, and worker materialization. It is not merely a
  helper split from the plugin service.
- `core/agent` is the one Agent loop for local, Desktop, worker, evaluation, and
  Tier 1 execution.
- `service/artifact` and the run-output compatibility path represent two
  deliberately different objects, as decided by the unified-artifacts design.

## Package Findings

Only packages with an ownership problem are listed here. Omission is not a
claim that a package cannot be improved; it means no repository-level move is
justified by the evidence in this audit.

| Package or group | Current responsibility | Intended responsibility | Finding | Proposed owner | Main risk |
|---|---|---|---|---|---|
| `internal/core/model` | Entities, state values, validation helpers, errors, inputs, and stores for identity, team, audit, artifacts, issues, Agents, workflows, tasks, quota, plugins, and managed LLM | Pure capability contracts, but grouped by change reason rather than one global namespace | **Mixed responsibility (7)**. It makes unrelated domains share one import and obscures ownership. The 140 production importers make a big-bang move unsafe. | Capability packages under `core`, migrated one domain at a time | Broad compile churn; architecture tests currently hard-code `core/model`; accidental JSON or store-signature changes |
| `internal/config` + `internal/service/llmgateway` | Each defines the same four wire protocols and credential rule | Config should default/parse; the pure LLM contract should own protocol vocabulary | **Canonical duplication (1)** | `internal/core/llm` | Default-empty config semantics differ from explicit catalog semantics and must remain at the config boundary |
| `internal/bootstrap/model_admin.go` + admin model handlers | Validate and mutate model catalog, record audit, format output | Parse/format at edges; one catalog administration workflow in service | **Canonical duplication (1)** and **mixed responsibility (7)** | New `internal/service/llmcatalog` or an explicit administration facet of `llmgateway` | Credential input must never be exposed by HTTP; audit actor provenance differs by edge |
| Auth handler package | Authentication transport plus password/code verification, token-pair issue, refresh rotation/reuse response, password establishment, logout, and disabled-user rules | Decode/encode HTTP and map service errors | **Mixed responsibility (7)** | New `internal/service/identity` authentication workflows | Security-sensitive timing, token rotation, and anti-enumeration behaviour must be characterized before movement |
| Operator user commands + admin user handlers | Two account creation, login-code, disable/enable, session-revocation, and audit paths | Edge-specific authorization/formatting over one account administration service | **Canonical duplication (1)** | `internal/service/identity` administration workflows | Operator recovery authority and System Admin authority are intentionally different |
| Operator admin commands + admin grant handlers | Two grant/revoke/list implementations and two audit paths | One grant workflow with an explicit last-admin policy supplied by the authorized edge | **Canonical duplication (1)** with one **boundary policy (2)** | New `internal/service/systemadmin` | HTTP must still refuse last-admin revocation; operator command must still allow and warn |
| `internal/server/access` + `internal/service/team` | HTTP action matrix plus a second owner-only member mutation check | One pure role/action decision, used both by guard and service | **Canonical knowledge duplication (1)** | `internal/core/team` policy, enforced by `service/team`; guard translates denial and records it | Changing error visibility could create authorization or enumeration regressions |
| Issue/workflow HTTP handlers | Transport plus assignment/team validation, conversation/task creation, and workflow-run launch | Transport should call an application workflow that owns starting assigned work | **Mixed responsibility (7)** and partial **canonical duplication (1)** | `internal/service/issue`, using narrow conversation/task/workflow ports | Transaction boundaries and partial creation cleanup must be explicit |
| `internal/service/artifact` and `service/plugin` imports of `infra/objectstore` | Application workflow depends on implementation-owned port/ref/error types | Consumer owns its narrow port; local/S3 types remain adapters | **Package fragmentation (6)**: each capability contract is separated from its only business consumer | Port in the consuming service; neutral ref/error vocabulary in the corresponding core capability if adapters need it | Avoid forcing `infra` to import `service`; preserve streaming and cleanup semantics |
| `internal/util/helpers.go` K8s job naming | Generic utilities plus DNS-1123 Job-name policy used by one K8s adapter | Generic formatting stays in util; Kubernetes naming stays with Kubernetes adapter | **Mixed responsibility (7)** | `internal/infra/k8s` | Name stability affects cleanup and lookup; move tests with the function unchanged |
| Route registration + static OpenAPI + Portal API DTOs | 121 registered method patterns (117 after excluding the four OpenAPI/Swagger delivery routes), 40 static OpenAPI operations, manually maintained TypeScript DTOs | One complete machine-checked API contract with explicit frontend mapping | **Canonical/knowledge duplication (1)**. The static contract is incomplete and contains wrong timestamp schemas. | Structured server API contract, generated checked OpenAPI; generated or checked Portal wire DTOs | User-visible contract; generation must not hide security middleware or handwritten response differences |
| Task quota error path | Custom `QuotaExceededError` despite a shared `apierr.KindQuotaExceeded`; two handlers special-case it | Task service uses common service error taxonomy; gateway keeps its protocol-specific classification | **Canonical duplication (1)** at the generic HTTP error boundary | `core/apierr` for Task; `llmgateway.QuotaError` remains local | Preserve the detailed quota reason and 429 response body |

The only production service packages importing `internal/infra` are the
artifact service and plugin service: they reach artifact/package object-storage
contracts, and the plugin service also reaches archive handling.
`pluginarchive` is a valid reusable adapter-level format capability and should
remain; only the object-storage ports are misplaced.

## Duplicate-Concept Inventory

| Concept | Implementations found | Classification | Intentional? | Canonical owner | Consolidation and risks |
|---|---|---|---|---|---|
| LLM provider protocol | `config.LLMProvider*`, `llmgateway.Provider*`; matching list/known/credential functions | 1 — canonical duplication | No | `core/llm` | Move typed values and `Known`/`All`/`NeedsCredential`; keep config's empty-value defaulting. Update adapters and all tests in one batch. |
| Configured model, persisted catalog model, resolved target | `config.ModelEntry`, `model.LLMModel`, `llmgateway.Target` | 2 — boundary translation | Yes | Each boundary; shared vocabulary in `core/llm` | Do not merge structs: one may hold local credentials, one is a durable redacted record, one is an execution target. Centralize only protocol/reasoning/cache vocabulary and validation rules that have one meaning. |
| Model catalog validation and enable/disable | `bootstrap/model_admin.go`, `llmgateway.validateTarget`, admin model handlers | 1 and 7 | No, except edge-specific credential input | `service/llmcatalog` | Characterize accepted inputs, duplicate-name/provider errors, audit, and enable state. Keep add-with-secret operator-only. |
| Team member authorization | `access.isRoleAllowed`, `service/team.requireOwner` and `hasRole` | 1 — canonical knowledge duplication | Defense in depth is intentional; two definitions are not | `core/team` policy called by `service/team` and guard | Preserve HTTP 403/404 policy and audit denial while sharing the pure decision. |
| System role grant/revoke | Bootstrap operator command and admin HTTP handlers | 1 plus 2 | Common action is not; last-admin policy is | `service/systemadmin` | Use an explicit policy/caller authority, not a boolean available to arbitrary HTTP callers. Preserve actor types. |
| Account administration | Bootstrap user commands and admin user handlers | 1 — canonical duplication | No | `service/identity` | Move create/code/disable/session-revoke rules. Keep shell prompts and JSON responses local. |
| Login/session lifecycle | Auth HTTP handlers and persistence primitives | 7 — mixed responsibility, not two complete implementations | No | `service/identity` | Inject token issuer, clock, random source, and stores; retain constant-time/anti-enumeration tests at service and HTTP levels. |
| Starting assigned Issue work | Agent-run and workflow-run handlers; workflow service revalidates issue assignment | 1 and 7 | Revalidation is useful, independent rules are not | `service/issue` application workflow | One method validates Issue/team/assignee then delegates. Define cleanup/transaction behaviour for conversation/task/workflow creation. |
| Quota checker interface | Identical narrow interfaces in `service/task` and `service/llmgateway` | 3 — intentional local duplication | Yes | Consumer-owned interfaces, implemented by `service/quota` | Keep separate: task admission and LLM-call admission can evolve independently. Do not create a generic interface package. |
| Quota refusal errors | `task.QuotaExceededError`, `llmgateway.QuotaError`, `apierr.KindQuotaExceeded` | 2 at the gateway; 1 at generic HTTP | Partly | `apierr` for normal services; gateway error class for managed inference protocol | Remove Task handler special cases only after response characterization. |
| Task-run state | `model.RunStatus*`, DB transition, scheduler/worker callers | 1 — already canonical | Yes | Future `core/task`; today `core/model` | Preserve. DB owns atomic application, not transition knowledge. |
| Issue, workflow, and task statuses | Separate constants/predicates | 4 — coincidental similarity | Yes | Their own domain packages | Do not invent a universal lifecycle. Values, transitions, casing, and terminal consequences differ. |
| Domain models and DB rows | `core/model` types and `infra/db/*Row` | 2 — boundary translation | Yes | Domain capability and DB adapter respectively | Keep separate. Mapping duplication is the cost of keeping numeric keys/GORM out of APIs. Test mappings and JSON, do not alias. |
| Local session and Portal conversation | `core/session` history and conversation/message rows | 4 with a shared interface | Yes | Their current owners | Keep the `agent.MessageHistory` seam. Local branch/recovery and concurrent team orchestration have different lifetimes. |
| Local Job and durable Task/TaskRun | `agentapp/job`, `service/task`/scheduler/worker | 4 — coincidental similarity | Yes | Their current owners | Do not unify lifecycle or IDs. One is process-scoped UX; one is durable, authorized background execution. |
| Task tool and Portal durable Task | `internal/tool.Task` delegation and `service/conversation` Task tools | 4 — coincidental name | Yes | Local Agent tool vs Tier 1 orchestration | Keep separate; document the scope when touching tool names because the security and persistence models differ. |
| Artifact and task-run output | `model.Artifact`/artifact service and `TaskRunArtifact`/run-output storage | 4 — related but deliberately separate | Yes | Artifact service and task-run execution | Do not auto-convert or merge stores. A run output is reproducibility state; an Artifact is an intentionally kept file. |
| Transport pagination and DB safety caps | `httputil.LimitOffset`, `infra/db` page clamps | 2 — boundary translation | Yes | Transport and DB adapter | Keep separate. Replace route-local `50, 200` literals with transport constants where found, without making DB limits a UX contract. |
| Portal API and UI types | `portal/src/lib/api/types.ts`, camel-case view types and mappers | 2 — boundary translation | Yes | Wire contract and Portal presentation | Keep view types/mappers. Generate or validate only the snake-case wire layer once OpenAPI is complete. |
| Handwritten mocks and production contracts | `internal/mock`, `internal/testsupport` | 3 — intentional test duplication | Yes | Tests | Keep test-only; existing architecture tests correctly prevent production imports. Regenerate/update them with moved interfaces. |

## Concepts That Must Remain Separate

The following separations are architecture, not cleanup debt:

1. **Local Job versus durable Task/TaskRun.** They differ in ownership,
   persistence, authorization, scheduling, failure recovery, and user promise.
2. **Local Session versus Portal Conversation.** Both satisfy a message-history
   need, but local branch/rewind and team-concurrent orchestration change for
   different reasons.
3. **Configured model versus catalog record versus resolved target.** Credentials,
   persistence, redaction, defaults, and runtime resolution belong at distinct
   trust boundaries.
4. **Artifact versus run output versus mutable team home file.** They represent
   intentionally kept immutable content, reproducibility output, and an editable
   workspace respectively.
5. **Domain entity versus database row versus API/view DTO.** Row keys/GORM,
   domain invariants, wire compatibility, and presentation must not leak across
   boundaries.
6. **Task-run, workflow-run, and Issue status machines.** Similar words do not
   imply common transitions or terminal effects.
7. **User-session authentication versus run-token authentication.** A user and
   a worker possess different authority, claims, expiry, and reachable stores.
8. **Plugin package storage versus team Artifact storage.** Marketplace packages
   are deployment-scoped immutable release material, not team-owned files with
   Artifact retention and authorization.
9. **Protocol-specific gateway errors versus general service errors.** Managed
   inference must classify upstream/target/capability failures for its wire
   protocol; ordinary application refusals use `apierr`.
10. **Small protocol and security packages.** `llmwire`, `pluginwire`,
    `authtoken`, `plugininspect`, and the handler trust-boundary packages are
    small because their reasons to change are narrow. Merging them would reduce
    directory count while weakening ownership.

## Proposed Ownership Delta

This is the proposed changed area, not a replacement copy of the repository
layout. The complete current layout remains owned by
[repo-layout.md](../contribute/repo-layout.md).

```text
internal/core/
  identity/       user, password, login-code, refresh-session, system-grant contracts
  team/           team/member contracts and pure role/action policy
  conversation/   durable conversation and message contracts
  issue/          issue/comment contracts and status/hierarchy vocabulary
  agentdef/       stored Agent definition and revision contracts
  workflow/       workflow definition/run/step contracts and state vocabulary
  task/           Task/TaskRun contracts, provenance, canonical run transitions
  artifact/       Artifact and run-output metadata contracts
  plugin/         existing plugin contracts plus persisted catalog/activation types
  audit/          audit event, cursor, and store contracts
  quota/          quota tier/decision vocabulary
  llm/            existing LLM contract plus provider vocabulary and catalog/call models

internal/service/
  identity/       login/session and account-administration workflows
  systemadmin/    grant/revoke/list workflows with caller-authority policy
  llmcatalog/     model catalog validation and administration
  team/           member mutation using core/team policy
  issue/          CRUD plus start-assigned-agent/workflow orchestration
  artifact/       owns its ContentStore port
  plugin/         owns its PackageStore port

internal/server/
  apicontract/    structured route/schema metadata used for registration checks
  static/         checked generated OpenAPI output
```

`core/model` disappears only after the last domain moves. No type aliases are
introduced. Each domain extraction updates all of its callers, DB mappers,
tests, architecture scanners, and documents in the same batch. Cross-domain
references use the owning package directly: for example Task imports Plugin's
resolved pin type, while Workflow services import both Task and Workflow
contracts.

The dependency direction remains:

```text
cmd -> bootstrap
bootstrap -> interface | server | service | agentapp | infra
interface | server -> service | agentapp | core
service | agentapp | infra -> core
core capability packages -> smaller core capability packages only
```

Consumer-owned storage interfaces should use primitive or pure-core reference
types so an infrastructure adapter can satisfy them without importing a
service package. The adapter need not declare that it implements the interface;
Go's structural satisfaction keeps the dependency pointing inward.

## Options And Trade-offs

### Option A: Keep The Current Package Shape And Add Tests

This is the smallest diff and would catch some drift, especially OpenAPI and
provider values. It does not solve transport-owned business workflows or the
global `core/model` import surface. New capabilities would continue to choose
between copying a rule and adding more unrelated names to the global package.

### Option B: Selective Extraction, Then Domain-By-Domain Model Split

This is the recommendation. First establish canonical services and pure rules,
then move types to the owners that already exist. It gives every batch a
behavioural seam and avoids inventing packages before a capability has a real
workflow. The cost is a longer sequence of mechanical import updates and a
temporary period in which `core/model` coexists with extracted domains.

### Option C: Big-Bang Vertical Feature Rewrite

Moving handlers, services, types, rows, and frontend contracts into feature
trees at once would reach the final visual shape sooner. It would also combine
authorization, persistence, wire-contract, and runtime changes in one review,
make regressions hard to localize, and conflict with the no-alias architecture
rule. The audit evidence does not justify that risk.

## Incremental Implementation Plan

Every batch starts with characterization tests, changes one ownership decision,
runs the narrow package tests while iterating, then runs `./make test`, the
relevant `./make check` scope, and `git diff --check`. User-visible corrections
also get one changelog entry. No batch changes a DB column merely to move a Go
type or workflow.

| Batch | Packages | Change | Required tests and checks | Compatibility and rollback |
|---|---|---|---|---|
| 0. Confirm contracts | `docs/start/support.md`, architecture/design owners | Resolve the conflict between the Alpha/no-migration guidance and the current N-1 binary rollback promise before any schema-affecting work; approve this proposal's owners | Documentation link/check suite | Documentation-only decision; no runtime rollback |
| 1. Repair API contract | All handler `Register` methods, new `server/apicontract`, `server/static/openapi.json`, Portal API wire types | Inventory all 121 registrations; choose structured Go metadata; make OpenAPI complete; fix timestamp schemas; add exact route coverage; remove the stale Portal archive reference; generate or check snake-case Portal DTOs | Route inventory vs OpenAPI test, JSON schema/timestamp tests, handler tests, Portal typecheck/build, `./make check go`, `./make check portal` | Keep route paths and JSON unchanged; checked OpenAPI is revertible as one batch. If generation proves too opaque, retain static schemas but keep exhaustive coverage tests. |
| 2. Canonical LLM vocabulary | `core/llm`, `config`, `llmgateway`, `infra/llm`, bootstrap model code | Move provider values/list/known/credential rule to `core/llm`; retain config empty-default translation; centralize shared reasoning/cache validation where semantics match | Existing config/gateway/adapter tests plus table test proving all surfaces accept the same inventory | No YAML/DB/JSON value changes. Revert is source-only; do not leave aliases. |
| 3. Canonical catalog administration | New `service/llmcatalog`, model bootstrap and admin handlers | Move create/validate/enable/disable/audit workflows behind one service; keep credential-bearing create shell-only | Characterize every validation and audit branch; CLI golden/output tests; admin HTTP status/body tests; catalog store tests | No route, CLI flag, row, or secret-exposure change. Revert service and both adapters together. |
| 4. Canonical team policy | `core/team`, `service/team`, `server/access` | Define typed actions and one role/action decision; service enforces it; guard translates denial and records audit | Existing team authorization matrix, service member tests, artifact non-enumeration tests, fuzz/table tests for unknown role/action | Preserve current status codes and owner-only membership policy. Pure source move, one-commit rollback. |
| 5. System and account administration | New `service/systemadmin`, `service/identity`; bootstrap user/admin commands; admin handlers | Extract grant/revoke/list and account create/code/disable/session-revoke; model operator versus user actors explicitly; make last-admin override available only to operator adapter | Operator command tests, admin authorization matrix, audit content tests, last-admin tests, disabled-user/session revocation tests | Preserve shell recovery and HTTP refusal. No store/schema changes. Roll back the batch as a unit. |
| 6. Authentication workflows | `service/identity`, auth handlers, auth token adapter | Extract password/code verification, token-pair issue, refresh rotation/reuse handling, password setup, and logout; inject token issuer/clock/randomness | Move current password/refresh tests to service characterization and retain HTTP integration tests for exact status/body/cookies; race tests for rotation | Security-sensitive: keep endpoint/wire behaviour exact and deploy independently. Revert without data migration because token rows/formats do not change. |
| 7. Issue execution orchestration | `service/issue`, work issue/workflow handlers, conversation/task/workflow ports | Add start-assigned-agent and start-assigned-workflow commands; make one rule validate team/assignment; define compensation for partial creates | Service tests for wrong team/assignee/deleted Agent/unpublished Workflow and each partial failure; existing handler tests become adapter tests; workflow progression tests | Preserve routes and produced provenance values. Roll back source-only; no new persisted intermediate state unless separately approved. |
| 8. Port and utility cleanup | `service/artifact`, `service/plugin`, `infra/objectstore`, `infra/k8s`, `util` | Move content/package ports to consumers using pure refs/errors; move worker Job naming and tests to K8s | Artifact streaming/cleanup tests, plugin dedup/streaming tests, local/S3 adapter contract tests, exact Job-name tests, architecture import test | Method behaviour and object keys/job names must be byte-for-byte unchanged. Mechanical source rollback. |
| 9. Normalize quota refusal | `service/task`, `core/apierr`, work handlers | Return `KindQuotaExceeded` with detail from Task and remove duplicate handler branches; leave gateway protocol error local | Task/conversation HTTP 429 body tests, service errors test, gateway classification tests | Preserve status and reason text; small independent revert. |
| 10. Split `core/model` incrementally | New core capability packages; every direct caller; `infra/db`; mocks; architecture tests; architecture docs | Move one cohesive domain per PR, starting with `llm`, `team`, and `identity`, then Issue/Agent/Workflow/Task/Artifact/Plugin/Audit/Quota/Conversation; delete `core/model` last | Compile-time caller update, domain/store/DB mapping tests, JSON golden tests, entity-ID/timestamp architecture tests updated to scan all domain packages, `./make test`, `./make check ci` before final deletion | No aliases or dual definitions. Keep JSON tags, store behavior, table names, and rows unchanged. Each domain move is independently revertible before the next; final deletion occurs only at zero importers. |

## Risks And Guardrails

- **Authorization drift:** move decisions before adapters, retain the team and
  system authorization matrix tests, and test unknown roles/actions as deny.
- **Security timing drift:** characterize login failure timing and refresh-token
  reuse before moving code. Do not combine identity extraction with token format
  or crypto changes.
- **Wire drift:** JSON golden tests must protect timestamps, omission rules,
  error bodies, status codes, and route paths. A complete OpenAPI document is a
  result of the batch, not evidence by itself.
- **Persistence drift:** type moves do not authorize row or migration changes.
  Keep GORM inside `infra/db` and retain explicit domain-row conversion.
- **Import-cycle pressure:** if two proposed core packages form a cycle, move
  the smallest shared concept to its true owner or pass an ID/input contract;
  do not recreate `model/common/shared` as an escape hatch.
- **Temporary split ownership:** do not copy a type into its target package and
  leave both live. A batch moves the type and every caller together.
- **Frontend generation risk:** generated wire types must not replace Portal
  view models or mappers. Generation is useful only after the API contract is
  complete and checked.

## Open Questions And Evidence Needed

1. Is the current N-1 binary/database-schema promise in
   `docs/start/support.md` intentional despite the Alpha guidance that no
   migration path is owed? A maintainer decision is required before any future
   schema change; the proposed source-only batches do not depend on the answer.
2. Should OpenAPI become generated from structured Go route/schema metadata, or
   remain checked-in handwritten JSON with exhaustive coverage tests? Prototype
   one representative auth route, one team route, and one streaming route and
   compare reviewability before committing to generation.
3. Should model administration be a facet of `llmgateway` or a separate
   `llmcatalog` service? Choose based on whether catalog writes can be tested
   without provider-call dependencies and whether their audit/credential policy
   changes independently.
4. Should identity login/session workflows and operator account administration
   share one package or two services under one `identity` domain? Use the
   resulting constructor dependency sets as evidence; if neither workflow needs
   the other's ports, keep two service types in the same capability package.
5. Does every `core/model` domain deserve extraction? After batches 1–9, measure
   import fan-in and cross-domain references again. Keep a small package when it
   owns a stable boundary; do not merge it merely to hit a package-count target.
6. Does Issue execution need a transaction/operation record, or is explicit
   compensation enough? Fault-injection tests across conversation, task, and
   workflow creation should decide before adding persistence.

## Acceptance Criteria For Starting Implementation

- Maintainers accept or amend the canonical owners in the findings table.
- The separate-concepts list is treated as a guardrail, not a later cleanup
  queue.
- One issue or PR is created per implementation batch, with behaviour-level
  acceptance criteria and named rollback scope.
- Batch 1 chooses and documents the OpenAPI generation/checking strategy.
- Security-sensitive identity batches have characterization tests before moves.
- The final `core/model` sequence is ordered only after earlier services and
  pure rules establish real owners.

## Likely Destination If Accepted

Accepted ownership decisions belong in the matching architecture documents and
architecture tests; implementation priority belongs in the roadmap or focused
issues. This proposal should then be deleted. Durable rationale for intentionally
separate concepts belongs in the existing subsystem design records rather than
in a permanent repository-wide audit document.
