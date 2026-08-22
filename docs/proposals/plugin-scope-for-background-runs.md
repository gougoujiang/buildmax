# Plugin Scope For Background Runs

> **Audience:** contributors and operators · **Status:** proposal — under discussion
>
> **Opened:** 2026-08-22

Related: [team and worker plugin
distribution](../design/plugin-team-distribution.md), which this may amend
before it is built, the [plugin Marketplace](../design/plugin-marketplace.md)
it follows, [team governance](../design/team-governance.md) for the authority
it would use, and [../ROADMAP.md](../ROADMAP.md).

## The Question

A team's background runs will be able to use published plugins. **At what
granularity is that decided — once for the team, or per agent definition?**

The [team and worker plugin distribution
design](../design/plugin-team-distribution.md) answers "once for the team": a
team activates a pinned release and every background run that team dispatches
gets it. That answer has not been implemented, and it is disputed. This paper
records the disagreement so it can be settled on evidence rather than in a
commit message.

Nothing here is decided. When it is, this file is deleted and the design record
changes.

## Why It Matters

A plugin's four kinds of content differ enormously in what an unwanted one
costs:

| Content | Cost of an agent having one it did not need |
|---|---|
| Skill | Tokens in the `Skill` tool listing, and a chance the model invokes it |
| Subagent | The same, on the `Task` tool |
| MCP server | A process started, or a host reached, when that agent asks for it |
| Hook | **Fires on every tool call**, whether the agent asks for anything or not |

The last row is what makes this worth deciding before implementation rather
than after. A documentation agent inheriting a deployment's `pre_tool_use` hook
is not a nuisance; it is the least-privilege failure the whole activation model
exists to prevent.

## Goals

- One place that answers "what may this team's background runs use", because
  that is where an operator's unattended-use decision is checked.
- Least privilege for an individual agent, so an agent gets what it needs and
  not what its team happens to have.
- An exact pin per run, so nothing resolves "latest" at dispatch.
- A moving-a-version cost low enough that reading the new release's capability
  report stays a real step rather than a dialog people click through.

## Non-Goals

- Local installs. `buildmax plugin install` is shipped and unaffected.
- Tier 1 conversations, which assemble no `agentapp` and load no plugins.
- Secret delivery, which is deferred in the design record and stays deferred.

## Option A — Team Activation Only

What the design record says today. The team activates pinned releases; every
background run for that team materializes all of them.

**For.** One list, one place to move a version, one place to check operator
eligibility. `Task.AgentID` is a pointer, so a task can run without an agent —
and under this option such a run is still defined.

**Against.** One privilege level per team. Every agent gets every activated
hook. There is no way to say "this agent, not that one", which is the thing
the `tools:` field already does for subagents and which the guide calls the
useful part.

## Option B — Agent Definition Only

The agent definition names the releases it uses; there is no team-level list.

**For.** Capability sits where behavior already is. `AgentRevision` is
append-only — an edit appends, and restoring an older revision appends
again — so a pin in a revision answers exactly what an agent had at any
point, which a
team-level list cannot. Authority does not weaken: editing an agent is
`ActionManageAgents`, owner or admin, the same authority the design proposes for
activation.

**Against.** Three problems, all grounded rather than hypothetical:

- **A task can have no agent.** `Task.AgentID` is `*string`. Agentless runs
  would have no definition of what they get.
- **The pin lives in N places.** Moving one release to a new version means
  editing every agent that names it, each appending a revision, each supposedly
  preceded by reading the new capability report. That is the friction that
  erodes the review it is meant to force.
- **No team-level answer.** Operator eligibility is a property of a release;
  with no team list, "what may our background runs use" is a scan across
  agents.

## Option C — Team Activation, Agent Selection

Team activation is the allow-list and the pin. An agent definition narrows it.

The open sub-question is what an agent that names nothing gets. One shape worth
considering reuses the split the design already makes, one level down:

| Content | Team activation | Agent definition |
|---|---|---|
| Skills, subagents | Allow-list and pin | Inherited when unnamed |
| Hooks, MCP servers | Allow-list, pin, and operator eligibility | **Named explicitly or not loaded** |

Inert content inherits; executable content is opted into. An agentless run then
gets the inherited half and none of the executable half, which is a defined
answer rather than a gap.

**For.** Keeps every goal above. Matches the shape of `tools:` on a subagent:
the runtime has a set, the definition narrows it.

**Against.** Two levels to learn and two places to look when a plugin is not
where somebody expected. An agent naming an executable plugin its team has not
activated is a state needing its own message.

## What Would Settle It

- **How many agents does a team actually have?** Option B's N-places cost and
  Option C's second level are both priced by that number. Two agents makes B
  reasonable; twenty makes it a version-bump treadmill.
- **How often do agentless task runs happen?** If they are rare or
  deprecated, Option B's first objection weakens considerably.
- **Do teams want one shared toolkit or per-agent kits?** If the realistic
  pattern is one set of plugins the whole team uses, Option A is not coarse —
  it is correct, and B and C are ceremony.
- **Would an operator accept a team-level eligibility check?** If eligibility
  has to be re-checked per agent, C's second level stops being optional.

## Open Questions

1. Should this decision be deferred entirely past D1? D1 carries skills and
   subagents only, where the cost of an unwanted one is tokens rather than
   execution. The question could be answered when hooks and MCP servers arrive
   in D2, with real usage to price it.
2. If an agent may narrow, may it also *widen* — name a release its team has
   not activated — or is the team list a hard ceiling?
3. Does a workflow step that targets an agent inherit that agent's selection, or
   the team's?
4. Open question 2 of the design record asks whether an activation may name
   a subset of a release's content. That is the same question at a different
   level, and whichever option wins here should absorb or replace it.

## If This Is Accepted

Option A needs no change; the design record already says it.

Option B or C amends [team and worker plugin
distribution](../design/plugin-team-distribution.md) §4 (the activation
record), §5 (who may activate what), and §12 (the delivery phases), and
replaces that record's open question 2. Then this file is deleted.
