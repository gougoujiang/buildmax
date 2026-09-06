# Conversations & issues

Conversations are how you talk to BuildMax in the Portal; issues are how work gets
tracked and handed to an agent. This page walks through both.

## Start a conversation

**Home** is the front door. Type what you want done in the composer — for example,
*"Help me analyze last month's sales data"* — and send it (Enter to send,
Shift+Enter for a new line). A conversation can answer you directly, or, when the
work is bigger, start background work and show you the result when it's ready.

Recent conversations are listed on Home so you can pick one back up.

## Create an issue

An **issue** is the user-facing unit of work — the thing you actually want done.
Open **Issues** in the sidebar and choose **New Issue**. An issue has:

- **Title** — a short statement of the work.
- **Description** — the detail an agent needs to act on it.
- **Business Status** — `todo`, `in progress`, or `done`. You set this yourself;
  it is not changed automatically by a run.
- **Assignee** — who should do it (see below).

Issues can be nested: from an issue you can add **sub-issues** to break the work
down. Sub-issue status is tracked independently — closing a parent while
sub-issues are still open is allowed and never rolls their status up.

You can discuss an issue in its comments, where both people and agents leave notes.

## Assign work to an agent or workflow

The **Assignee** on an issue is what turns it into action. Open an issue and set
the assignee to one of:

- **Unassigned** — no one yet.
- **A person** (including *Me*) — a human owns it.
- **An agent** — a saved [agent](portal-agents-workflows.md) runs the issue in the
  background.
- **A workflow** — a published [workflow](portal-agents-workflows.md) runs its
  steps for the issue.

Assigning to an agent or workflow schedules a background run on a worker: it
materializes the space's files, runs the agent, writes any outputs, and reports
back — without tying up your browser.

## Follow a run on Issue Detail

Open an issue to see its detail view, where a run's progress and results appear:

- **Stop Run** — while a run is pending or running, you can stop it. A run nobody
  has picked up yet ends immediately; a run a worker is executing is asked to stop
  and finishes as *canceled*, usually within seconds. Either way it keeps whatever
  it had already produced.
- **Retry Run** — once a run is over, you can repeat it with the same
  instructions, which is how you recover from a worker that died or a model that
  timed out without retyping anything. A retry counts against your space's quota
  and leaves the original run's record intact. A run that is a workflow step is
  retried by re-running its workflow, not from here.
- **Outputs** — files and results a run produced show up on the issue, including
  the latest result and any saved [artifacts](portal-overview.md). Larger outputs
  are stored as artifacts you can open or download.

## Next

- Define the agents and plans you assign here: [Agents & workflows](portal-agents-workflows.md).
- Get oriented in the rest of the app: [Portal overview](portal-overview.md).
