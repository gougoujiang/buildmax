# Tools

> **Audience:** contributors · **Status:** current
>
> User-facing tool guide: [guide/tools.md](../../guide/tools.md)

## Purpose

The `internal/tool` package provides the runtime tools the agent can invoke.
Each tool implements the `internal/core/llm.Tool` interface and is registered
through `internal/agentapp`. Tools are designed for LLM consumption — their
results are sent back to the model as tool-role messages.

## Key Types and Interfaces

| Name | Kind | Role |
|------|------|------|
| **llm.Tool** | interface | Contract: `Name()`, `Description()`, `Parameters()`, `Execute(ctx, args)` |
| **llm.AccessDeclarer** | interface | Optional: `Access(args) Access` — does this call change anything |
| **llm.ArgChecker** | interface | Optional: `CheckArgs(args) ToolAction` — argument-level risk |
| **llm.PolicyProvider** | interface | Optional: `DefaultAction() ToolAction` — override the derived default |
| **llm.GrantScoper** | interface | Optional: `GrantScope(args) string` — narrow what one session grant covers |
| **ReadFile** | struct | Reads files under a root directory |
| **WriteFile** | struct | Creates/overwrites files under a root directory |
| **EditFile** | struct | Performs exact string replacements in files |
| **WebFetch** | struct | Fetches URLs, converts HTML to markdown |
| **Bash** | struct | Runs shell commands in the workspace |
| **Glob** | struct | Lists files matching glob patterns |
| **Grep** | struct | Searches file contents by regex |
| **TodoWrite** | struct | Records the session task list |
| **NoteWrite** | struct | Records durable session notes |
| **SkillTool** | struct | Loads a discovered skill's instructions (`Skill`) |
| **TaskTool** | struct | Runs a subagent of a named type (`Task`) |
| MCP gateway | structs | `LoadMcpTools` and `CallMcpTool` |

## Tool Inventory

### ReadFile (`Read`)

- **Parameters**: `path` (required), `offset` (optional, 1-based line), `limit` (optional, default 1000)
- **Behavior**: Reads file content with line numbers (`LINE|CONTENT` format). Supports offset/limit for large files. Path must be under the configured root.
- **Error handling**: Returns clear errors for path-outside-root, file-not-found, etc.

### WriteFile (`Write`)

- **Parameters**: `path` (required), `content` (required)
- **Behavior**: Creates or overwrites a file. Creates parent directories as needed. Path must be under root.

### EditFile (`Edit`)

- **Parameters**: `path` (required), `old_string` (required), `new_string` (required), `replace_all` (optional bool)
- **Behavior**: Performs exact string replacement in a file. By default replaces the first unique match; `replace_all` replaces all occurrences. Fails if `old_string` is not found or is ambiguous (multiple matches without `replace_all`).

### WebFetch (`WebFetch`)

- **Parameters**: `url` (required)
- **Behavior**: Fetches a URL, converts HTML to markdown. Caches results (default 15 min TTL). Optionally uses LLM to process/summarize content.

### Bash (`Bash`)

- **Parameters**: `command` (required), `timeout` (optional, default 120s, max 600s)
- **Behavior**: Runs a shell command in the workspace root. Returns combined stdout+stderr. Output truncated at 30k characters.

### Glob (`Glob`)

- **Parameters**: `pattern` (required)
- **Behavior**: Lists files matching a glob pattern under the root. Returns paths sorted by modification time (newest first). Patterns not starting with `**/` are auto-prefixed for recursive search.

### Grep (`Grep`)

- **Parameters**: `pattern` (required), plus optional `path`, `glob`, `type`, `output_mode`, `-A`, `-B`, `-C`, `-i`, `multiline`, `head_limit`, `offset`
- **Behavior**: Searches file contents by regex pattern. Supports output modes: `content` (matching lines with context), `files_with_matches` (file paths only), `count` (match counts). Supports glob/type filters, context lines, case-insensitive, and multiline mode.

### TodoWrite (`TodoWrite`)

- **Parameters**: `todos` (required array of {id, content, status})
- **Behavior**: Replaces the session task list. Statuses: pending, in_progress, completed; at most one entry is in_progress.

### NoteWrite (`NoteWrite`)

- **Parameters**: `notes` (required array of strings)
- **Behavior**: Replaces the session's durable notes. At most 15 entries of 200 characters; an over-limit call fails with a message naming the limit.

An additional system prompt, when the run has one, contributes a fourth layer to
the system prompt and its `## Invariants` section is restated in the same block
these tools render into. See [design/context-durability.md](../../design/context-durability.md).

Both write durable session state rather than returning a formatted string and
nothing else. The state lives on `session.Session`, is reached through the
context (`agent.CtxWithNoteStore`) because the tool registry is cached per model
and shared across sessions, and is re-rendered after the message list on every
call by `agent.RenderSessionState`. It is therefore never trimmed and never
accumulates in the history. A subagent run is pointed at its own session, so it
cannot overwrite the state of the run that delegated to it. See
[design/context-durability.md](../../design/context-durability.md).

