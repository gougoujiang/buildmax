# Tools

> **Audience:** users · **Status:** current

Tools are what turn a language model into an agent. Every run gets the same
built-in set; MCP servers, skills, and subagents add to it.

Tool names are what you write in a hook `matcher` and what shows up in `/tools`,
so they are worth knowing exactly.

## Built-in Tools

| Name | Does | Key arguments |
|---|---|---|
| `Read` | Read a file, with line numbers | `path`, `offset`, `limit` (default 1000 lines) |
| `Write` | Create or overwrite a file, creating parent directories | `path`, `content` |
| `Edit` | Exact string replacement in a file | `path`, `old_string`, `new_string`, `replace_all` |
| `Glob` | List files matching a pattern, newest first | `pattern` |
| `Grep` | Regex search over file contents | `pattern`, `path`, `glob`, `type`, `output_mode`, `-A/-B/-C`, `-i`, `multiline`, `head_limit` |
| `Bash` | Run a shell command in the workspace | `command`, `timeout` (default 120s, max 600s) |
| `WebFetch` | Fetch a URL as markdown, optionally summarized by the model | `url`, `prompt` |
| `TodoWrite` | Track multi-step progress | `todos[]` of `{id, content, status}` |
| `NoteWrite` | Keep durable notes that survive compaction | `notes[]` of strings |
| `Skill` | Load a skill's instructions | skill name |
| `Task` | Delegate to a subagent | `description`, `prompt`, `subagent_type` |
| `LoadMcpTools` / `CallMcpTool` | Discover and invoke MCP server tools | see [mcp.md](mcp.md) |

Run `/tools` in the TUI to see the set active for the current run — it varies
with what is configured.

## Behavior Worth Knowing

**`Edit` fails loudly on ambiguity.** If `old_string` matches more than once and
`replace_all` is not set, the edit is rejected rather than guessing. Give it more
surrounding context.

**`Grep` has three output modes.** `content` returns matching lines with context,
`files_with_matches` returns paths only, `count` returns counts. The model
usually picks well, but the modes are why grep results sometimes look different
between runs.

**`Bash` output is truncated at 30 000 characters** and combines stdout and
stderr. A command producing more than that should be redirected to a file the
agent then reads in ranges.

**`WebFetch` caches for 15 minutes** and converts HTML to markdown. On a
cross-host redirect it returns the redirect URL instead of following it, so the
agent decides whether to fetch the new host.

**`Read` returns the first 1000 lines by default.** Large files are read in
ranges via `offset` and `limit` rather than all at once.

**`NoteWrite` and `TodoWrite` outlive the conversation history.** Both replace
what they store rather than adding to it, so each call carries the complete
list. What they hold is shown to the agent on every turn and is not part of the
message history, which means it survives the compaction that eventually
discards the messages that produced it. Notes are capped at 15 entries of 200
characters; a longer list is rejected and the agent is asked to merge it. A
session that never writes either one carries nothing extra.

## The Path Boundary

File tools resolve every path against the workspace root and refuse anything
outside it. This boundary is independent of the sandbox — it applies whether or
not the sandbox is enabled, and it applies to `Read`, `Write`, `Edit`, `Glob`,
and `Grep`.

`Bash` is the exception: a shell command can reach anywhere the process can.
That is exactly what [the sandbox](sandbox.md) is for, and why it only covers
`Bash`.

## Failure Is Information

Tools are written for the model, not for a terminal. A failure returns a
specific message — `path outside allowed root`, `file not found`,
`old_string not found` — because the model's next move depends on which one it
was. When you see the agent recover gracefully from a bad edit, this is why.

## Extending The Set

| Mechanism | Adds | Guide |
|---|---|---|
| MCP servers | Tools from any MCP-compatible service | [mcp.md](mcp.md) |
| Skills | Instructions, invoked on demand rather than filling the prompt | [skills-and-subagents.md](skills-and-subagents.md) |
| Subagents | A separate agent with its own tool subset and prompt | [skills-and-subagents.md](skills-and-subagents.md) |

## Related

- [tool-permissions.md](tool-permissions.md) — which of these stop and ask before running
- [hooks.md](hooks.md) — gate tool calls by name, using the names above
- [sandbox.md](sandbox.md) — confine what `Bash` can reach
- [contribute/architecture/tools.md](../contribute/architecture/tools.md) — how the registry is built
