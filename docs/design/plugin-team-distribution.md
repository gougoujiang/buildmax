# Team And Worker Plugin Distribution

> **Audience:** contributors and operators · **Status:** planned — design ready
> for review; implementation has not started
>
> Follows [plugin-marketplace.md](./plugin-marketplace.md), whose §12 Phase D
> asks for this record before anything here is built.

## Status

- roadmap_priority: `post-Beta, after the Marketplace`
- status: `ready_for_review` — §5.3 decides the granularity: team activation,
  agent selection
- follows: [plugin-marketplace.md](./plugin-marketplace.md)
- depends_on: [sandbox-boundaries.md](./sandbox-boundaries.md) §3.2, whose
  worker surface is written and not wired — §9 explains why half of this
  waits on it
- relates_to: [team-governance.md](./team-governance.md),
  [trust-harness.md](./trust-harness.md), and
  [worker-run-token.md](./worker-run-token.md)
- roadmap: [../ROADMAP.md](../ROADMAP.md)
- created_at: `2026-08-22`

## 1. Decision

A team may activate published plugin releases for its background runs. An
activation names an exact release — plugin, version, digest — and a worker
materializes exactly that, verified, into its run-scoped `BUILDMAX_HOME`.
Nothing resolves "latest" while a run is starting.

Activation is split by what the content can do rather than by who wrote it:

- **Instructions** — skills and subagents — are activated by a team admin
  alone. They cause tool use, and every tool call they cause still passes tool
  permissions, hook gating, and the sandbox.
- **Executable content** — hooks and MCP servers — additionally requires a
  System Administrator to have marked that release eligible for unattended use.
  It starts processes and opens connections on infrastructure the operator
  owns, and the operator is the only party who can weigh that across teams.

Activation is a team decision; an agent definition then narrows it. An agent
inherits the team's inert content when it names none, and loads executable
content only when it names it. See §5.3.

Secret delivery is deliberately not in this design. An activation names the
environment variables a release reads; supplying them is a separate record with
its own trust boundary.

## 2. What Already Crosses This Boundary

The question is narrower than it looks, because team content already reaches a
worker.

`internal/agentapp/taskrun` materializes the team's persistent `home` into the
run directory, and `WriteRunAgentsMd` promotes that home's `AGENTS.md` into the
workspace one. A team can already put arbitrary instructions in front of every
background run it dispatches, with no activation, no review, and no record
beyond the file itself.

So the boundary this design guards is not "team content reaches a worker". It
is **team content that executes reaches a worker**. A skill is prose that an
agent may read; a `command` hook is a program that runs whether the agent reads
anything or not. Requiring more ceremony for a skill than the product already
requires for `AGENTS.md` would be incoherent, and it is why §1 splits where it
does.

Two other facts bound the work:

- A worker's `BUILDMAX_HOME` is created fresh per run
  (`RuntimeTaskRunGlobalDir`), so its plugins directory is empty and no plugin
  loads today. That is why the Marketplace shipped without touching workers.
- Tier 1 conversations build their own tool registry and call `agent.RunLoop`
  directly rather than assembling `agentapp`, so they load no plugins either.
  Bringing plugins to Tier 1 is a separate decision and is out of scope here.

## 3. Why Activation Is A Team Decision

A deployment's catalog says what exists. It cannot say what a team's background
runs should have, because the person who publishes a plugin and the person
whose task runs are not the same person and do not share a purpose.

Activation is also the record that answers "why did this run have this
capability", which nothing else can answer once a release is published to
everyone. A worker run is unattended by definition: there is nobody to approve
a tool call and nobody to notice a hook. What replaces that presence is a
decision made earlier, by a named person, that a trail records.

The team is the right owner because Team is already the ownership and
authorization boundary for Portal resources, and because a run is dispatched in
a team's name against a team's files.

## 4. The Activation Record

One row per team and plugin:

| Field | Meaning |
|---|---|
| team | The team whose background runs get it |
| plugin | Catalog identity |
| version, digest | The exact release, pinned |
| enabled | Activated or suspended without losing the pin |
| activated_by, activated_at | Who decided, and when |
| updated_by, updated_at | Who last moved the pin |

The pin is the point. §10 of the Marketplace design forbids resolving "latest"
while a run starts, and a pin is what makes that true rather than aspirational:
a release published five minutes ago cannot change what a run already dispatched
is about to do. Moving to a newer release is a team action with its own audit
event, taken by a person looking at the new release's capability report.

