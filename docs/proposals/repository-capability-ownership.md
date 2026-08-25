# Repository Capability Ownership

> **Audience:** contributors · **Status:** accepted — plan of record
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

The ownership direction was accepted on 2026-08-25, with breaking changes
explicitly permitted: the project is Alpha, so nothing is owed a compatibility
window and no batch below needs a shim. Four amendments were made to the plan
this paper first proposed:

1. **The API contract work left this proposal.** Route and OpenAPI drift is a
   correctness bug, not an ownership problem. Measurement showed it to be
   documentation-only, so it ships now, on its own, and gates nothing.
2. **There is no approval gate before the first change.** The N-1 rollback
   promise in `docs/start/support.md` is an operating promise about upgrading a
   deployment, not a constraint on source ownership, and no change in this plan
   touches a column.
3. **A package is judged by its reason to change, not its size**, and an
   ownership move carries every caller in one commit. Both rules now live in
   [conventions.md](../contribute/conventions.md); this plan follows them
   rather than restating them.
4. **Mechanical moves come first.** They deliver this audit's core claim for
   the smallest reviewable cost, and they create the owners the later tracks
   need.

The audit does not treat fewer packages or fewer lines as a goal. The desired
outcome is that one business fact has one owner and every boundary translates
it explicitly. The licence to break things removes cost, not judgment: a break
is taken when the current shape is wrong, never as a side effect of moving a Go
type between packages.

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
  apierr/         existing error taxonomy, plus the cross-cutting ErrNotFound sentinel

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

## Execution Plan

The ownership direction above is accepted. This section replaces the batch
table this paper first proposed, and is the plan of record until the last track
lands and this document is deleted.

Work is grouped into four tracks. Track A is independent of ownership and ships
now. Track B is mechanical and can be reviewed in parallel. Track C is service
extraction and waits for the roadmap. Track D dissolves `core/model` last.

Every change moves the concept **and every caller in one commit**: no aliases,
no dual-live definitions, no compatibility shims. `TestNoInternalTypeAliases`
already refuses the usual shim, and the project is Alpha, so nothing is owed a
migration window. Incrementality is between pull requests, never inside one.
The rules this plan follows are in
[conventions.md](../contribute/conventions.md).

Breaking changes are permitted. That licence removes cost, not judgment: a
break is taken when the current shape is wrong, never as a side effect of
moving a Go type between packages. No change below alters a database column,
and each one says explicitly whether it changes the wire.

### Track A — Contract Repair, Independent Of Ownership

These are correctness and documentation bugs. They are not ownership problems,
they gate nothing, and none of them waits for a decision.

| PR | Scope | Change | Verification | Wire |
|---|---|---|---|---|
| A1 | `internal/server/static/openapi.json`, new coverage test | Fix the five `created_at` schemas (`Agent`, `Task`, `Conversation`, `ConversationMessage`, `Artifact`) from `integer` to `string`/`date-time`; add the missing operations toward the 117 contract-bearing patterns; add a test asserting every registered method pattern appears in the document | New route-vs-OpenAPI coverage test, `./make check go`, `./make check docs` | **None.** Measured: every affected Go field is `time.Time` and `portal/src/lib/api/types.ts` already declares `string`. Only the document was wrong. |
| A2 | `AGENTS.md`, `internal/architecture/docs_test.go` | `routes.go` is named the source of truth for routes but registers 7 of 121 patterns; the rest are in the subpackage `Register` methods, and `TestAgentsMDRoutesExist` only scans `routes.go`. Correct the claim and widen the scan | `./make check docs` | None |
| A3 | `docs/start/support.md`, `docs/contribute/architecture/data-model.md` | Record how the N-1 binary rollback promise stands against the Alpha guidance that no migration path is owed. Answered: the forward-only half is structural and cannot lapse; the one-release rollback is a discipline that alpha may spend on a wrong stored shape, with a changelog entry as the cost. Recorded in both places, because the rule was stated twice | `./make check docs` | None |

A1 is the largest of the three and is worth splitting if the added operations
outgrow one review: the timestamp fix and the coverage test are one PR, filling
in the missing operations is another. Whether OpenAPI eventually becomes
generated from structured Go metadata stays open — the coverage test makes the
document honest either way, and is the prerequisite for deciding.

