# Design Docs

This directory contains both:

- the **current planning path**
- older design references kept for historical context

## Current Entry Points

Start here if you want the current product and planning picture:

- [018-versioned-workspace-and-outcome-roadmap.md](./018-versioned-workspace-and-outcome-roadmap.md)
  Active roadmap after the Team / Issue / Workflow program.
- [019-phase-1-product-and-docs-reset.md](./019-phase-1-product-and-docs-reset.md)
  Current phase document for the active roadmap.
- [010-team-task-workflow-roadmap.md](./010-team-task-workflow-roadmap.md)
  Previous roadmap, effectively complete for its intended scope.
- [001-about-portal.md](./001-about-portal.md)
  Long-range product vision reference for the AI-native workspace direction.
- [007-two-tier-agent.md](./007-two-tier-agent.md)
  Conceptual reference for the Tier 1 / Tier 2 split. Read it together with the current `internal/core/conversation` package layout (`contracts.go`, `service.go`, `runtime_facade.go`, plus `channel/`, `runtime/`, and `tool/`) rather than assuming the older flat package sketch is still current.

## Current Foundations

These docs still describe important current implementation foundations:

- [002-env-config-maintainability.md](./002-env-config-maintainability.md)
- [003-store-workspacestorage-reorg.md](./003-store-workspacestorage-reorg.md)
- [008-backend-architecture-refactor.md](./008-backend-architecture-refactor.md)
- [011-phase-1-issue-uplift.md](./011-phase-1-issue-uplift.md)
- [012-phase-2-team-foundation.md](./012-phase-2-team-foundation.md)
- [013-phase-3-workflow-foundation.md](./013-phase-3-workflow-foundation.md)
- [014-phase-4-issue-flow-visualization.md](./014-phase-4-issue-flow-visualization.md)
- [015-portal-navigation-conversation-recent-refactor.md](./015-portal-navigation-conversation-recent-refactor.md)
- [016-phase-5-enterprise-governance-foundation.md](./016-phase-5-enterprise-governance-foundation.md)
- [017-team-scoped-files-upload-alignment.md](./017-team-scoped-files-upload-alignment.md)
- [022-portal-src-maintainability-refactor.md](./022-portal-src-maintainability-refactor.md)

## Archive

Outdated or superseded design docs live in [archive/](./archive/).

These are kept as references for:

- migration history
- older naming/models
- design rationale that no longer reflects the active product surface
