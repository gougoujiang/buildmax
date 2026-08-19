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
- See also: [Agent Loop](agent-loop.md), [CLI](cli.md).