LLM-facing names are the camelCase constants in `names.go` — `Read`, `Write`,
`Edit`, `Glob`, `Grep`, `Bash`, `WebFetch`, `TodoWrite`, `NoteWrite`, `Skill`,
`Task` — plus
`LoadMcpTools` and `CallMcpTool` from `mcp_gateway.go`. `names.go` is the single
source of truth; hook matchers and subagent `tools:` fields match against these
exact strings.

## What A Tool Declares About Itself

Beyond `llm.Tool`, four optional interfaces feed the permission layer. Full
layering: [design/tool-permissions.md](../../design/tool-permissions.md).

**`Access(args)` is the one every tool should implement.** It answers whether
the call changes anything the user owns. The zero value is `AccessWrite`, so
omitting it is safe but uninformative — the tool will prompt on interactive
surfaces for no stated reason.

Two things it does *not* mean:

- **It is not a concurrency claim.** `AccessReadOnly` says the call changes
  nothing; it does not promise `Execute` is safe on several goroutines.
  `CallMcpTool` reports read-only on a third party's word, which this runtime
  cannot underwrite, which is why it declares `AccessWrite` at the tool level and
  makes its per-call decision in `CheckArgs` instead.
- **It is not the permission answer.** Permission is *derived* from it, and the
  derivation is deliberately not the tool's to make. A tool that could name its
  own action would eventually name `allow`.

**`CheckArgs` is for risk, not category.** `ReadFile` and `WriteFile` return
identical results from it — `Ask` for a sensitive path, `Allow` otherwise —
because the axis is how dangerous *this* call is, not what kind of act it is.
Note that `Allow` here means *abstain*: resolution continues to later layers.

**`DefaultAction` overrides the derivation, and needs a reason.** Three tools
implement it, all writes that must not prompt:

| Tool | Why |
|---|---|
| `TodoWrite`, `NoteWrite` | write the agent's own scratch state, not the user's files |
| `Bash` | has a sharper judgement of its own in `CheckArgs`; the category default would prompt for every `ls` |

**`GrantScope` is for tools that dispatch.** Without it, one session grant for
`CallMcpTool` would cover every tool on every configured server.

### Concurrency

The scheduler runs adjacent `AccessReadOnly` calls from one model message at the
same time, so declaring read-only carries a second obligation the type system
cannot check: **`Execute` must be safe to call from several goroutines at
once.** Read-only in the effect sense does not imply it. `WebFetch` is the case
to keep in mind — it changes nothing a user owns, and is only schedulable
because the response cache it writes is guarded by `cacheMu`. Remove that mutex
and it stays read-only and stops being safe to run in a batch.

What that means in practice: no unsynchronised package-level or struct-level
mutable state, and no assumption that a sibling is not touching the same file.
`./make test race` is the check; write the test that would catch it.

If a tool is read-only but genuinely cannot run concurrently, declare
`AccessWrite` and say why in the comment. `TodoWrite` and `NoteWrite` do exactly
that — they write only the agent's own scratch state, which is why they declare
`DefaultAction() = Allow` for permission, but that state has no lock, so the
write classification is what keeps them out of a batch.

### Adding a tool

Declare `Access`, and read the concurrency obligation above before choosing
`AccessReadOnly`. Add `CheckArgs` if some arguments are riskier than others.
Reach for `DefaultAction` only when the tool genuinely knows better than the
category, and say why in the comment. Then add a row to the table in
[design/tool-permissions.md](../../design/tool-permissions.md) section 6 —
`internal/tool/permission_test.go` is table-driven against it and will fail
until you do.

## How It Works

1. `internal/agentapp` resolves the workspace root and builds the base tool registry.
2. Base tools include file operations, bash, glob/grep, web fetch, todo, skill, and optional MCP gateway tools.
3. `internal/core/agent.RunLoop` receives a `llm.ToolRegistry`.
4. During the loop, when the LLM returns tool calls, the agent looks up each tool by name, parses JSON arguments, and calls `Execute()`.
5. Results (or errors) are appended to the active history as tool-role messages.

## Dependencies

- **Uses**: `internal/core/llm` (tool contracts), `internal/infra/llm` (for WebFetch's LLM caller), `internal/infra/mcp` (for MCP gateway)
- **Used by**: `internal/agentapp` (builds registries), `internal/core/agent` (executes tool calls)

## Notes

- All tools enforce path security — file operations must be under the configured root directory.
- Tool output is designed for LLM consumption: meaningful messages on both success and failure.
- Error messages are prefixed with `error:` by the agent when sent to the LLM.
- See also: [Agent Loop](agent-loop.md), [CLI](cli.md), [guide/tool-permissions.md](../../guide/tool-permissions.md).
