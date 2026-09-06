# Agents & workflows

Agents and workflows are the reusable building blocks you assign work to. An agent
is a saved definition of *how* an agent should behave; a workflow is an ordered
plan that runs one or more agents in sequence. Both live in the current space and
keep a numbered history.

## Create an agent

Open **Agents** in the sidebar and create a new agent. An agent definition has:

- **Name** and **description** — how it shows up when you assign it.
- **Instructions** — the standing prompt that tells the agent how to work. This is
  the heart of the definition. (In a background run, the space's shared Agent
  instructions from **Space → Overview** are sent first, then these.)
- **Model** — which of the deployment's models it runs on.
- **Plugins** — optional [plugins](plugins.md) the agent may use.
- **Sandbox tiers** — the filesystem and network confinement for its `Bash` tool.
  See [Sandbox](sandbox.md).

Save the definition to make it assignable. You assign an agent to an issue from the
issue's **Assignee** field — see [Conversations & issues](portal-issues.md).

### Versions

Every time you save an agent, BuildMax records a new numbered version along with
who wrote it. You can **Restore** an earlier version, which records a *new* version
rather than erasing the ones since — so history stays intact. A workflow run notes
the exact agent version each step ran under, so past runs stay readable even after
the definition moves on.

Deleting an agent removes it from the space but keeps the record behind it, so runs
and history that already name it stay readable. An agent that a published workflow
still uses can't be deleted until that workflow is changed or archived.

## Create a workflow

Open **Workflows** in the sidebar and choose **New Workflow**. A workflow is a
reusable, step-by-step execution plan you can run manually or assign to an issue.
Build it from **steps**:

- Use **Add Step** to add a step.
- Each step targets an **agent** and carries a **prompt** describing what that step
  should do.
- Steps run in order; the plan is currently a linear sequence.

### Draft, publish, archive

A workflow has a status:

- **Draft** — still being edited.
- **Published** — ready to use. A workflow must be published before you can run it
  manually or assign it to an issue.
- **Archived** — retired from use.

Set the status from the workflow's detail view.

### Run a workflow

Once a workflow is published, use **Run Workflow** to run it. You'll be taken to
the run's detail view, where each step shows its own status as it executes. You can
also assign the workflow to an issue so it runs as that issue's work — see
[Conversations & issues](portal-issues.md).

Like agents, workflows keep a numbered history, and a run records the workflow
version it expanded so the record of a past run stays accurate.

## Plugins from the Marketplace

The **Marketplace** icon in the top bar lists the plugins this deployment
publishes — skills, subagents, MCP servers, and hooks. It's a browse surface:
installing happens where the agent actually runs, so the catalog hands you the
install command rather than a button. See [Plugins](plugins.md).

## Next

- Assign these to real work: [Conversations & issues](portal-issues.md).
- Tune what an agent can run: [Sandbox](sandbox.md) and [Tool permissions](tool-permissions.md).
