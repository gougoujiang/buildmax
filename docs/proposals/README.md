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

Every proposal opens with `Status: proposal — under discussion`, identifies
related current documents, and separates goals, non-goals, options, and open
questions. It should be narrow enough that readers can agree, disagree, or
offer evidence without first reverse-engineering the repository.

When a decision is made, update [../ROADMAP.md](../ROADMAP.md), the matching
design record, or a GitHub Issue. Then delete the proposal. Rejected and
superseded proposals are also deleted rather than archived; git history keeps
their context.

## Open Proposals

| Proposal | Question |
|---|---|
| [Portal interaction and execution model](portal-interaction-execution-model.md) | How should a full foreground Tier 1 Agent coordinate a potentially specialized Tier 2 execution Agent plane without owning durable state and delivery? |
| [Enterprise identity and access](enterprise-identity-and-access.md) | How should a private deployment connect corporate identity to BuildMax teams and roles? |
| [Agent execution policy](agent-execution-policy.md) | Who chooses a worker's execution boundary, and what happens when the chosen one cannot be applied? |
| [Trusted private execution loop](trusted-private-execution-loop.md) | Should the next cross-cutting milestone prove one constrained, managed, auditable private team task before broader product expansion? |
| [Entity identity and relational keys](entity-identity-and-relational-keys.md) | Should public opaque IDs be separated from compact relational keys before Beta? |
| [Local background work and monitors](local-background-work-and-monitors.md) | Should TUI and Desktop share a process-scoped job manager for detached commands, subagents, and event-driven monitors? |
| [Durable Agent sessions](durable-agent-sessions.md) | Should authenticated local Agent sessions become revisioned Server resources for recovery, provenance, sharing, and cross-device continuation? |
| [Session tree, agent mailbox, and branched workspaces](session-tree-and-agent-mailbox.md) | Should interactive sessions fork isolated workspaces, return structured child reports, and resume their parent through a durable mailbox? |
| [Plugin scope for background runs](plugin-scope-for-background-runs.md) | Is a team's plugin set decided once for the team, or per agent definition? |

Three papers have been retired. *System administration* asked how a private
deployment should authorize and audit System Administrators; the direction was
accepted and is now the [system administration
design](../design/system-administration.md), which decides the grant model,
the bootstrap and recovery path, the first API, and what stays out of it.

The other two were retired because the work they proposed shipped. *Private
production operations* asked for an operating contract for private deployment;
`deployment/production/` and the compatibility section of
[../start/support.md](../start/support.md) are that contract, and what it still
lacks is operational evidence, now recorded as open questions in the
[enterprise deployment design](../design/enterprise-deployment.md). *Audit and
data governance* asked for the smallest useful evidence model; the append-only
audit trail is it, and retention, export, and correlation remain open in the
[team governance design](../design/team-governance.md). Git history holds all
three papers.

## Starting A Proposal

Use a semantic filename. Start from the sections used by the existing papers:

1. Problem and current context.
2. Goals and non-goals.
3. Options and trade-offs.
4. Open questions and evidence needed for a decision.
5. Likely destination if accepted.

Do not create a proposal for a focused bug, a documentation correction, or an
implementation task that already has acceptance criteria. Use an Issue instead.
