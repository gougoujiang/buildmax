# Proposals

> **Audience:** contributors and early adopters · **Status:** current

Proposals are short papers for cross-cutting directions that are worth
discussing before they become roadmap work. They are not commitments, product
announcements, or user documentation.

## How This Directory Works

| Artifact | Purpose |
|---|---|
| [../ROADMAP.md](../ROADMAP.md) | Prioritized and accepted work |
| [../design/](../design/README.md) | Accepted rationale and active plans |
| This directory | Open questions and options before a direction is accepted |
| GitHub Discussions | Early community feedback and alternatives |
| GitHub Issues | Implementable work with an owner and acceptance criteria |

Every proposal opens with `Status: proposal — under discussion` and an
`Opened: YYYY-MM-DD` date, identifies related current documents, and separates
goals, non-goals, options, and open questions. It should be narrow enough that
readers can agree, disagree, or offer evidence without first
reverse-engineering the repository.

When a decision is made, update [../ROADMAP.md](../ROADMAP.md), the matching
design record, or a GitHub Issue. Then delete the proposal. Rejected and
superseded proposals are also deleted rather than archived; git history keeps
their context.

## Open Proposals

A paper stays open until its direction is accepted, which is not the same as
nothing being built. Where an early slice shipped ahead of the decision, the
last column says so, and the paper's own delivery phases hold the detail.

| Proposal | Question | Built so far |
|---|---|---|
| [Client sessions and API credentials](client-sessions-and-api-credentials.md) | Should interactive login issue any long-lived credential beyond a rotating refresh token, and how should native managed clients and unattended callers authenticate? | None of its stages. The rotating two-token session it proposes to harden is in `internal/infra/db/user_refresh_token.go` |
| [Enterprise identity and access](enterprise-identity-and-access.md) | How should a private deployment connect corporate identity to BuildMax teams and roles? | Nothing |
| [Durable Agent sessions](durable-agent-sessions.md) | Should authenticated local Agent sessions become revisioned Server resources for recovery, provenance, sharing, and cross-device continuation? | Nothing; no server route serves a session resource |
| [Assistant orchestration and the Workflow boundary](assistant-orchestration-and-workflow-boundary.md) | Does a bounded manager Agent create enough value over one strong Agent to become an Assistant product, and should Workflow narrow toward deterministic Automation? | Nothing; current Agents cannot admit durable child Team Agent Tasks |
| [Local Issue work bridge](local-issue-work-bridge.md) | How should connected CLI/TUI and Desktop handle Team Issues locally without becoming Portal clones or weakening direct local use? | Most of phase 1: `buildmax issue list`, `show`, and `status`, `buildmax --issue`, and the two Issue tools of [issue agent access](../design/issue-agent-access.md). The durable Issue-to-Session link is not built, and phases 2 and 3 are untouched |
| [Session tree, agent mailbox, and branched workspaces](session-tree-and-agent-mailbox.md) | Should interactive sessions fork isolated workspaces, return structured child reports, and resume their parent through a durable mailbox? | Nothing |

Thirteen papers have been retired. Nine were accepted into a design
record. *Durable Workflow graphs* asked whether Workflow should remain a linear
prompt sequencer, delegate control to one LLM, or become a durable graph over
Task/TaskRun. The accepted [Workflow runtime design](../design/workflow-runtime.md)
chooses deterministic state and policy around Agent execution, then adds typed
model decisions and bounded dynamic expansion on that substrate.

*System administration* asked how a private
deployment should authorize and audit System Administrators; the direction was
accepted and is now the [system administration
design](../design/system-administration.md), which decides the grant model,
the bootstrap and recovery path, the first API, and what stays out of it.
*Entity identity and relational keys* asked whether opaque public IDs should be
separated from compact relational keys before Beta; the direction was accepted
and is now the [entity identity design](../design/entity-identity.md), which
decides the identifier format, the table-by-table split, the store boundary,
and the Alpha cutover. *Local background work and monitors* asked whether TUI
and Desktop should share a process-scoped job manager for detached commands,
subagents, and event-driven monitors; the direction was accepted and is now the
[local background jobs design](../design/local-background-jobs.md), which
commits the staged delivery and its prerequisites.

