# Archived Design Documents

Design documents that no longer describe the current direction. They are kept
so the reasoning behind past decisions stays inspectable.

**These are history, not specification.** They are not edited to match new
behavior, and they frequently describe package names, ownership models, and APIs
that have since changed. For the current picture, start from:

- [../../ROADMAP.md](../../ROADMAP.md) — what we are building next
- [../architecture/](../architecture/) — how the system works today
- [../design/](../design/) — current design documents

A document is archived when it describes a completed migration, a superseded
product model, or a plan that has been replaced. Its number is retired with it
and never reused. Two numbers below (004 and 018) were allocated more than once
before that rule was established.

## Program Roadmaps

The multi-phase programs that produced the current team/issue/workflow model.

| # | Document |
|---|---|
| 010 | [Team / Issue / Workflow Roadmap](010-team-task-workflow-roadmap.md) |
| 018 | [Versioned Workspace And Outcome Roadmap](018-versioned-workspace-and-outcome-roadmap.md) |

## Phase Plans

Individual phases executed under the roadmaps above.

| # | Document |
|---|---|
| 011 | [Phase 1: Issue Uplift](011-phase-1-issue-uplift.md) |
| 012 | [Phase 2: Team Foundation](012-phase-2-team-foundation.md) |
| 013 | [Phase 3: Workflow Foundation](013-phase-3-workflow-foundation.md) |
| 014 | [Phase 4: Issue Flow Visualization](014-phase-4-issue-flow-visualization.md) |
| 016 | [Phase 5: Enterprise Governance Foundation](016-phase-5-enterprise-governance-foundation.md) |
| 019 | [Phase 1: Product And Docs Reset](019-phase-1-product-and-docs-reset.md) |
| 020 | [Phase 2: Outcome Surface](020-phase-2-outcome-surface.md) |
| 021 | [Phase 5: Team Quota Foundation](021-phase-5-team-quota-foundation.md) |

## Architecture And Refactors

Completed structural work. Useful for understanding why packages sit where they do.

| # | Document |
|---|---|
| 003 | [Reorganizing `internal/store` and `internal/workspacestorage`](003-store-workspacestorage-reorg.md) |
| 004 | [Server Refactor Opportunities](004-server-refactor-opportunities.md) |
| 007 | [Two-Tier Agent Architecture](007-two-tier-agent.md) |
| 008 | [Backend Architecture Refactor Blueprint](008-backend-architecture-refactor.md) |
| 009 | [User-Scoped Ownership Refactor](009-user-scoped-ownership-refactor.md) |
| 017 | [Team-Scoped Files / Upload Alignment](017-team-scoped-files-upload-alignment.md) |
| 018 | [Internal Package Refactor](018-internal-package-refactor.md) |

## API, Portal, And Runtime Details

| # | Document |
|---|---|
| 004 | [Portal API Contract](004-portal-api-contract.md) |
| 004 | [Webhook configuration](004-webhook.md) |
| 005 | [Portal Streaming: Design Options](005-portal-streaming.md) |
| 006 | [Metering auxiliary LLM usage](006-metering-auxiliary-llm-usage.md) |
| 015 | [Portal Navigation Conversation / Recent Refactor](015-portal-navigation-conversation-recent-refactor.md) |
| 022 | [Portal Source Maintainability Refactor](022-portal-src-maintainability-refactor.md) |