### Track B — Mechanical Ownership Moves

Pure source moves. Each removes one duplicated business fact, changes no
persisted shape and no wire, and reverts as a single commit. They have no
ordering dependency on each other and can be reviewed in parallel.

| PR | Scope | Change | Measured size | Verification |
|---|---|---|---|---|
| B1 | `core/llm`, `config`, `service/llmgateway`, `infra/llm`, `bootstrap`, `interface/cli` | Move the four protocol values and `Known`/`All`/`NeedsCredential` to `core/llm`; `config` keeps only its empty-value defaulting, which is a genuine boundary translation | 18 production files, 20 test files | Table test asserting every surface accepts one inventory; existing config, gateway, and adapter tests |
| B2 | `service/plugin`, `infra/objectstore`, `infra/k8s`, `util` | Plugin service owns its `PackageStore` port, key derivation included, so layout stays with the adapter; move `WorkerJobNameForTaskRun`/`…At`, their regexes, and their tests to the Kubernetes adapter | 2 symbols crossed for plugin; 1 function and 2 regexes for K8s | Plugin publish/download tests, adapter conformance assertion in the port's own package, exact Job-name tests, `TestInternalLayerImports` |
| B2b | `service/artifact`, `infra/objectstore`, and every `objectstore.ErrNotFound` caller | **Deferred, see below.** The artifact port cannot move without a home for `ArtifactRef` and the not-found sentinel | 23 non-infra callers of `ErrNotFound` | — |
| B3 | `service/task`, `core/apierr`, `server/handlers/work`, `server/httputil` | Replace `task.QuotaExceededError` with `apierr.KindQuotaExceeded` carrying the quota service's reason as the message; delete the two handler special cases and the now-unused `httputil.WriteQuotaExceeded` | 1 type, 2 call sites, 1 superseded helper | 429 status and body tests for task and conversation routes; gateway classification tests prove `llmgateway.QuotaError` stayed local |
| B4 | `core/team`, `service/team`, `server/access` | One typed role/action decision in `core/team`; `service/team.requireOwner`/`hasRole` and `access.isRoleAllowed` both call it; the guard keeps translating denial and recording audit | 8 actions, 2 decision sites | Team authorization matrix, member mutation tests, artifact non-enumeration tests, table test proving an unknown role or action denies |
| B5 | `core/team`, `server/access`, `service/team` | Settle what a membership row with no role means: member. `EffectiveRole` owns the reading; the guard's local copy goes | 1 helper, 3 call sites | Role matrix driven with an unset-role member through every team-scoped route; `core/team` table tests |

B4 turned up a latent disagreement inside `server/access` itself. `TeamAction`
read a membership row with no role as "not a member" and answered 403;
`Guard.teamRole` read the same row as plain membership. Nothing can write such a
row today — the team service defaults an unset role to member before storing one
— so the two never disagreed about a real row. B4 preserved both readings rather
than bundling an authorization change into a mechanical batch.

**Settled since, as B5.** A row with no role is a member: the row is what says
somebody belongs to the team, and member is the least the three roles can mean.
`core/team.EffectiveRole` owns that reading and all three call sites use it, so
the normalization has one owner exactly as the decision does.

Consolidating also exposed a coverage gap. `service/team` had no test proving an
admin may not change membership: "owner" was written into that package, so no
change to the matrix could reach it. Now that both enforcers read one rule, the
service states its own expectation, and each of the three enforcement points
fails independently when the rule is widened.

B3 keeps the response body byte-for-byte. The reason the quota service supplies
is already a whole sentence — `quota exceeded: run limit` — so it becomes the
error's message rather than a detail appended to one, which would have said it
twice.

B2's plugin half resolved the judgment call this plan flagged. `PluginPackageKey`
derives an object key, which is storage layout — but the key is also persisted
on the release record and handed back to `Open`, so the service must hold it.
Key derivation therefore became a port method: the adapter still owns the
layout, and the service gets the value it has to store without knowing it.