*Portal interaction and execution model* asked how a full foreground Tier 1
Agent should coordinate a potentially specialized Tier 2 execution Agent plane
without owning durable state and delivery; the direction was accepted and is now
the [Portal execution design](../design/portal-execution-model.md), which
separates the two Agent tiers from the substrate that carries them and from the
projection derived off it, and records which of its phases shipped, which are
evidence-gated, and which are deferred behind a storage migration.

*Two-tier Agent architecture* reopened whether that hierarchy was the stable
product boundary. Its synthesis is now the [Agent execution and Task threads
design](../design/agent-execution-and-task-threads.md): an Agent can execute
directly through a Team-owned Task and TaskRun, Task is its durable interaction
thread, and Conversation is an independent foreground caller and optional
result surface rather than an execution parent.

*Plugin scope for background runs* asked whether a team's plugin set is decided
once for the team or per agent definition; the answer is both, and §5.3 of the
[team and worker plugin distribution design](../design/plugin-team-distribution.md)
now decides it. A team's activation is the allow-list and the pin, an agent
definition narrows it, and the two levels split by what an unwanted item costs:
inert content is inherited when an agent names none, executable content is
loaded only when an agent names it.

*Run-scoped Secret Broker and workload identity* asked how a Team should
authorize a stored or externally managed credential for a run without exposing
it to the whole worker; the direction was accepted and is now the [Team Secrets
design](../design/team-secrets.md). It answers the delivery half against the
paper's own first recommendation: a credential is delivered to the run, as an
environment variable or a rendered credential file, not into one named plugin
consumer, because an Agent invokes tools it selects at run time and per-tool
adaptation cannot reach them. A Secret is a Team-owned group of key/values in
one encrypted row, consumption is configured on the Agent, and the record states
plainly that an Agent can read what its run was granted — moving the safety onto
Team ownership, per-Agent consumption, short-lived credentials, and audit.

*Agent-managed worktrees and a mutable workspace root* asked whether one
interactive session should create a Git worktree and move its own workspace
root into it; the direction was accepted and is now the [workspace root and
worktrees design](../design/workspace-root-and-worktrees.md), which settles
where worktrees live, which of the root's dependents move with it, the
permission asymmetry between creating and removing one, and what happens to a
dirty tree, running jobs, and the cacheable prompt prefix.

Three were retired because the work they proposed shipped. *Private production
operations* asked for an operating contract for private deployment;
`deployment/production/` and the compatibility section of
[../start/support.md](../start/support.md) are that contract, and what it still
lacks is operational evidence, now recorded as open questions in the
[enterprise deployment design](../design/enterprise-deployment.md). *Audit and
data governance* asked for the smallest useful evidence model; the append-only
audit trail is it, and retention, export, and correlation remain open in the
[team governance design](../design/team-governance.md). *Trusted private
execution loop* asked whether one constrained, managed, auditable private team
task should be proven before broader expansion; managed inference in the worker
and the run-scoped credential shipped, the Beta gate was restated so it no
longer claims a bounded egress it does not have, and the reachability the paper
called its remaining work is the
`GET /api/teams/{team_id}/task-runs/{task_run_id}/llm-calls` route.

One was narrowed rather than answered. *Agent execution policy* asked who
chooses a worker's execution boundary; what a run holds, runs inside, is bounded
by, and records all became settled behaviour while it was open, leaving one
question that belongs to an existing plan — it is now §3.9 of the [trust harness
design](../design/trust-harness.md), with the egress half it blocks.

Git history holds all twelve papers.

## Starting A Proposal

Use a semantic filename. Start from the sections used by the existing papers:

1. Problem and current context.
2. Goals and non-goals.
3. Options and trade-offs.
4. Open questions and evidence needed for a decision.
5. Likely destination if accepted.

Add its row to [Open Proposals](#open-proposals), and keep the last column
true as slices land. A reader who consults only the index and finds "Built so
far" stale will conclude the wrong thing about the whole directory.

When a question needs independently authored Agent positions, use a semantic
directory with a `README.md` that owns the question and decision process, plus
one explicitly attributed `<agent-name>-view.md` per contributor. Index only
the directory's README here. Contributors do not edit one another's positions;
a later synthesis preserves disagreements and moves accepted rationale into the
normal design record.

Do not create a proposal for a focused bug, a documentation correction, or an
implementation task that already has acceptance criteria. Use an Issue instead.
