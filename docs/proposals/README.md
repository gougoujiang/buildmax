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
| [Enterprise identity and access](enterprise-identity-and-access.md) | How should a private deployment connect corporate identity to BuildMax teams and roles? |
| [Agent execution policy](agent-execution-policy.md) | What default policy and approval boundary make worker execution acceptable for a trusted enterprise deployment? |
| [Private production operations](private-production-operations.md) | What minimum operating contract turns the local Kubernetes path into a supported private deployment? |
| [Audit and data governance](audit-and-data-governance.md) | What is the smallest useful evidence and data-control model for shared agent work? |

## Starting A Proposal

Use a semantic filename. Start from the sections used by the existing papers:

1. Problem and current context.
2. Goals and non-goals.
3. Options and trade-offs.
4. Open questions and evidence needed for a decision.
5. Likely destination if accepted.

Do not create a proposal for a focused bug, a documentation correction, or an
implementation task that already has acceptance criteria. Use an Issue instead.