A yanked release stays pinned and keeps working. Yank removes a release from
default selection; it does not reach into a team's activation, because doing so
would change what a team's runs do without anybody deciding to. What it does is
surface: the activation reads as pinned to a withdrawn release, with the reason,
so the team can move deliberately.

Archiving a plugin behaves the same way. Neither deletes.

## 5. Who May Activate What

| Content | Authority |
|---|---|
| Skills, subagents | Team owner or admin |
| Hooks, MCP servers | Team owner or admin, **and** the release marked eligible for unattended use by a System Administrator |

Team owner-or-admin matches `ActionManageAgents` and `ActionManageWorkflows` in
`internal/server/access`: managing shared automation assets is already this
authority, and an activation is that shape of decision.

### 5.1 Unattended Eligibility

Eligibility is a property of a **release**, not of a team, and an administrator
sets it on a release they already control. That is one flag rather than a
per-team approval queue, and it puts the decision where the knowledge is: an
operator can reason about what starting that process on their infrastructure
costs; a team admin usually cannot.

The word is "unattended" rather than "worker" because the property is *nobody is
present*, and three surfaces have that shape today: workers, print mode, and
Portal conversations. Naming the surface would make the flag wrong the first
time a fourth appeared.

Marking a release eligible means an administrator read its capability report —
the executables it starts, the hosts it reaches, the hook events it
registers — and accepted them running where nobody will be asked. It is a
statement about that release, so it does not transfer to the next version: a
new release starts ineligible, which is what keeps the reading from being a
formality.

### 5.2 What Eligibility Does Not Do

It does not grant permission. Tool permissions, hook gating, sensitive-path
checks, and the sandbox apply to an activated plugin's contributions exactly as
they apply to a team's own configuration. A plugin cannot widen what a run may
do; it can only bring more things that ask.

### 5.3 Granularity: Team Activation, Agent Selection

Absorbed from the retired *Plugin scope for background runs* proposal, which
disputed the granularity this record originally assumed. **Decided: two levels.**
A team's activation is the allow-list and the pin; an agent definition narrows
it. Neither level alone was right. A team-wide list gives every agent one
privilege level. An agent-only list has nowhere to check operator eligibility,
no team-level answer to "what may our background runs use", and no answer at all
for a run with no agent (§13).

The two levels split by what an unwanted item costs, reusing the split §1 makes
one level up:

| Content | Team activation | Agent definition |
|---|---|---|
| Skills, subagents | Allow-list and pin | Narrows when it names any; inherits the team's set when it names none |
| Hooks, MCP servers | Allow-list, pin, and operator eligibility | Named explicitly, or not loaded |

Inert content is inherited; executable content is opted into. The asymmetry is
the whole point. A skill an agent did not need costs tokens in a tool listing
and a chance the model invokes it. A hook fires on **every** tool call whether
the agent asked for anything or not, so a documentation agent inheriting a
deployment's `pre_tool_use` hook is not a nuisance — it is the least-privilege
failure activation exists to prevent.

**An agent names plugins, not releases.** The version and digest come from the
team's activation; an agent's selection is a set of catalog identities. This is
what stops the second level from re-creating the defect that disqualifies an
agent-only list: moving a plugin to a new release stays one edit in one place,
so reading the new capability report stays a real step rather than a dialog
people click through.

**A run with no agent is defined, not a gap.** `Task.AgentID` is `*string`, so
this is a live state rather than a hypothetical. Such a run gets the inherited
half — everything inert its team activated — and none of the executable half,
because nothing named it. A workflow step that targets an agent uses that
agent's selection; a step that targets none is the agentless case.

**The team's list is a ceiling.** An agent definition may narrow it and may not
widen it. Operator eligibility (§5.1) is checked once, against the team's
activation; an agent able to name a release its team has not activated would
route around that check.

**A plugin an agent named but its team has not activated fails the run.** Not
loaded-with-a-warning, and not skipped: §7 already refuses to start a run whose
pinned package fails verification, because a background run's output is acted on
by somebody who was not watching it. An agent that names a plugin has declared it
needs one, and a run that quietly does less than its definition says is the same
wrong failure. The operational consequence is intended and belongs in the error
text: suspending an activation stops the agents that name it, visibly, rather
than silently changing what they do.

The cost of two levels is two places to look when a plugin is not where somebody
expected. §10 pays that down by showing a team's activated set and which agents
narrow it in one place, and §8 makes a single run's trace answer "why did this
run not have X" from the same record that answers "why did it have Y".

