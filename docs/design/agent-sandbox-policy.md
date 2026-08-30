# Agent-Scoped Sandbox Policy

## Contents

- [Status](#status)
- [1. Problem](#1-problem)
- [2. Decision](#2-decision)
- [3. Current Baseline](#3-current-baseline)
- [4. Design](#4-design)
- [5. Portal Surface](#5-portal-surface)
- [6. Out Of Scope](#6-out-of-scope)
- [7. Risks](#7-risks)
- [8. Open Questions](#8-open-questions)
- [9. Backend Plan](#9-backend-plan)
- [10. Frontend Plan](#10-frontend-plan)
- [11. Validation](#11-validation)
- [12. Recommended First PR](#12-recommended-first-pr)

## Status

- roadmap_priority: `P0.5` follow-on — closes the network/filesystem-granularity
  half of the gap [`current-state.md`](../current-state.md) calls P0 ("Worker
  Execution Is Not Contained By Default") and answers the granularity row of
  [trust-harness.md](./trust-harness.md) §3.9
- status: `backend implemented, Portal and team default open` — this document
  reopens and narrows trust-harness.md §3.9's "deployment-wide holds until
  evidence says otherwise" call; see §2 for what changes and what does not.
  §9's M1, M2, and M4 are shipped: the three tiers per axis, the maintained
  registry catalog, `agentdef.Agent`/`Revision` fields with
  create/update validation, claim-time resolution and pinning on `task.Run`,
  and `SandboxSurfaceWorker` selection in `taskrun/runtime.go` — closing
  [current-state.md](../current-state.md)'s worker-surface-selection P0 for an
  agent that declares nothing, independent of whether an agent ever declares a
  tier. §10's Portal selectors and §9 M3's team default tier are not started;
  an agent's tier is API-only until Portal ships. Neither the k8s pod/`bwrap`
  interaction nor the cluster `NetworkPolicy` question this document leaves to
  trust-harness.md §3.9 has been verified against a real cluster.
- follows: [sandbox-boundaries.md](./sandbox-boundaries.md),
  [trust-harness.md](./trust-harness.md) §3.2, §3.9,
  [plugin-team-distribution.md](./plugin-team-distribution.md) (the closest
  precedent for a per-agent capability declaration)
- roadmap: [../ROADMAP.md](../ROADMAP.md)
- created_at: `2026-08-30`

## 1. Problem

[`current-state.md`](../current-state.md) P0 states the worker task runtime
never selects `SandboxSurfaceWorker`: `agentapp/taskrun/runtime.go` builds its
`AppConfig` with an empty `SandboxSurface`, which `agentapp/app_builder.go`
resolves to the permissive CLI baseline. The direct fix — select
`SandboxSurfaceWorker` unconditionally — runs into a real usability cost, not
a hypothetical one. That baseline is default-deny on both axes this document
cares about: no network domain is pre-allowed and, per
[sandbox-boundaries.md](./sandbox-boundaries.md) §10, the worker's
`policy.yaml` narrows `allowed_domains` further and denies
`~/.aws`/`~/.ssh` reads. Flipping it on today would break every worker agent
that installs a dependency, calls a package registry, or fetches a web page,
and the only way to unbreak one is to hand-author a raw domain/path allow-list
in `policy.yaml` — a document only an operator edits, per §10's "locked, via
policy.yaml shipped with the worker container image."

Whoever defines an `agentdef.Agent` for background work is a different person
from whoever ships the worker container image, and asking the former to
either get the latter to edit a cluster-wide file, or understand
`sandbox.filesystem`/`sandbox.network` syntax well enough to get a
pull request merged, is not a cost most agent authors should pay for the
common case: an agent that installs dependencies and edits files in its own
workspace, nothing else.

## 2. Decision

trust-harness.md §3.9 considered "one deployment-wide profile in
`server.yaml` against layered operator/team/task profiles" and chose
deployment-wide, reasoning that "a per-team boundary should be paid for by an
operator who asks for it rather than assumed," and left as open exactly the
question this document answers: "Is a per-team boundary a real requirement?
... Until there is one, deployment-wide stands."

The evidence this document offers is the usability cost in §1: a
deployment-wide default-deny profile is cheap to build but expensive to live
under, because it forces every operator who wants workers to do anything past
edit-files-in-place into hand-authoring `policy.yaml` entries for every agent
that needs a registry or the open web. That cost falls on exactly the people
[current-state.md](../current-state.md)'s P1 account/team section says
BuildMax should not burden with deployment-wide files: a team owner defining
an agent, not a system operator.

This document proposes a narrower reopening than "layered per-team profiles"
in general:

- **Granularity moves from deployment-wide to agent-revision-scoped**, for the
  network and filesystem axes of `config.SandboxConfig` only. Every other axis
  of the worker sandbox — `enabled`, `fail_if_unavailable`,
  `allow_unsandboxed_commands`, process limits once they exist — stays
  deployment-wide, set once by the worker's `SandboxSurfaceWorker` baseline and
  `policy.yaml`, exactly as today.
- **The operator ceiling does not move.** `policy.yaml`'s
  `allow_managed_domains_only` / `allow_managed_read_paths_only` remain final
  and can lock any team or agent to the deployment-wide list, unchanged from
  today's `mergeSandbox` semantics (`internal/config/sandbox.go`). An operator
  who wants trust-harness.md's original deployment-wide behavior gets it by
  setting those two flags; nothing about this document forces a deployment to
  adopt agent-scoped policy.
- **What a workload declares is a coarse tier, not a domain list.** §4.1
  argues the granularity that actually removes the usability cost is much
  coarser than a per-agent domain/path editor, so this is not "layered
  profiles" in the general sense §3.9 declined — it is a fixed, small,
  versioned set of tiers a workload picks from.
- **The cluster egress question in §3.9's table is untouched.** Whether a
  production topology also needs a `NetworkPolicy` generated from the union of
  resolved `allowed_domains` across a team's active agents is still open and
  still belongs to §3.9, not this document. This document is scoped to the
  Go-side `SandboxConfig` the in-process proxy and OS backend already enforce.

If a future deployment does need per-team profiles for axes other than
network/filesystem, or needs domain-list granularity finer than the tiers
below, that is a separate reopening of §3.9 with its own evidence — this
document does not pre-decide it.

## 3. Current Baseline

- `config.SandboxConfig` (`internal/config/sandbox.go:26-66`) already
  separates `Filesystem` (`allow_write`/`deny_write`/`allow_read`/`deny_read`)
  from `Network` (`allowed_domains`/`denied_domains`/...).
- `ResolveSandboxForRun` (`internal/config/sandbox.go:198-237`) already merges
  five layers — policy > per-run override > env > settings > surface default —
  with array fields unioned (`mergeSandbox`, `internal/config/sandbox.go:266-`)
  and two "managed-only" flags that let policy.yaml suppress lower layers'
  allow-entries while still accepting their deny-entries. This document adds
  one more layer to that chain; it invents no new merge semantics.
- `agentdef.Agent` and `agentdef.Revision`
  (`internal/core/agentdef/agentdef.go:8-50`) already carry a per-agent
  capability declaration with exactly this document's shape:
  `Plugins []string` — "names the catalog plugins this agent loads for a
  background run. Nothing is inherited from the team's activations." The
  network/filesystem tier this document adds is the same kind of field:
  author-declared, versioned with the revision, not inherited.
- `task.Run` already pins a resolved-at-claim-time snapshot of exactly this
  shape twice: `AgentRevision *int` and `PluginPins []coreplugin.Pin`
  (`internal/core/task/task.go:142-158`), both resolved in
  `internal/server/handlers/worker/worker.go`'s `getTaskRun`
  (`internal/server/handlers/worker/worker.go:41-57`) — the route a worker
  polls to receive its run — and both recorded immediately
  (`recordAgentRevision`, `recordPluginPins`) so what a specific run actually
  received survives a later edit to the agent or the team's plugin
  activations. §4.4 reuses this exact chokepoint and pattern.
- The gap: nothing today lets an `agentdef.Agent` say anything about network
  or filesystem access, and nothing in `getTaskRun` resolves or pins a sandbox
  profile for the run it hands out.

## 4. Design

### 4.1 Two independent capability tiers, not personas

A named "agent persona" preset (`builder`, `researcher`, ...) was considered
and rejected: task shapes do not partition cleanly, an agent that mostly edits
files may occasionally need one web fetch, and a persona system either grows a
combinatorial number of names or forces an awkward-fit choice. Instead, two
independent, small, monotonically ordered tiers — an agent author answers two
questions, not one classification:

**Network tier**

| Tier | Behavior |
|---|---|
| `none` (default) | No domain pre-allowed. Matches today's `SandboxSurfaceWorker` baseline. |
| `registries` | `allowed_domains` includes a BuildMax-maintained default list of package-registry hosts (§4.6). |
| `open` | Outbound HTTPS is not domain-restricted. Filesystem tier is unaffected. |

**Filesystem tier**

| Tier | Behavior |
|---|---|
| `workspace` (default) | `allow_write` is the run's own workspace only — today's behavior, not a new option. |
| `workspace_plus_shared_read` | Adds `allow_read` for a deployment-configured shared cache path; `allow_write` unchanged. |
| `workspace_plus_external_write` | Adds one explicitly enumerated external write path (an artifact/output directory), never an unbounded grant. |

Each tier is a fixed translation into concrete `SandboxConfig.Network` /
`SandboxConfig.Filesystem` values — an agent author picks `registries` /
`workspace`, never types a domain or a path. Tiers are strictly ordered
supersets on their own axis so an operator can cap self-service at a tier
(§4.5) without reasoning about arbitrary combinations. A need past `open` or
`workspace_plus_external_write` is not a tier — it is a `policy.yaml`
exception, same as today.

### 4.2 Where the declaration lives

Add to `agentdef.Agent`, `agentdef.Revision`, and `agentdef.Definition`
(`internal/core/agentdef/agentdef.go`), beside `Plugins`:

```go
// SandboxNetworkTier and SandboxFilesystemTier declare this agent's worker
// sandbox needs. Nothing is inherited from the team's default: an agent that
// sets neither gets the strictest tier on both axes, the same way an agent
// that names no Plugins loads none.
SandboxNetworkTier    string `json:"sandbox_network_tier,omitempty"`
SandboxFilesystemTier string `json:"sandbox_filesystem_tier,omitempty"`
```

Empty string means the strictest tier (`none` / `workspace`), so an existing
agent with no opinion keeps today's behavior once `SandboxSurfaceWorker` is
finally selected — this document does not change what an agent that declares
nothing gets.

### 4.3 Resolution order

Insert the agent's declared tier, translated to a `SandboxConfig`, as one more
layer in `ResolveSandboxForRun`, between the team's default and the surface
baseline:

```text
policy.yaml (operator, final, can lock via managed-only)
  > per-run override
  > env
  > agent-declared tier   (new layer; allow-arrays only, unioned like any other)
  > team settings default (a team's own chosen default tier, itself expressed
                            as a SandboxConfig — most teams never set one and
                            inherit the surface baseline)
  > surface default (SandboxSurfaceWorker baseline: none / workspace)
```

No change to `mergeSandbox`'s union/managed-only semantics: the agent layer is
just another `SandboxConfig` value passed through the existing function. Deny
arrays from every layer still always apply; an operator's `deny_write`/
`denied_domains` in policy.yaml cannot be widened by an agent's tier
regardless of which tier it names.

### 4.4 Pinning at claim time

`getTaskRun` (`internal/server/handlers/worker/worker.go:41-57`) already
resolves `AgentRevision` and `PluginPins` at the moment a worker claims a run,
beside the agent it looked up. Add sandbox resolution to the same block:

```go
resolved := config.ResolveSandboxForRun(teamDefault, config.SandboxRunOverride{},
    policy, config.SandboxSurfaceWorker, agentTierConfig(runAgent))
h.recordSandboxProfile(r, run, resolved.Config)
```

`task.Run` gains a recorded snapshot beside `AgentRevision`/`PluginPins` —
either the two tier names or the resolved `SandboxConfig` (open question 3,
§8) — written once via a `RecordTaskRunSandboxProfile` call mirroring
`recordAgentRevision`, and included in `workerclient.GetTaskRunResponse`
beside `Plugins`. The worker (`internal/agentapp/taskrun/runtime.go`) sets
`AppConfig.SandboxSurface` and the resolved config from that response instead
of leaving it empty — closing [current-state.md](../current-state.md)'s P0 and
this document's new layer in one change, not two: there is no intermediate
state where the surface is wired but the per-agent layer is not, because the
worker has never had a `SandboxConfig` to apply until this record exists.

This buys the same audit property `AgentRevision`/`PluginPins` were built for:
after an incident, `task_run` answers what boundary a specific run had, even
if the agent's tier or the team's default changed since.

### 4.5 Team default and operator ceiling

A team may set its own default tier (stored beside other team-scoped
settings), which is what makes the common case free: a team that mostly builds
Node services sets its default network tier to `registries` once, and every
agent that declares nothing inherits it. `allow_managed_domains_only` /
`allow_managed_read_paths_only` in `policy.yaml` remain the operator's cap —
setting them turns `registries`/`open` from self-service into "whatever
policy.yaml's own `allowed_domains` already lists," without any change to how
those flags work today.

### 4.6 Maintained package-registry catalog

The `registries` tier's domain list (`registry.npmjs.org`, `pypi.org` +
`files.pythonhosted.org`, `crates.io` + `static.crates.io`,
`proxy.golang.org`, `rubygems.org`, and equivalents) is a BuildMax-maintained
default, not something a team authors from nothing — the same relationship
the plugin catalog has to a team's activation list. It ships as a Go literal
next to `defaultSandbox` in `internal/config/sandbox.go`, versions with
releases, and can be extended (never replaced) by a deployment's own
`policy.yaml` `allowed_domains` for an internal mirror or private registry.

## 5. Portal Surface

The agent editor gains two selectors beside the existing plugin picker —
"Network access" and "Filesystem access," each showing the three tiers above
with a one-line description, defaulting to the strictest. Team settings gains
one "default worker sandbox tier" control that sets what an agent inherits
when it declares nothing. Neither surface exposes a raw domain or path field;
that stays an operator-only `policy.yaml` edit, unchanged from today.

## 6. Out Of Scope

- **A raw domain/path editor for agent authors.** The tiers in §4.1 are the
  whole self-service surface. Anything past `open` / a shared external write
  path is a `policy.yaml` exception, same as today — not a UI this document
  adds.
- **Per-run override of an agent's tier.** Granularity stays at the agent
  revision, per §4.2 — asking for network access on a single ad hoc run would
  need a prompt on every dispatch, which is worse UX than what this document
  fixes. An agent that sometimes needs more should be revised, not overridden
  per run.
- **Cluster-level `NetworkPolicy` generation.** trust-harness.md §3.9's egress
  table row — whether a production topology needs a default-deny
  `NetworkPolicy` derived from resolved `allowed_domains` — is unaffected and
  still open.
- **Process resource limits, CLI/Desktop sandbox defaults, or any axis of
  `SandboxConfig` other than `Network` and `Filesystem`.** Those stay
  deployment-wide, set by the surface baseline and `policy.yaml` alone.
- **A fourth tier or arbitrary per-team tier definitions.** Three tiers per
  axis is the whole proposal; expand only from an observed deployment that
  cannot be served by `open` / `workspace_plus_external_write` plus a policy
  exception — the same restraint
  [team-governance.md](./team-governance.md) §11 states for custom roles.

## 7. Risks

- **The three tiers do not fit every workload.** Accepted, not solved: the
  long tail still goes through a `policy.yaml` exception, same as before this
  document. The goal is shrinking how often that path is needed, not
  eliminating it.
- **The registry catalog goes stale or omits a common host.** Mitigated by
  §4.5's cap being additive, not exclusive — a deployment extends it via
  `policy.yaml` without waiting on a BuildMax release, and an omission fails
  closed (a blocked pull, not a silent allow) rather than surprising anyone.
- **Reading this as license for a general per-agent policy platform.** It is
  not: §2 narrows the reopening to two axes and three fixed tiers each,
  explicitly declining domain/path editors and per-run overrides. A future
  request for either is a new proposal against fresh evidence, not an
  extension read into this one.

## 8. Open Questions

1. Does `getTaskRun`'s pinned snapshot record the two tier names, the fully
   resolved `SandboxConfig`, or both? The trace already carries a
   `sandbox_boundary` record (durable-run-trace.md); recording only tier names
   on `task_run` and leaving the expanded domain/path list to the trace would
   match how `AgentRevision` records a number, not the agent's full text.
2. Can a team set its default tier itself, or does raising the deployment-wide
   default above `none`/`workspace` require operator sign-off the way §4.5's
   ceiling does? Leaning toward team-owner self-service up to whatever
   `policy.yaml`'s managed-only flags still allow, consistent with owner
   authority over team settings elsewhere.
3. Should `open` ship with `allow_managed_domains_only` defaulted on in the
   worker surface baseline, so a deployment must opt in to *any* team
   self-serving unrestricted egress, versus defaulting available and letting
   an operator lock it down after the fact? This is the one place the tier
   design still has to pick a default posture rather than just exposing one.
4. Exact catalog contents for §4.6 and who maintains it as registries change —
   a BuildMax release note process, or a living file reviewed like any other
   default.

## 9. Backend Plan

### M1. Tier Types And Translation

- `config.SandboxNetworkTier` / `config.SandboxFilesystemTier` string enums
  and `TierToSandboxConfig(tier) SandboxConfig` in `internal/config/sandbox.go`,
  beside `defaultSandbox`.
- The `registries` domain list as a package-level literal, per §4.6.

### M2. Agent Definition Fields

- `SandboxNetworkTier` / `SandboxFilesystemTier` on `agentdef.Agent`,
  `agentdef.Revision`, `agentdef.Definition`
  (`internal/core/agentdef/agentdef.go`), following the `Plugins` pattern:
  versioned per revision, validated against the known tier enums on write.
- `agent_revision` row and its migration in `internal/infra/db`, per
  [data-model.md](../contribute/architecture/data-model.md)'s rules for a
  schema change.

### M3. Team Default Tier

- A team-scoped default tier, stored and read the way other team settings are,
  consumed by `ResolveSandboxForRun`'s new layer (§4.3).

### M4. Claim-Time Resolution And Pinning

- Sandbox resolution added to `getTaskRun`
  (`internal/server/handlers/worker/worker.go:41-57`), beside
  `recordAgentRevision`/`recordPluginPins`.
- `RecordTaskRunSandboxProfile` in the task-run store, and the corresponding
  field(s) on `task.Run` (open question 1).
- `workerclient.GetTaskRunResponse` carries the resolved profile.
- `internal/agentapp/taskrun/runtime.go` sets `AppConfig.SandboxSurface` and
  applies the resolved config instead of leaving `SandboxSurface` empty —
  this is also where [current-state.md](../current-state.md)'s P0 closes.

## 10. Frontend Plan

- Agent editor: two tier selectors beside the plugin picker, each a short
  dropdown with the descriptions from §4.1, defaulting to the strictest tier
  for a new agent.
- Team settings: one default-tier control (§4.5), visible to owner/admin.
- Task-run detail view: surface the resolved tiers the way plugin pins are
  already shown, so a reader can see what boundary a specific run had without
  reading the trace file.

## 11. Validation

```sh
./make test ./internal/config ./internal/core/agentdef ./internal/core/task \
  ./internal/server/handlers/worker ./internal/agentapp/taskrun
```

Manual scenarios:

1. An agent that declares neither tier runs on a worker with
   `SandboxSurfaceWorker` selected and gets exactly today's baseline
   (`none`/`workspace`) — no regression for an unmigrated agent.
2. An agent declaring `registries` installs a dependency from a catalog host
   successfully; a request to an uncataloged host is denied and recorded as a
   violation.
3. An agent declaring `open` reaches an arbitrary HTTPS host; filesystem
   access is unaffected by the network tier.
4. A team sets its default network tier to `registries`; an agent in that team
   declaring nothing inherits it; an agent in a different team still gets
   `none`.
5. Operator sets `allow_managed_domains_only: true` in `policy.yaml`; an agent
   declaring `open` is confined to whatever `policy.yaml`'s own
   `allowed_domains` lists, regardless of its declared tier.
6. `task_run` for each scenario above records the resolved profile, readable
   after the agent's tier or the team default subsequently changes.

## 12. Recommended First PR

1. M1 (tier types and translation) and M2 (agent definition fields), with the
   validation-on-write for the enum.
2. M4's claim-time resolution and pinning, and wiring
   `AppConfig.SandboxSurface` in `taskrun/runtime.go` — this alone closes
   [current-state.md](../current-state.md)'s P0 for an agent that declares
   nothing, before any UI exists.
3. Portal's two tier selectors and the task-run detail surfacing.
4. M3's team default tier, as a follow-up once the first agents have tiers to
   default from.