**B2b is deferred, and the audit under-scoped it.** The finding said the only
production service packages reaching `internal/infra` are artifact and plugin.
That is true of the service layer and false of the repository: `ErrNotFound` has
23 callers outside `infra/objectstore`, across `server/handlers`,
`service/artifact`, `agentapp`, and the mocks. It is already the repository-wide
vocabulary for "no such object", exactly as `model.ErrNotFound` is for "no such
row".

So moving the artifact port alone buys nothing: the consumer would still import
`infra/objectstore` for the sentinel and for `ArtifactRef`, and twenty other
files would keep importing it regardless. The real change is to give the
not-found sentinel a home both layers can import — the same question D0 answers
for `model.ErrNotFound`, and with the same answer. Do B2b as part of D0, with
`ArtifactRef` following its domain in D6; do not spend churn on the port in
isolation first.

### Track C — Service Extraction

These extract business workflows out of transports. They are correct, but their
payoff is testability rather than user-visible behavior, and they are the
largest changes in this plan. They are sequenced **after** steps 1–4 of the
[roadmap](../ROADMAP.md#suggested-order): deployment proof, operating
exercises, evaluation, and worker hardening are ahead of them.

| PR | Scope | Change | Guardrail |
|---|---|---|---|
| C1 | New `service/llmcatalog`; `bootstrap/model_admin.go`; admin model handlers | One catalog administration workflow: validate, create, enable, disable, audit. Credential-bearing create stays operator-only | Characterize every validation branch and audit record first. HTTP must never accept or echo a credential |
| C2 | New `service/systemadmin`; account administration in `service/identity`; bootstrap user/admin commands; admin handlers | One grant/revoke/list workflow and one account create/code/disable/session-revoke workflow. Operator authority and System Administrator authority are modeled explicitly, not as a boolean any HTTP caller can set | The last-admin refusal must stay on the HTTP path and the operator override must stay on the shell path |
| C3 | `service/identity`; auth handlers; auth token adapter | Extract password and code verification, token-pair issue, refresh rotation and reuse handling, password establishment, logout. Inject token issuer, clock, and randomness | Security-sensitive. Characterize login failure timing and refresh reuse **before** moving anything, and do not combine this with any token format or crypto change |
| C4 | `service/issue`; work issue and workflow handlers | Add start-assigned-agent and start-assigned-workflow commands so one rule validates team, issue, and assignee; the workflow service stops revalidating independently | Fault-injection across conversation, task, and workflow creation decides whether explicit compensation is enough or an operation record is needed. Do not add persistence before that evidence |

C1 answers whether catalog administration is its own service or a facet of
`llmgateway`: build it as `service/llmcatalog` and keep it if its tests run with
no provider-call dependency. C2 and C3 answer whether identity is one package
or two service types under one capability: if neither workflow needs the
other's ports, keep two types in one package.

### Track D — Dissolve `core/model`

Last, and only after Tracks B and C have created real owners. One target
package per pull request, ordered so that cheap moves prove the mechanics
before expensive ones. `core/model` is deleted when it reaches zero importers.

**`errors.go` does not move as a unit.** It has the largest fan-in in the
package, but it is eight sentinels belonging to four capabilities, so it
dissolves instead: `ErrEmailExists`, `ErrUserNotFound`, and `ErrUserDisabled`
travel with identity; `ErrRunInProgress`, `ErrInvalidRunTransition`,
`ErrRunCanceled`, and `ErrRunInterrupted` travel with task; only `ErrNotFound`
is genuinely cross-cutting. It moves first, on its own, to `core/apierr`, which
already owns what an error means to the whole application and already declares
`KindNotFound`. That is the one design decision in this track and it should be
settled in D0's review.

| PR | Target package | Sources | Production files referencing the moved symbols |
|---|---|---|---|
| D0 | `core/apierr` | `ErrNotFound` from `errors.go` | 41 (the sentinel alone; the rest of `errors.go` waits for D5 and D9) |
| D1 | `core/llm` (exists) | `llm_model.go`, `llm_call.go` | 10, 10 |
| D2 | `core/quota` | `quota.go` | 4 |
| D3 | `core/conversation` | `conversation.go` | 13 |
| D4 | `core/workflow` | `workflow.go` | 13 |
| D5 | `core/agentdef` | `agent_definition.go` | 15 |
| D6 | `core/artifact` | `artifact.go` | 15 |
| D7 | `core/issue` | `issue.go` | 15 |
| D8 | `core/team` (from B4) | `team.go`, `webhook_key.go` | 18, 5 |
| D9 | `core/identity` | `user.go`, `password.go`, `login_code.go`, `refresh_token.go`, `system_grant.go`, 3 sentinels | 18, 2, 7, 7, 9 |
| D10 | `core/audit` | `audit.go` | 25 |
| D11 | `core/plugin` (exists) | `plugin.go`, `plugin_activation.go` | 28, 18 |
| D12 | `core/task` | `task.go`, `task_result_delivery.go`, 4 sentinels | 38, 4 |
| D13 | open | `schema.go` | 4 |

The counts are files referencing any exported symbol defined in that source
file, measured statically. They overlap — one file can appear in several rows —
and the distinct total is 140 production files in 38 directories.

`schema.go` has no obvious owner: `SchemaMigration` and `SchemaStore` describe
the database's own applied-migration state rather than a business capability.
Decide in D13 whether it belongs beside the migration machinery or in a named
capability package; do not park it in whatever remains of `core/model`.

Each of D1–D12 keeps JSON tags, store method signatures, table names, and row
structs unchanged. `entity_identity_test.go` and `timestamp_test.go` currently
scan `core/model` and must be widened to scan every capability package as it
appears — that widening is part of each PR, not a follow-up.

**Track D is not obligatory in full.** After Tracks B and C, re-measure. A
source file stays where it is unless a real owner exists for it and its current
placement is causing a concrete problem — an import cycle, a misuse, or a
capability that cannot be tested alone. If a handful of types are still best
served by one shared package at the end, that is an acceptable outcome. The
goal is that the owner of a concept is easy to find, not that a particular
directory disappears.

### Sequencing And Cost

Track A ships immediately and in parallel with everything. Track B is four
independent PRs and should follow at once; it delivers the core claim of this
audit — one business fact, one owner — for the smallest reviewable cost. Track
C waits for roadmap steps 1–4. Track D follows C.

The whole plan is roughly 4 + 4 + 4 + 14 pull requests. If only part of it is
funded, **B1, B2, and B3 are the minimum worth doing**: together they remove
three duplicated facts, touch no wire and no schema, and each reverts in one
commit.

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

## Open Questions

Answered questions have moved into the plan above. What is still genuinely open:

1. Should OpenAPI become generated from structured Go route/schema metadata, or
   remain handwritten JSON held honest by an exhaustive coverage test? A1's
   coverage test makes the document truthful either way and is the prerequisite
   for deciding. Prototype one auth route, one team route, and one streaming
   route and compare reviewability before committing to generation.
2. Should `apierr.ErrNotFound` carry `KindNotFound`? D0 settled where it lives
   and moved it as a plain sentinel, so nothing about which status a route
   answers changed. A Kind would make an unhandled store miss answer 404 rather
   than 500 — better on most routes, and a disclosure change on the ones that
   answer 404 precisely so an opaque ID cannot be probed.
3. Where does `schema.go` belong? `SchemaMigration` and `SchemaStore` describe
   the database's own state, not a business capability. Settle it in D13.
4. Is model administration its own service or a facet of `llmgateway`? C1 builds
   `service/llmcatalog` and keeps it only if its tests run with no provider-call
   dependency.
5. Is identity one package or two service types under one capability? C2 and C3
   decide from the resulting constructor dependency sets.
6. Does Issue execution need an operation record, or is explicit compensation
   enough? C4's fault-injection tests decide, before any persistence is added.

## Definition Of Done For This Document

This paper is the plan of record until its tracks land. It is deleted when:

- Accepted ownership decisions have moved into the matching architecture
  documents under [`docs/contribute/architecture/`](../contribute/architecture/README.md)
  and into tests under `internal/architecture`.
- The rationale for the deliberately separate concepts above has moved into the
  subsystem design records that own them, not into a permanent repository-wide
  audit.
- Remaining tracks are tracked as issues rather than as sections here.

Until then, each pull request names its track and PR identifier, states which
package owns each affected concept, what duplicated knowledge it removed, what
similar code it deliberately kept apart, and how behavior was verified.