**Granularity stops here.** The plugin is the unit at both levels: neither an
activation nor an agent selection names part of a release. A third,
within-release level would have to be paid for by the same test, and for inert
content the answer is tokens, which does not buy it (§15).

## 6. Secrets Are Not In This Design

An MCP server that authenticates needs a credential. Today a worker's
environment is the deployment's own, plus `BUILDMAX_HOME` and the run token —
there is no per-team secret anywhere in the product.

This design does not add one. What it adds is the honest failure: an activation
records the environment variable names the release reads, taken from the
inspection the catalog already stores, and a run reports which of them were
unset. A plugin whose server needs a token will start, fail to authenticate,
and say so — the same way it does locally.

Delivering secrets means deciding who may name a value a worker will hold, where
it is stored, how it rotates, and what an audit trail says about a value nobody
should read. That is a record of its own, and building it inside this one would
make both worse. Until it exists, the useful half of this design is skills,
subagents, and MCP servers that need no credential.

## 7. Materializing A Run

```text
scheduler                        worker
  │                                │
  ├─ read the team's activations   │
  ├─ narrow by the agent's selection (§5.3)
  ├─ resolve each to (name, version, digest, object key)
  ├─ dispatch with the pins ──────►│
  │                                ├─ for each pin:
  │                                │    GET the package with the run token
  │                                │    verify the digest before extraction
  │                                │    extract with the hardened reader
  │                                │    inspect; refuse what would not load
  │                                └─ place under <global>/plugins/<name>/
```

Pins are resolved **before dispatch** and travel with it. A worker that resolved
its own activations could pick up a release published in the interval, which is
exactly the "latest at start" §10 forbids.

The narrowing happens with the resolution, for the same reason. The agent
revision a run uses is fixed at dispatch, so editing an agent cannot change what
an already-dispatched run loads. What the scheduler sends is a resolved set,
not a team list plus a rule for reading it.

The bytes come over a worker-scoped route authorized by the run token, serving
only the packages that run's pins name. The browse and download routes stay
user-scoped: a run token is not a user, and letting it read the catalog would
make it one.

Everything below the download reuses what already exists —
`internal/core/plugin/archive` for extraction with its traversal, link,
duplicate-path, and size guards, and `internal/core/plugin/inspect` for the
check that decides whether a package would load. A second implementation for
workers would be a second set of rules about what an archive may contain.

A package that fails verification or inspection fails the run rather than
starting it without that plugin. A background run's output is acted on by
someone who was not watching it; silently doing less than the team activated is
the wrong failure. A plugin an agent named that resolves to no activation fails
the run the same way, for the same reason (§5.3).

## 8. What A Run Records

The trace already carries a plugin inventory per run. For a worker every entry
is a Marketplace release, so it names the catalog id, version, digest, and
source server. Two fields are added per entry, because a worker's inventory
answers a question a local one does not:

- the activation that caused it, and
- who activated that release.

That is what turns "this run had a hook that posted somewhere" into "this team
activated this release on this date, and that is the decision to revisit".

The inventory also records, once per run, whether an agent's selection narrowed
the team's set, naming the agent and the revision that was in force — or that
the run had no agent and took the inherited inert half (§5.3). Without it, an
inventory explains what a run had and cannot explain what it did not have, which
is the question a two-level model creates.

None of it carries package content, configuration values, or secrets, in keeping
with §10 of the Marketplace design.

## 9. Sandbox And Egress

The Bash sandbox defaults off on every surface, and `config.defaultSandbox`
defines a stricter `SandboxSurfaceWorker` baseline that no worker path passes:
`internal/agentapp/taskrun` leaves `AppConfig.SandboxSurface` empty, so a worker
resolves to the CLI baseline. The baseline is written and not wired.

That gap costs little today, because a worker runs a team's own files under a
non-interactive policy. It costs considerably more once a team can activate a
release that starts a process and opens a connection: the thing that would
bound what that process reaches is precisely the boundary that is not wired.

So this design makes the dependency explicit rather than discovering it later:

- **The instruction half does not need it.** Skills and subagents cause tool
  calls, which the non-interactive policy already refuses or allows on the same
  terms as a team's own configuration.
- **The executable half does.** Unattended eligibility must not be extended to
  hooks or MCP servers until the worker sandbox surface is wired, and the
  eligibility flag should be inert until it is.

