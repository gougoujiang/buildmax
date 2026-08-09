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
calls and tool calls — under `<BUILDMAX_HOME>/traces/`.

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

## Next

- Run something locally: [quickstart.md](quickstart.md)
- Stand up the team deployment: [deploy/overview.md](../deploy/overview.md)
- How the code is arranged: [contribute/repo-layout.md](../contribute/repo-layout.md)
