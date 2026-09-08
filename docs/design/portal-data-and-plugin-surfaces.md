# Portal Data and Plugin Surfaces

> **简体中文：** [阅读中文镜像](../zh-CN/design/Portal数据与插件界面.md)
> **Audience:** Portal, plugin, and artifact contributors · **Status:** planned

This record defines the names and boundaries for Workspace Files, Artifacts,
Marketplace, Space Plugins, and Agent plugin selection. It is an independently
deliverable R3 information-architecture improvement and does not change roadmap
priority or the underlying ownership model.

## Contents

- [Outcome](#outcome)
- [Evidence and constraints](#evidence-and-constraints)
- [Data surfaces](#data-surfaces)
- [Plugin surfaces](#plugin-surfaces)
- [Contextual connections](#contextual-connections)
- [Implementation slices](#implementation-slices)
- [Acceptance criteria](#acceptance-criteria)
- [Alternatives rejected](#alternatives-rejected)
- [Related records](#related-records)

## Outcome

A user can predict whether an object is mutable input or a published result, and
whether a plugin action affects the deployment catalog, a Space, an Agent, or a
local installation. The Portal uses one name and one primary location for each
concept.

## Evidence and constraints

- Workspace file browsing currently appears inside Chat and behind an “Explore”
  route whose breadcrumb calls it Files. The mixed labels make a durable Space
  resource look like a composer attachment feature.
- Artifacts are Space-owned, immutable published outputs with their own sharing
  and preview lifecycle. Merging them with mutable workspace files would erase
  meaningful trust and lifecycle differences.
- Marketplace is a deployment-wide release catalog, Space Plugins controls
  activation for background runs, and an Agent selects a subset of activated
  releases. These are different operations at different scopes.
- Account settings and the global Marketplace currently provide overlapping
  plugin catalog entry points, while local installation belongs to CLI/Desktop
  rather than the Portal server.
- Plugin resolution for workers remains server-owned and run-scoped as defined
  by [Space and worker plugin distribution](plugin-space-distribution.md).

## Data surfaces

Portal uses these terms consistently:

| Surface | Scope | Mutability | Purpose |
|---|---|---|---|
| Workspace Files | Space | Mutable during authorized work | Inputs and working state materialized for execution |
| Artifacts | Space, with optional public capability | Immutable after publication | Durable outputs intended for consumption or sharing |

Workspace Files is a first-class destination in the Space **Data** navigation
group. Chat may embed a file picker or recent-file view, but that is a contextual
entry into the same Files surface rather than its primary ownership location.
The route, page title, breadcrumb, and empty-state wording all use “Workspace
Files”; “Explore” is removed as a competing product name.

Artifacts remain a separate Space collection. Artifact pages state that the
object is a published output, show its media/type and origin when available, and
link to sharing and preview actions governed by the artifact design records.
Workspace Files do not inherit artifact immutability or public sharing merely
because both surfaces display files.

## Plugin surfaces

The plugin lifecycle is presented as a scoped sequence:

```text
Marketplace release -> Space activation -> Agent selection -> run resolution
```

| Surface | Scope | Primary action |
|---|---|---|
| Marketplace | Deployment/global | Discover a release and inspect availability |
| Space Plugins | Space | Activate, pin, update, or deactivate allowed releases |
| Agent Plugins | Agent revision | Select from releases activated for that Space |
| Local Plugins | Local BuildMax home | Install or manage through CLI/Desktop |

Marketplace remains a global top-level surface. It may provide a copyable local
installation command, but Portal does not imply that browsing or activating a
release installs it on the user’s machine.

Space Plugins remains within Space management and shows policy, activation
state, resolved version, and effect on future runs. Agent editing only offers
eligible Space activations and explains why an unavailable release must first be
activated. A run displays the resolved plugin set it actually received.

The duplicate Account > Plugins catalog is removed. Account settings continue
to hold account identity and invitations, not deployment or Space plugin state.

## Contextual connections

Primary locations stay singular, while contextual links preserve workflow:

- Chat and Issue inputs link to Workspace Files when a user needs to inspect or
  choose working data.
- Artifact detail links to its producing Issue, Task, and TaskRun when recorded.
- A TaskRun links to the artifacts it published and lists the plugin resolutions
  used for that attempt.
- Marketplace links to Space activation when the user has a selected Space and
  sufficient permission.
- Agent plugin selection links to Space Plugins for activation or policy errors.

These links do not copy management controls into every page. Authorization and
state feedback follow
[Portal state and permission feedback](portal-state-and-permission-feedback.md).

## Implementation slices

1. **Names and navigation.** Rename Explore to Workspace Files, add it to the
   Data group, keep Artifacts separate, and remove the duplicate Account plugin
   destination. No API change is required.
2. **Scope explanations.** Add concise page introductions and empty states for
   Files, Artifacts, Marketplace, Space Plugins, and Agent Plugins.
3. **Contextual links.** Connect existing origin, activation, and selection
   identifiers without duplicating controls.
4. **Resolved-run evidence.** Expose and present the authoritative plugin
   resolutions and artifact origins when the current API lacks them.

Each slice can ship independently. Slice four changes API or persistence only
where existing run evidence is insufficient; it reuses the authoritative plugin
and artifact models rather than introducing a Portal-only record.

## Acceptance criteria

- The navigation, route title, breadcrumb, and documentation use “Workspace
  Files” for the mutable Space file surface and “Artifacts” for published output.
- Users can reach Workspace Files without first opening Chat.
- Artifact and Workspace File pages explain their different mutability and
  sharing behavior.
- There is one global Marketplace entry and no Account-scoped catalog duplicate.
- Space and Agent plugin pages state their scope and only offer actions valid at
  that scope.
- Agent selection cannot imply activation, and Space activation cannot imply
  local installation.
- A run can show the resolved plugins and produced artifacts from authoritative
  execution data.
- Portal navigation and browser tests cover the scoped plugin path and both data
  surfaces.

## Alternatives rejected

- **Merge Workspace Files and Artifacts.** Similar rendering does not outweigh
  their different mutability, publication, sharing, and retention semantics.
- **Put all plugin controls in Marketplace.** Deployment discovery, Space policy,
  Agent selection, and local installation have different authorities.
- **Keep multiple catalog entry points for convenience.** Duplicated locations
  obscure scope and drift in labels and permissions.
- **Create a generic “Resources” entity.** The current requirements need clearer
  presentation of existing concepts, not another persistence lifecycle.

## Related records

- [Unified artifacts](unified-artifacts.md)
- [Artifact public sharing and preview](artifact-public-sharing-and-preview.md)
- [Plugin distribution and private marketplace](plugin-marketplace.md)
- [Space and worker plugin distribution](plugin-space-distribution.md)
- [Portal navigation and Space context](portal-navigation-and-space-context.md)