Egress reporting follows from the same wiring: once a worker runs under the
worker surface, its egress proxy already reports what a run reached, and an
activated plugin's connections appear there with everything else.

## 10. Product Surfaces

Portal owns activation, because Portal is where a team's shared automation is
managed and where the audit trail is read.

A team's plugin section lists the catalog, marks what this team has activated
and at which version, and shows for each release what it contributes: the
same sanitized report an install shows locally. A release that is not eligible
for unattended use says so, and says that an administrator decides that, rather
than offering a button that would fail.

Because narrowing lives on the agent (§5.3), that section also answers the
question two levels create: for each activated release it says which of the
team's agents name it, and an agent's own page says which of the team's
activations it uses and which it leaves behind. Two levels are affordable when
both are visible from one place; they are not when finding out means reading
every agent definition.

Portal continues not to claim anything about a local machine. A team activation
is about that team's background runs; what somebody installed on their laptop is
still `buildmax plugin list` there.

The CLI gets a read path — what this team activated, at which version — so a
person debugging a run can see it without a browser. Changing an activation
stays in Portal, where the audit trail and the team's other shared automation
already are.

## 11. Implementation Ownership

- `internal/core/model` — the activation record and its store contract.
- `internal/infra/db` — one table, `plugin_activation`, singular, with a new
  prefixed public ID.
- `internal/service/plugin` — activation lifecycle and the eligibility flag,
  beside publication.
- `internal/core/model` and `internal/service/agent` — the agent definition's
  plugin selection, carried on `AgentRevision` so it versions with the rest of
  the definition and an old revision still answers what that agent named.
- `internal/server/handlers/team` — team-scoped activation routes, under the
  authority §5 names.
- `internal/server/handlers/admin` — unattended eligibility on a release.
- `internal/server/handlers/worker` — the run-token-scoped package download.
- `internal/agentapp/taskrun` — resolving pins into the run's plugins
  directory before the runtime is assembled.
- `internal/core/plugin/archive` and `.../inspect` — reused unchanged.

`internal/agentapp` needs no change beyond what already exists: it discovers
plugins from `BUILDMAX_HOME`, and a worker's is populated before assembly rather
than after.

## 12. Delivery Phases

### Phase D1 — Team Activation Of Instructions

- the activation record, its store, and the team-scoped routes;
- pins resolved before dispatch and materialized into the run;
- skills and subagents only; a release contributing anything executable cannot
  be activated;
- the agent definition's plugin selection, which at this phase only narrows:
  inert content inherits when an agent names nothing (§5.3);
- activation and provenance in the audit trail and the run trace;
- Portal's team plugin section.

Acceptance: a team admin activates a release contributing a skill, a background
run for that team loads it, and the run's trace names the activation, the
version, and the digest. An agent that names a subset loads that subset and the
trace says so. A release contributing a hook is refused with the reason.

### Phase D2 — Executable Content, Gated On The Worker Sandbox

- unattended eligibility on a release, set by a System Administrator;
- activation of releases contributing hooks and MCP servers;
- the worker sandbox surface actually wired, which §9 makes a precondition
  rather than a companion;
- the executable half of §5.3: a hook or MCP server loads only for an agent that
  names it, an agentless run gets none, and naming one the team has not
  activated fails the run.

Acceptance: eligibility cannot be granted while a worker resolves to the CLI
sandbox baseline, an activated hook runs under the worker boundary with its
egress reported, and a second agent on the same team — one that did not name
that hook — runs without it.

### Phase D3 — Secret Delivery, Deferred

Write a follow-on record. It must decide who may name a value a worker will
hold, where it is stored, how it rotates, and what the trail says about a value
nobody should read.

## 13. Alternatives Rejected

### A Team Installs Into A Shared Directory

Giving each team a persistent plugins directory it manages, materialized like
its home, would need no activation model. It also removes the pin: whatever is
in the directory when a run starts is what the run gets, which is the mutable
source §10 refuses for exactly this case. A run could then change behaviour
because somebody edited a directory between dispatch and start.

### Agent Definition Instead Of A Team List

Letting an agent definition name its own releases, with no team-level list, puts
capability where behavior already is, and `AgentRevision` is append-only, so a
pin in a revision answers exactly what an agent had at any point — something a
mutable team list cannot. Authority would not weaken either: editing an agent is
`ActionManageAgents`, the same authority §5 gives activation.

