# Team And Worker Plugin Distribution

> **Audience:** contributors and operators · **Status:** partly implemented —
> D1 works end to end and Portal manages a team's activations; the agent
> definition's plugin field is not in Portal yet. D2 and D3 are not started
>
> Follows [plugin-marketplace.md](./plugin-marketplace.md), whose §12 Phase D
> asks for this record before anything here is built.

## Contents

- [Status](#status)
- [1. Decision](#1-decision)
- [2. What Already Crosses This Boundary](#2-what-already-crosses-this-boundary)
- [3. Why Activation Is A Team Decision](#3-why-activation-is-a-team-decision)
- [4. The Activation Record](#4-the-activation-record)
- [5. Who May Activate What](#5-who-may-activate-what)
- [6. Secrets Are Not In This Design](#6-secrets-are-not-in-this-design)
- [7. Materializing A Run](#7-materializing-a-run)
- [8. What A Run Records](#8-what-a-run-records)
- [9. What Bounds Executable Content](#9-what-bounds-executable-content)
- [10. Product Surfaces](#10-product-surfaces)
- [11. Implementation Ownership](#11-implementation-ownership)
- [12. Delivery Phases](#12-delivery-phases)
- [13. Alternatives Rejected](#13-alternatives-rejected)
- [14. Validation](#14-validation)
- [15. Open Questions](#15-open-questions)
- [Related Documents](#related-documents)

## Status

- roadmap_priority: `post-Beta, after the Marketplace`
- status: `partially_implemented` — D1 ships the activation record, the team
  routes and the CLI read path, the agent's selection, server-side resolution
  when a worker claims its run, materialization into the run, and Portal's team
  plugin section. What §10 still lacks is the agent's own plugin field: a
  selection can be set through the API but not yet in Portal's agent modal,
  whose form takes text fields only. D2, executable content, and D3, secrets,
  are not started
- follows: [plugin-marketplace.md](./plugin-marketplace.md)
- depends_on: nothing unbuilt. An earlier draft made the executable half wait
  on the worker sandbox surface being wired; §9 retires that, because the Bash
  sandbox never bounded the processes in question
- relates_to: [team-governance.md](./team-governance.md),
  [trust-harness.md](./trust-harness.md),
  [worker-run-token.md](./worker-run-token.md), and
  [sandbox-boundaries.md](./sandbox-boundaries.md)
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

Activation is a team decision about what may be used; an agent definition
decides what is used. An agent loads exactly the plugins it names, and nothing
reaches a run because a team activated it. See §5.3.

Not every team wants to curate. A team chooses between two modes: **curated**,
where an admin activates each plugin, and **open**, where the whole catalog may
be named and the activation is created automatically the first time an agent
names one. Open is the default. Both modes produce the same pinned record — the
difference is who fills the list, not whether there is one. See §4.1.

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

That promotion is the whole of it, and the limit is worth stating because the
rest of this record rests on it. The team's files land in `<run>/home`, while
workspace hooks are read from `<run>/.buildmax/hooks.yaml`, so a team's home
cannot contribute a hook to a run today even though it can contribute the
prompt. Instructions reaching a worker are not new. Executable content reaching
one is.

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

A team that does not want to make this decision per plugin still makes it: it
chooses open mode, once, and that choice is itself recorded and attributable.
What §4.1 refuses to do is let the absence of curation become the absence of a
record.

## 4. The Activation Record

One row per team and plugin:

| Field | Meaning |
|---|---|
| team | The team whose background runs get it |
| plugin | Catalog identity |
| version, digest | The exact release, pinned |
| enabled | Activated or suspended without losing the pin |
| origin | `curated` — an admin activated it — or `automatic` (§4.1) |
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

### 4.1 Curated And Open Teams

Curating a list is work, and a team that will not do it should not be forced to
choose between doing it badly and having no plugins. So curation is a team
setting with two values, and it is the *only* thing the two modes differ in:

| | Curated | Open |
|---|---|---|
| What an agent may name | What the team has activated | The whole catalog |
| Who creates the activation | An admin, deliberately | The system, when an agent first names the plugin |
| Pinned to | The release the admin chose | The latest eligible, unyanked release at that moment |
| `activated_by` | That admin | Whoever saved the agent revision that named it |
| Operator eligibility (§5.1) | Enforced | **Enforced, unchanged** |
| The pin, the digest, the audit event, the trace | Same | Same |

Open is the default, because the gate that matters across teams is eligibility,
not curation, and a deployment that wants curation everywhere can say so rather
than making every team say so.

**Open mode is auto-activation, not no-activation.** The row is created the
moment an agent names the plugin, not when the run starts, so by resolution
(§7) there is always a pinned row to resolve. A team's list therefore exists in
both modes: in curated mode it is a gate, in open mode it is a ledger. Switching
a team from open to curated later needs no reconstruction — the list is already
accurate and complete.

**An automatic pin does not move on its own.** Once created it behaves exactly
like a curated one: a newer release changes nothing until a person moves the
pin. This is not an oversight to be optimized away later. "A pin moves only when
somebody moves it" is the invariant §4 rests on and the reason §7 can stop
worrying about timing; making it conditional on a team setting would put an
"unless the team is open" exception under every argument in this record. What
open mode buys is that nobody had to *create* the pin; keeping it current is the
same deliberate act it is for everyone, and §10 makes it a visible one rather
than something a team has to go looking for.

**The authority does not change.** Naming a plugin in an agent already requires
`ActionManageAgents`, which is the same owner-or-admin authority §5 gives
activation. Open mode removes a step, not a permission: nobody who could not
already have activated the plugin can cause it to be activated.

## 5. Who May Activate What

| Content | Authority |
|---|---|
| Skills, subagents | Team owner or admin |
| Hooks, MCP servers | Team owner or admin, **and** the release marked eligible for unattended use by a System Administrator |

Team owner-or-admin matches `ActionManageAgents` and `ActionManageWorkflows` in
`internal/server/access`: managing shared automation assets is already this
authority, and an activation is that shape of decision. Setting the team's
curation mode (§4.1) is the same authority for the same reason, and in open mode
the automatic activation is caused by an agent edit that already required it.

### 5.1 Unattended Eligibility

Eligibility is a property of a **release**, not of a team, and an administrator
sets it on a release they already control. That is one flag rather than a
per-team approval queue, and it puts the decision where the knowledge is: an
operator can reason about what starting that process on their infrastructure
costs; a team admin usually cannot.

Curation mode does not reach this flag. An open-mode team may name any plugin in
the catalog; it may not name one contributing hooks or MCP servers that no
administrator has marked eligible, and auto-activation refuses such a release
exactly as a curated activation does. Open mode relaxes the team's own
housekeeping, never the operator's gate.

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

It does not grant permission. A skill, a subagent, and an MCP server's tools
reach the model as things that ask: tool permissions, hook gating, and
sensitive-path checks apply to them exactly as they apply to a team's own
configuration, and none of them widens what a run may do.

A hook command is the exception, and it is why eligibility exists. It runs
whether the agent asks for anything or not, and it is not itself subject to tool
permissions. It still cannot widen the agent's permissions — a hook may block a
tool call and has no way to approve one — but it does run code with the run's
own privileges. That is what an administrator accepts in §5.1, and §9 says what
does and does not bound it.

### 5.3 Granularity: Team Activation, Agent Selection

Absorbed from the retired *Plugin scope for background runs* proposal, which
disputed the granularity this record originally assumed. **Decided: two levels.**
A team's activation says what its background runs *may* use; an agent definition
says what a run *does* use. Neither level alone was right. A team-wide list gives
every agent one privilege level. An agent-only list has nowhere to check operator
eligibility and no team-level answer to "what may our background runs use" (§13).

| Content | Team activation | Agent definition |
|---|---|---|
| Skills, subagents | Allow-list and pin | Loaded only when the agent names it |
| Hooks, MCP servers | Allow-list, pin, and operator eligibility | Loaded only when the agent names it |

**Nothing is inherited.** An earlier draft of this record made inert content
inherit — an agent naming no plugin got every skill and subagent its team had
activated — on the argument that an unwanted skill costs only tokens while an
unwanted hook fires on every tool call. The asymmetry was rejected. An agent is
where a run's behavior is declared, and a capability that arrives because
somebody else edited a team setting is not declared anywhere a reader of the
agent can see. Cheap is not the same as invisible.

Two things follow, and both are improvements.

**Activation is purely permissive.** Activating a release changes no existing
agent's behavior; it changes what an agent may be edited to name. That makes
activation a safe action with no blast radius to warn about, and it keeps the
question "why did this run have X" answerable from the agent alone.

**A run with no agent loads no plugin.** `Task.AgentID` is `*string`, so this is
a live state rather than a hypothetical, and under a name-it-or-lose-it rule it
has only one coherent answer: nothing declared a need. A workflow step that
targets an agent uses that agent's selection; a step that targets none is the
agentless case.

**An agent names plugins, not releases.** The version and digest come from the
team's activation; an agent's selection is a set of catalog identities. This is
what keeps the second level from re-creating the defect that disqualifies an
agent-only list: moving a plugin to a new release stays one edit in one place,
so reading the new capability report stays a real step rather than a dialog
people click through. It is also why naming a plugin in N agents is an
acceptable cost while pinning a version in N agents is not.

**The team's ceiling is what an agent may name.** In curated mode that is the
team's activated list; in open mode it is the catalog, and naming creates the
activation (§4.1). Either way the agent names a plugin its team's record then
pins, and operator eligibility (§5.1) is checked against that record — which is
why no mode lets an agent reach a release the check has not seen.

**The activation set is the run's team's, not the agent's.** A worker already
refuses an agent whose team is not the task's team — `a.TeamID == task.TeamID`
in the worker run route, where the agent's instructions are resolved. Plugin
selection follows that rule rather than inventing a second one: the pins resolve
against `Task.TeamID`, and an agent that fails the team check takes the agentless
path above.

**A plugin an agent named but its team has not activated fails the run.** Not
loaded-with-a-warning, and not skipped: §7 already refuses to start a run whose
pinned package fails verification, because a background run's output is acted on
by somebody who was not watching it. An agent that names a plugin has declared it
needs one, and a run that quietly does less than its definition says is the same
wrong failure. The check runs at all three points for one reason — the picker
offers what the team's mode allows, the write either refuses an unactivated name
or activates it (§4.1), and the run refuses again — because an agent revision is
append-only, so a team suspending an activation, or switching to curated mode,
after the agent was saved is drift that cannot be edited away. The
operational consequence is intended and belongs in the error text: suspending an
activation stops the agents that name it, visibly, rather than silently changing
what they do.

The cost of two levels is that activating a plugin does not make it usable until
an agent names it. That is the price of the paragraph above, and §10 pays down
its other half — two places to look when a plugin is not where somebody expected
— by showing a team's activated set and which agents name each entry in one
place.

**Granularity stops here.** The plugin is the unit at both levels: neither an
activation nor an agent selection names part of a release. A third,
within-release level would have to be paid for by the same test that bought the
second one, and no case has been made for it (§15).

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
server                                        worker
  │                                             │
  │                    claims the run ◄─────────┤  GET /worker/task-runs/{id}
  ├─ read the run's team's activations          │
  ├─ intersect with the agent's named set (§5.3)│
  ├─ resolve each to (name, version, digest)    │
  ├─ record the pins on the run ────────────────►
  │                                             ├─ for each pin:
  │                                             │    GET the package with the run token
  │                                             │    verify the digest before extraction
  │                                             │    extract with the hardened reader
  │                                             │    inspect; refuse what would not load
  │                                             └─ place under <global>/plugins/<name>/
```

Every pin this resolves already exists, in both curation modes: an open-mode
activation is created when an agent names the plugin, not when a run starts
(§4.1), so resolution never has to invent one.

**The server resolves; the worker never does.** Resolution happens in the route
where a worker claims its run — the same place, and the same moment, that the
agent revision is resolved and recorded today. A worker that read its team's
activations would be a run token reading team state, which §13 rejects; what it
receives instead is a finished list.

An earlier draft resolved before dispatch instead, on the reasoning that a later
resolution could pick up a release published in the interval. It cannot: an
activation names an exact version and digest, so publishing a release changes
nothing until somebody moves a pin. The pin, not the timing, is what forbids
"latest at start". Resolving where the agent revision is already resolved avoids
a second, contradictory answer to "when is a run's definition fixed", and it
means suspending an activation still stops a run that was dispatched but has not
yet started — which is the behaviour §5.3 promises.

The resolved pins are recorded on the run, for the reason `TaskRun.AgentRevision`
is recorded: afterwards, nothing else can say which versions this run actually
had. The trace says so too (§8), but a trace is fail-open and lives in run-global
storage; the run's own row is the queryable fact, and a retry reads it.

The bytes come over a worker-scoped route authorized by the run token, serving
only the packages that run's pins name. The browse and download routes stay
user-scoped: a run token is not a user, and letting it read the catalog would
make it one.

Everything below the download reuses what already exists —
`internal/infra/pluginarchive` for extraction with its traversal, link,
duplicate-path, and size guards, and `internal/service/plugininspect` for the
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

The inventory also records, once per run, the agent and the revision whose
selection produced it — or that the run had no agent and therefore loaded no
plugin (§5.3). Without it, an inventory explains what a run had and cannot
explain what it did not have, which is the question a two-level model creates.

None of it carries package content, configuration values, or secrets, in keeping
with §10 of the Marketplace design.

## 9. What Bounds Executable Content

Not the Bash sandbox. An earlier draft of this record made the executable half
wait on the worker sandbox surface being wired, and that was wrong about what
the sandbox covers.

The sandbox wraps one thing: a child of the Bash tool. `internal/tool/bash.go`
applies it in `spawnArgs` and injects the filtering proxy through `childEnv`.
A hook command spawns from `internal/infra/hook/command.go`, and an MCP stdio
server from `internal/infra/mcp/transport.go`, each with a plain `exec.Command`
that carries neither. This is true on every surface, sandboxed or not. Wiring
`SandboxSurfaceWorker` would change the baseline a worker's *Bash tool* runs
under; it would not touch a hook process or an MCP server, which are precisely
the two things activation gates.

The same fact retires an egress claim this record used to make. The in-agent
proxy filters hostnames for a sandboxed bash child, and
[sandbox-boundaries.md](./sandbox-boundaries.md) says outright that it is not a
pod egress boundary. An activated MCP server's connections would not appear in
it, so "egress is reported once the worker surface is wired" was not something
this design could promise.

What actually bounds the executable half is two things, and neither is deferred:

- **The operator's reading.** §5.1 is the control: a named administrator accepts
  a specific release's capability report — the executables it starts, the hosts
  it reaches, the events it registers — before it may run where nobody is
  present.
- **The deployment's own isolation.** A worker runs inside whatever container,
  pod, and network policy the operator gave it. That boundary is the operator's
  and applies to an activated plugin's processes the same way it applies to
  everything else the run does.

Two consequences. Phase D2 is not blocked on sandbox work; the eligibility flag
is meaningful the day it ships. And confining hook and MCP processes is a real
gap — it exists on the local CLI today, not only on workers — which belongs to
[sandbox-boundaries.md](./sandbox-boundaries.md) rather than to this record.

## 10. Product Surfaces

Portal owns activation, because Portal is where a team's shared automation is
managed and where the audit trail is read.

A team's plugin section lists the catalog, marks what this team has activated
and at which version, and shows for each release what it contributes: the
same sanitized report an install shows locally. A release that is not eligible
for unattended use says so, and says that an administrator decides that, rather
than offering a button that would fail.

The section also carries the team's curation mode (§4.1) and says plainly what
each value means: in curated mode an admin activates before an agent may name;
in open mode naming activates. An entry activated automatically is labelled as
such, with the agent and the person whose edit caused it, so an open-mode team's
list reads as the history it is rather than as a list somebody curated.

Because a pin never moves on its own in either mode, this section is also where
staleness has to be visible: an activation with a newer release available says
so, next to what that release changes, with the update as a deliberate action.
A team that chose not to curate did not choose to run last quarter's plugin
forever — it chose not to be asked before first use. Leaving it to notice on its
own is how open mode would rot.

Because selection lives on the agent (§5.3), that section also answers the
question two levels create: for each activated release it says which of the
team's agents name it, and — because nothing is inherited — an activation no
agent names is shown as activated and unused rather than silently in force. An
agent's own page lists what it names and what its team activated that it does
not. Two levels are affordable when both are visible from one place; they are
not when finding out means reading every agent definition.

An agent's plugin field offers what its team's mode allows — the activated list
when curated, the catalog when open — and says which it is showing, because
"this name will activate a plugin for the whole team" is not something to learn
afterwards. It offers no version in either mode: the version is the team's
(§5.3), and a selector that implied otherwise would put the same pin in as many
places as there are agents.

Portal continues not to claim anything about a local machine. A team activation
is about that team's background runs; what somebody installed on their laptop is
still `buildmax plugin list` there.

The CLI gets a read path — what this team activated, at which version — so a
person debugging a run can see it without a browser. Changing an activation
stays in Portal, where the audit trail and the team's other shared automation
already are.

## 11. Implementation Ownership

- `internal/core/plugin` — the activation record and its store contract.
- `internal/infra/db` — one table, `plugin_activation`, singular, its public
  identifier from `util.NewPublicID` (prefixes are retired; see
  [entity-identity.md](./entity-identity.md) §4.4).
- `internal/service/plugin` — activation lifecycle, the eligibility flag, and
  auto-activation for open-mode teams, beside publication.
- `internal/core/team` and `internal/service/team` — the team's curation mode,
  defaulting to open.
- `internal/core/agentdef` and `internal/service/agent` — the agent definition's
  plugin selection, one JSON array of catalog names on `AgentRevision` so it
  versions with the rest of the definition and an old revision still answers
  what that agent named.
- `internal/core/task` and `internal/infra/db` — the resolved pins on
  `TaskRun`, beside `AgentRevision` and written at the same moment (§7).
- `internal/server/handlers/team` — team-scoped activation routes, under the
  authority §5 names.
- `internal/server/handlers/admin` — unattended eligibility on a release.
- `internal/server/handlers/worker` — resolving and recording the run's pins
  where the agent revision is already resolved, returning them with the run, and
  the run-token-scoped package download that serves only what those pins name.
- `internal/agentapp/taskrun` — resolving pins into the run's plugins
  directory before the runtime is assembled.
- `internal/infra/pluginarchive` and `internal/service/plugininspect` — the
  extraction mechanism and sanitized application-level capability inspection.

`internal/agentapp` needs no change beyond what already exists: it discovers
plugins from `BUILDMAX_HOME`, and a worker's is populated before assembly rather
than after.

## 12. Delivery Phases

### Phase D1 — Team Activation Of Instructions

- the activation record, its store, and the team-scoped routes;
- the team's curation mode, defaulting to open, and auto-activation on first
  naming (§4.1);
- pins resolved server-side when the worker claims its run, recorded on the
  run, and materialized into it (§7);
- skills and subagents only; a release contributing anything executable cannot
  be activated, and a pin cannot be moved onto one — a plugin whose next version
  adds a hook stops at the version before it;
- the agent definition's plugin selection, checked at the picker, at the write,
  and again at the run (§5.3); an agent that names nothing loads nothing, and so
  does a run with no agent;
- activation and provenance in the audit trail and the run trace;
- Portal's team plugin section.

Acceptance: a team admin activates a release contributing a skill, a background
run for that team loads it, and the run's trace names the activation, the
version, and the digest. An agent that names a subset loads that subset and the
trace says so. A release contributing a hook is refused with the reason. On an
open-mode team the same run works with no admin having activated anything, and
its activation reads as automatic, pinned, and attributed to the person who
saved the agent.

### Phase D2 — Executable Content

- unattended eligibility on a release, set by a System Administrator;
- activation of releases contributing hooks and MCP servers;
- moving a pin onto a release that newly contributes executable content is
  refused on the same terms as a first activation;
- hooks and MCP servers materialized only for the agents that name them, which
  is the same rule §5.3 applies to everything else — this phase widens what may
  be named, not how naming works.

Acceptance: a release contributing a hook cannot be activated until an
administrator marks it eligible; an activated hook fires in a background run for
the agent that named it, and a second agent on the same team — one that did not
name it — runs without it.

### Phase D3 — Secret Delivery, Answered Elsewhere

[team-secrets.md](team-secrets.md) is that follow-on record. It decides that a
Team owns the value, an Agent revision declares which Secrets it needs and how
they arrive, and delivery is run-level rather than into a named plugin consumer.
That last decision removes the release digest from the authorization path: a
value delivered to the run is visible to every process in it, so pinning which
plugin release receives it would protect nothing. The pin still governs what
code a run loads, which is §10's concern and unaffected.

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

It is rejected on two grounded objections. The pin would live in N places, so
moving one release to a new version means editing every agent that names it,
each supposedly preceded by reading the new capability report — the friction
that erodes the review it exists to force. And there would be no team-level
answer to "what may our background runs use", which is exactly where operator
eligibility is checked.

A third objection this record used to make no longer holds and is withdrawn
rather than quietly kept: that a run with no agent would have no definition of
what it gets. Under §5.3 it has one — it loads nothing. That answer is available
to an agent-only model too, so it is not an argument for the team level.

What is rejected is an agent definition *instead of* a team list. Selecting from
a team list in an agent definition is the other half of the decision and is
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

### Open Mode Resolving The Latest Release Per Run

Letting an open-mode team skip the pin entirely — resolve the newest eligible
release each time a run starts — is what "we do not want to maintain this"
sounds like it is asking for, and it would keep such a team permanently current
for free.

It is rejected because it makes the pin conditional. §4's guarantee that a
release published five minutes ago cannot change what a run is about to do would
hold for curated teams and not for open ones, and every argument built on it —
§5.1's re-reading, §7's freedom to resolve late, §13's rejection of a shared
directory — would need an "unless the team is open" clause. Two teams would get
different answers to "why did this run behave differently from the last one",
which is the question the whole record exists to answer. Currency is worth
buying with a visible prompt (§10), not with the invariant.

### Making Curation Mandatory For Every Team

Requiring curation everywhere is the safer-sounding default and was rejected as
the default rather than as a capability. Most teams' plugin use is skills and
subagents, which §2 shows are the kind of content a team can already put in
front of a worker through `AGENTS.md` with no ceremony at all. Requiring an
activation step for them and not for `AGENTS.md` would be ceremony where the
product already decided there is none, and the predictable result is teams that
do not use plugins rather than teams that curate. What must not be optional is
operator eligibility, and that is a separate control that open mode does not
touch (§5.1).

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
- a team member without owner or admin cannot activate anything, and cannot
  change the team's curation mode;
- an open-mode team's agent naming an unactivated plugin creates the activation,
  pinned to the latest eligible unyanked release, attributed to the person who
  saved that revision;
- a curated-mode team's agent naming an unactivated plugin is refused at the
  write instead;
- an automatic pin does not advance when a newer release is published, in either
  mode;
- open mode does not permit naming a release contributing hooks or MCP servers
  that no administrator has marked eligible;
- a release contributing a hook or an MCP server cannot be activated until an
  administrator marks it eligible, and moving a pin onto such a release is
  refused on the same terms as a first activation;
- eligibility does not carry from one release to the next;
- suspending an activation fails the runs of the agents that name that plugin,
  naming the plugin in the error, rather than running them without it;
- yanking or archiving leaves an activation working and reports its state;
- the audit trail records activation, deactivation, pin changes, and
  eligibility, naming actor, team, plugin, version, and digest prefix, and no
  configuration value;
- an agent that names no plugin loads no plugin, whatever its team activated;
- an agent that names a subset loads that subset and nothing else;
- activating a release changes no already-defined agent's behavior until an
  agent names it;
- an agent naming a plugin its team has not activated fails the run, and the
  error names the plugin;
- a run with no agent loads no plugin;
- an agent whose team is not the run's team is treated as no agent, matching the
  worker route's existing handling of its instructions;
- an agent edited after its run resolved its pins does not change what that run
  loads, and an agent edited between dispatch and that resolution does — the
  same rule the agent revision already follows (§7);
- a run's trace names the activation and who made it, and names the agent
  revision whose selection produced the set — or records that there was no
  agent;
- Tier 1 conversations still load no plugins;
- a team's declared environment variables that are unset are reported by the
  run rather than silently ignored.

## 15. Open Questions

1. Should a team be able to activate a release *older* than one it already runs,
   and if so does that need a different word than "update" in Portal?
2. ~~Should an activation be able to name a subset of a release's content — this
   skill but not that subagent — or is a plugin the unit a team accepts?~~
   **Decided: the plugin is the unit**, at both levels §5.3 defines. Selecting
   is the agent definition's job and it names plugins. A third, within-release level
   would have to be paid for by the same test that bought the second one, and
   for inert content the answer is tokens, which does not buy it.
3. Does an eligible release need re-reading when the deployment changes the
   boundary a worker runs inside — a different container or network policy —
   given the administrator accepted it under the old one?
4. Should Tier 1 conversations ever load plugins, and if so does the same
   activation record serve them, or is a conversation a different scope?
5. What does a team see when a release it pinned is yanked — a warning it can
   dismiss, or a state that blocks the next dispatch until somebody looks?
6. Should a deployment be able to force curated mode for every team, overriding
   §4.1's per-team choice? It is defensible where a deployment's teams are
   separate customers, and it is not built: eligibility already holds the line
   that crosses teams, and no deployment has asked. Adding it later is a setting,
   not a redesign.
7. Should a team be able to mark an activation *mandatory*, so that every agent
   loads it whether or not it names it? Deliberately not in this design: it is
   the inheritance §5.3 rejected, reintroduced as an explicit team choice rather
   than a default, and it is not worth its complexity until a deployment asks
   for it. The question is recorded so that answering it later does not read as
   reversing §5.3.

## Related Documents

- [plugin-marketplace.md](./plugin-marketplace.md) — the catalog it builds on
- [team-governance.md](./team-governance.md) — the authority model §5 reuses
- [sandbox-boundaries.md](./sandbox-boundaries.md) — the boundary §9 says is
  *not* this one, and where confining hook and MCP processes belongs
- [worker-run-token.md](./worker-run-token.md) — the credential §7 uses
- [guide/plugins.md](../guide/plugins.md) — what ships today
