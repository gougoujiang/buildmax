# Tools

## Purpose

The `internal/tools` package provides all tool implementations the agent can invoke. Each tool implements the `agent.Tool` interface and is registered with the Agent at startup. Tools are designed for LLM consumption — their results are sent back to the model as tool-role messages.

## Key Types and Interfaces

| Name | Kind | Role |
|------|------|------|
| **agent.Tool** | interface | Contract: `Name()`, `Description()`, `Parameters()`, `Execute(ctx, args)` |
| **ReadFile** | struct | Reads files under a root directory |
| **WriteFile** | struct | Creates/overwrites files under a root directory |
| **EditFile** | struct | Performs exact string replacements in files |
| **WebFetch** | struct | Fetches URLs, converts HTML to markdown |
| **Bash** | struct | Runs shell commands in the workspace |
| **Glob** | struct | Lists files matching glob patterns |
| **Grep** | struct | Searches file contents by regex |
| **TodoWrite** | struct | Formats task lists for progress tracking |

## Tool Inventory

### ReadFile (`read_file`)
- **Parameters**: `path` (required), `offset` (optional, 1-based line), `limit` (optional, default 1000)
- **Behavior**: Reads file content with line numbers (`LINE|CONTENT` format). Supports offset/limit for large files. Path must be under the configured root.
- **Error handling**: Returns clear errors for path-outside-root, file-not-found, etc.

### WriteFile (`write_file`)
- **Parameters**: `path` (required), `content` (required)
- **Behavior**: Creates or overwrites a file. Creates parent directories as needed. Path must be under root.

### EditFile (`edit_file`)
- **Parameters**: `path` (required), `old_string` (required), `new_string` (required), `replace_all` (optional bool)
- **Behavior**: Performs exact string replacement in a file. By default replaces the first unique match; `replace_all` replaces all occurrences. Fails if `old_string` is not found or is ambiguous (multiple matches without `replace_all`).

### WebFetch (`web_fetch`)
- **Parameters**: `url` (required)
- **Behavior**: Fetches a URL, converts HTML to markdown. Caches results (default 15 min TTL). Optionally uses LLM to process/summarize content.

### Bash (`bash`)
- **Parameters**: `command` (required), `timeout` (optional, default 120s, max 600s)
- **Behavior**: Runs a shell command in the workspace root. Returns combined stdout+stderr. Output truncated at 30k characters.

### Glob (`glob`)
- **Parameters**: `pattern` (required)
- **Behavior**: Lists files matching a glob pattern under the root. Returns paths sorted by modification time (newest first). Patterns not starting with `**/` are auto-prefixed for recursive search.

### Grep (`grep`)
- **Parameters**: `pattern` (required), plus optional `path`, `glob`, `type`, `output_mode`, `-A`, `-B`, `-C`, `-i`, `multiline`, `head_limit`, `offset`
- **Behavior**: Searches file contents by regex pattern. Supports output modes: `content` (matching lines with context), `files_with_matches` (file paths only), `count` (match counts). Supports glob/type filters, context lines, case-insensitive, and multiline mode.

### TodoWrite (`todo_write`)
- **Parameters**: `todos` (required array of {id, content, status})
- **Behavior**: Formats a task list for progress tracking. Statuses: pending, in_progress, completed.

## How It Works

1. At startup, `setupAgentAndSession()` in `cmd/buildmax/root.go` creates each tool with the workspace root (CWD) as the allowed directory.
2. Tools are passed to `agent.NewAgent()`, which caches their definitions as `llm.ToolDef` for the LLM.
3. During the agent loop, when the LLM returns tool calls, the agent looks up each tool by name, parses the JSON arguments, and calls `Execute()`.
4. Results (or errors) are appended to the session as tool-role messages.

## Dependencies

- **Uses**: `internal/agent` (Tool interface), `internal/llm` (for WebFetch's LLM caller)
- **Used by**: `cmd/buildmax` (creates tools), `internal/agent` (executes tools)

## Notes

- All tools enforce path security — file operations must be under the configured root directory.
- Tool output is designed for LLM consumption: meaningful messages on both success and failure.
- Error messages are prefixed with `error:` by the agent when sent to the LLM.
- See also: [Agent Loop](agent-loop.md), [CLI](cli.md).
