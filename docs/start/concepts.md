# Concepts

> **Audience:** users and operators · **Status:** current

BuildMax is one Go agent runtime exposed through three surfaces. Understanding
which surface you are on, and which objects exist there, explains most of the
product.

## One Runtime, Three Surfaces

| Surface | Binary / directory | What it is for |
|---|---|---|
| **CLI / TUI** | `buildmax` | One user, one local directory, one terminal |
| **Desktop** | Wails app, built from source | The same local capability with a richer UI |
| **Portal** | `buildmax-server` + `portal/` | A team: shared work, background execution, results |

All three run the **same agent loop, the same tools, and the same MCP, skill,
and subagent behavior**. Differences between them come from environment and
permissions, not from separate agent implementations. You can use only the
local surfaces, deploy only the Portal, or use both.

## Two Operating Profiles, Two Model Transports

BuildMax has two product operating profiles:

- **Local Workbench:** CLI/TUI or Desktop runs an agent in a directory on one
  machine. No BuildMax Server is required.
- **Team Platform:** Server, Portal, and workers add shared work, background
  execution, managed models, results, and governance for a private deployment.

These profiles are separate from the way a model call travels. A local CLI or
Desktop may call a provider directly, or it may use models approved by a
BuildMax deployment:

| Agent execution | Model transport | Typical use |
|---|---|---|
| Local CLI/Desktop | `direct` | Personal endpoint, BYOK, or local inference |
| Local CLI/Desktop | `buildmax` | Local files and tools with enterprise-managed models |
| Worker | `buildmax` | Centrally authorized and accounted background execution |
| Worker | `direct` | A deployment that already distributes or injects provider access |

The transport is always explicit. BuildMax never falls back from a managed
model to a direct entry, because that would silently change where prompts,
source code, and tool results go. See
[Managed models](../reference/configuration.md#managed-models).

## The Agent Loop

Every run, on every surface, is the same cycle:

```text
prompt → LLM → tool calls → execute tools → results back to LLM → … → reply
```

The tools are ordinary local operations: `Read`, `Write`, `Edit`, `Bash`,
`Glob`, `Grep`, `WebFetch`, `TodoWrite`, plus skills, subagents, and any tools
exposed by connected MCP servers — see [guide/tools.md](../guide/tools.md).

Two mechanisms sit around this loop and are worth knowing about early:

- **Hooks** can observe or *block* events — a prompt, a tool call, a compaction.
  See [guide/hooks.md](../guide/hooks.md).
- **The sandbox** confines `Bash` subprocesses by filesystem path and network
  domain. See [guide/sandbox.md](../guide/sandbox.md).

Every run also writes a **durable trace** — a redacted JSONL record of the LLM
calls and tool calls — inside its session's folder, under
`<BUILDMAX_HOME>/sessions/<session_id>/traces/`.

## Local Objects

| Object | Meaning |
|---|---|
| **Workspace** | The directory the agent operates in. Defaults to the current directory; set with `--workspace`. |
| **Session** | A multi-turn conversation with its message history, saved under `<BUILDMAX_HOME>/sessions/`. Resume with `--continue` or `--resume <id>`. |
| **`AGENTS.md`** | Optional file at the workspace root, appended to the system prompt so the agent picks up project conventions. |

## Team Objects (Portal)

The Portal adds a shared model on top of the same runtime:

| Object | Meaning |
|---|---|
| **Team** | The ownership boundary. Everything below belongs to a team. Personal use is a single-member team called `My Space`. |
| **Conversation** | How a user talks to the system. This is the front door. |
| **Issue** | The user-facing unit of work — what someone actually wants done. |
| **Agent** | A saved agent definition a team can reuse. |
| **Workflow** | A reusable execution plan; currently a linear sequence of steps. Lifecycle: `draft`, `published`, `archived`. |
| **Task / TaskRun** | The low-level execution record. One task can have several runs. Users rarely see these directly. |

Team roles are `owner`, `admin`, and `member`. Uploaded files, issues,
workflows, conversations, and tasks are all team-scoped.

Deleting an agent removes it from the team but keeps the record behind it, so
runs and history that already name it stay readable, and a workflow run in
flight finishes. An agent a published workflow still uses cannot be deleted
until that workflow is changed or archived.

Agents and workflows keep a numbered history. Every edit records the definition
it produced, along with who wrote it, and an earlier version can be restored —
which records a new version rather than erasing the ones since. A workflow run
notes the workflow version it expanded and the agent version each step ran
under, so a past run stays readable after the definitions move on.

## Two Tiers

The Portal separates *talking* from *doing*:

```text
Tier 1  conversation  ──creates──▶  Tier 2  task / task_run
   ▲                                            │
   └──────────── reports back ──────────────────┘
```

- **Tier 1** is the conversation orchestrator. It is the single voice to the
  user: it decides whether a message can be answered directly or needs
  background work, and it turns results into replies.
- **Tier 2** is execution in the back. A worker process materializes the team's
  files into a run directory, runs the shared agent runtime there, writes
  artifacts, and reports status. It never messages the user directly.

This is why a long-running job does not block the conversation, and why the
result of a job always comes back through the conversation that started it.

## How Work Actually Executes

```text
Portal ──▶ server ──▶ task_run (PENDING)
                          │
                     scheduler claims it
                          │
                          ▼
                  buildmax-worker process
                     ├─ materialize team files → run home/
                     ├─ prepare AGENTS.md
                     ├─ run the shared agent runtime
                     └─ write artifacts/ → report back
```

The scheduler runs inside the server process. The worker talks to blob storage
directly rather than proxying files through the server, and reports status back
over a token-authenticated worker API.

A run in flight can be stopped: Issue Detail offers **Stop Run** while a task is
pending or running. A run nobody has picked up ends immediately. A run a worker
is executing is asked to stop, and finishes as `canceled` once that worker
stops — usually within seconds. Either way the run keeps whatever it had
produced by then, so stopping early costs you the rest of the work, not the part
already done.

A finished run can be repeated: Issue Detail offers **Retry Run** once a run is
over. The retry runs the same instructions the last run had, so recovering from
a worker that died or a model that timed out does not mean retyping them. It
counts against your team's quota like any other run, and it leaves the run it
repeats untouched — the record of what went wrong stays readable. A task that
is a workflow step cannot be retried this way: the workflow owns that step's
outcome, and re-running the workflow is what repeats it.

## Next

- Run something locally: [quickstart.md](quickstart.md)
- Stand up the team deployment: [deploy/overview.md](../deploy/overview.md)
- How the code is arranged: [contribute/repo-layout.md](../contribute/repo-layout.md)