It is rejected on three grounded objections. A task can run with no agent
(`Task.AgentID` is `*string`), and such a run would have no definition of what it
gets. The pin would live in N places, so moving one release to a new version
means editing every agent that names it, each supposedly preceded by reading the
new capability report — the friction that erodes the review it exists to force.
And there would be no team-level answer to "what may our background runs use",
which is exactly where operator eligibility is checked.

What is rejected is an agent definition *instead of* a team list. Narrowing a
team list from an agent definition is the other half of the decision and is
adopted; see §5.3.

### Activation Without Operator Involvement

Letting a team admin activate anything published is simpler and is defensible
where teams are departments of one company. It is not defensible where a
deployment's teams are separate customers, because a team admin would then
cause arbitrary programs to run on shared infrastructure. Making eligibility a
property of a release rather than a per-team approval keeps the operator's
decision cheap enough that this is not a meaningful loss of speed.

### Eligibility Inherited By Later Versions

Carrying the flag forward from one release to the next would make activation
feel less like paperwork. It would also make the administrator's reading a
formality: the report they accepted describes bytes that no longer exist. A new
release starting ineligible is what keeps the flag meaning something.

### Yank Or Archive Deactivating A Team

Having a yank switch off every activation pinned to that release is tempting as
a kill switch. It changes what a team's runs do without anybody on that team
deciding, and it makes yank a much heavier action than "remove from default
selection" — which is what would then stop administrators from using it. The
kill switch that does exist is the deployment's: mark the release ineligible,
which stops the executable half immediately.

### Resolving Activations On The Worker

Letting a worker read its team's activations at start would remove a field from
the dispatch. It also reintroduces "latest at start" through the back door, and
it means a run token can read team state. Both are things the design already
decided against for reasons that have not changed.

## 14. Validation

Implementation is not complete until tests prove:

- an activation pins a version and a digest, and a release published afterwards
  does not change what a dispatched run loads;
- a worker materializes exactly the pinned release, verified against its digest
  before extraction, into its run-scoped `BUILDMAX_HOME`;
- a package that fails verification or inspection fails the run rather than
  starting it without that plugin;
- a run token can download only the packages its own run's pins name, and
  cannot read the catalog;
- a team member without owner or admin cannot activate anything;
- a release contributing a hook or an MCP server cannot be activated until an
  administrator marks it eligible, and eligibility cannot be granted while the
  worker sandbox surface is unwired;
- eligibility does not carry from one release to the next;
- yanking or archiving leaves an activation working and reports its state;
- the audit trail records activation, deactivation, pin changes, and
  eligibility, naming actor, team, plugin, version, and digest prefix, and no
  configuration value;
- an agent that names no plugin loads every skill and subagent its team
  activated, and no hook or MCP server;
- an agent that names a subset loads that subset and nothing else;
- an agent naming a plugin its team has not activated fails the run, and the
  error names the plugin;
- a run with no agent loads the inherited inert half and no executable content;
- an agent edited after dispatch does not change what the dispatched run loads;
- a run's trace names the activation and who made it, and names the agent
  revision that narrowed the team's set — or records that there was none;
- Tier 1 conversations still load no plugins;
- a team's declared environment variables that are unset are reported by the
  run rather than silently ignored.

## 15. Open Questions

1. Should a team be able to activate a release *older* than one it already runs,
   and if so does that need a different word than "update" in Portal?
2. ~~Should an activation be able to name a subset of a release's content — this
   skill but not that subagent — or is a plugin the unit a team accepts?~~
   **Decided: the plugin is the unit**, at both levels §5.3 defines. Narrowing is
   the agent definition's job and it names plugins. A third, within-release level
   would have to be paid for by the same test that bought the second one, and
   for inert content the answer is tokens, which does not buy it.
3. Does an eligible release need re-reading when a deployment's sandbox policy
   changes, given the administrator accepted it under the old boundary?
4. Should Tier 1 conversations ever load plugins, and if so does the same
   activation record serve them, or is a conversation a different scope?
5. What does a team see when a release it pinned is yanked — a warning it can
   dismiss, or a state that blocks the next dispatch until somebody looks?

## Related Documents

- [plugin-marketplace.md](./plugin-marketplace.md) — the catalog it builds on
- [team-governance.md](./team-governance.md) — the authority model §5 reuses
- [sandbox-boundaries.md](./sandbox-boundaries.md) — the boundary §9 needs
- [worker-run-token.md](./worker-run-token.md) — the credential §7 uses
- [guide/plugins.md](../guide/plugins.md) — what ships today
