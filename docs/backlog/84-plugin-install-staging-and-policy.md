---
id: plugin-install-staging-and-policy
title: Add PluginInstall staging and the autonomous-acquisition policy gate
roadmap: R5
source: docs/design/plugin-space-distribution.md#16-task-scoped-autonomous-acquisition
depends_on: [82-plugin-search-inspect-tools.md]
verification: ["./make test", "./make test mysql", "./make e2e cli"]
claim:
---

## Outcome

Lets an authorized running Agent acquire a needed capability at Task scope
without turning `BUILDMAX_HOME/plugins/` into durable state. The install stages a
pending Plugin environment revision and advances the Task's environment head for
the next Continue; it does not switch capability mid-run.

## Scope

- Add the `PluginInstall` tool and constant. It never mutates the running
  process; it stages the requested release into a pending Plugin environment
  revision (task 80) for the Task and returns that outcome to the Agent.
- Add the Space acquisition policy gate: a separate Space policy decides whether a
  running Agent may request autonomous acquisition at all. Curated mode may select
  only an activated release; open mode may cause the same pinned auto-activation an
  authorized Agent edit causes today, attributed to the TaskRun and initiating
  principal.
- Reuse existing gates unchanged: operator eligibility, executable-content gates,
  permission resolution, and Secret grants; installing a package never grants a
  Secret. A private or Agent-produced package is normalized, inspected,
  digest-verified, and stored once in the Package Store before the revision may
  reference it.
- Extend the environment entry (task 80) with the resolved permission revision
  and richer installer provenance (principal, source run) now that this task
  resolves permissions and knows the installing principal.
- On commit at the next TaskRun boundary, advance `task.plugin_environment_head_id`
  so a later Continue loads the addition.

## Out Of Scope

- The immediate capability handoff — quiesce, checkpoint commit, and successor
  TaskRun spawn (task 86). This task only stages and advances the head for the
  next Continue.
- Promotion to the Agent definition or Space activation, which stays a separate
  authorized, audited action.

## Acceptance Criteria

- `PluginInstall` never changes the current run's loaded plugin set, tool
  schemas, prompt layers, MCP servers, hooks, sandbox rules, or Secret
  requirements.
- A request refused by the Space policy fails with an LLM-meaningful message and
  stages nothing.
- Curated mode cannot install a non-activated release; open mode's auto-activation
  is attributed to the TaskRun and principal.
- A private/Agent-produced package is store-once and digest-verified before it is
  referenced; extraction debris and caches are never captured.
- A committed install advances the Task environment head so the next Continue
  loads it.

## Verification

- `go-unit`: staging produces a pending revision without touching the running
  runtime; policy gate for curated/open/refused; executable-content and permission
  gates still apply; store-once behavior.
- `mysql`: environment-head advance and revision persistence against real MySQL.

## Notes

The Plugin environment revision model (`plugin_environment` table,
`coreplugin.PluginEnvironment`/`EnvironmentEntry`, and the store's
`FinalizePluginEnvironment`) already landed; this task stages entries into it and
extends the entry with the resolved permission revision and installer principal.
It reuses the read/inspect surface (82). The mid-run capability switch is
intentionally deferred to task 86 because it requires the checkpoint-backed
orchestration designed in plugin-space-distribution.md §16.1.
