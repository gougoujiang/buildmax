# Design 018 - Bash tool

## Goal

Enable the agent to run shell commands in the workspace (e.g. git, npm, docker) via a new Bash tool: one command per call, configurable timeout, combined stdout+stderr, and output truncation at 30k runes, with clear success/failure messages for the LLM.

## Modules

| Module (package) | Responsibility | Owns |
|------------------|----------------|------|
| **internal/tools** | Concrete agent tools (Read, Write, WebFetch, TodoWrite). | Tool types, New* constructors, Execute logic; bash.go, bash_test.go. |
| **internal/agent** | Agent loop, tool interface, dispatch by name. | Tool interface; no change in this task. |
| **cmd/buildmax** | CLI entry, setup of LLM client and tools, TUI/prompt mode. | root.go: setupAgentAndSession creates tools and passes them to NewAgent. |

## Structure

**Directory / files**

- `internal/tools/`
  - `bash.go` — Bash tool: struct, NewBash, Tool implementation (Name, Description, Parameters, Execute).
  - `bash_test.go` — Unit tests for NewBash and Execute (success, non-zero exit, timeout, truncation, invalid args).

**Main types and interfaces**

- **Bash** (internal/tools): Tool that runs one shell command per call. Holds `root string` (absolute path = working directory). Implements `agent.Tool`.
- **agent.Tool** (existing): `Name() string`, `Description() string`, `Parameters() any`, `Execute(ctx, args) (string, error)`.

## Method design

| Receiver | Method | Signature | Responsibility |
|----------|--------|-----------|-----------------|
| **(package)** | **NewBash** | `(root string) (*Bash, error)` | If root empty, use `os.Getwd()`. Absolutize and clean root; return `&Bash{root: ...}` or error. Same pattern as NewReadFile/NewWriteFile. |
| **Bash** | Name | `() string` | Return `"Bash"`. |
| **Bash** | Description | `() string` | One short paragraph: run shell command in workspace; optional timeout (default 2 min, max 10 min); output truncated at 30k chars; prefer Read/Write/Glob/Grep for file ops. |
| **Bash** | Parameters | `() any` | OpenAI-style schema: `command` (string, required), `timeout` (number, optional; default 120000, max 600000 ms). |
| **Bash** | Execute | `(ctx context.Context, args map[string]any) (string, error)` | Parse `command` (required, non-empty after trim); parse `timeout` (default 120_000, max 600_000; invalid → default). Build shell argv (see below). Run with context that honours both ctx and timeout; Dir = b.root; capture combined stdout+stderr. On exit 0 return output (truncate at 30k runes + note). On non-zero exit or run error return self-explanatory message (e.g. "exit code N" + truncated output). |

**Shell invocation**

- **Unix**: Prefer `bash -c "<command>"`; if `exec.LookPath("bash")` fails, use `sh -c "<command>"`. Single argument after `-c` is the user command (properly passed to exec, no extra shell parsing in Go).
- **Windows**: Use `cmd /c "<command>"` (or `cmd /c <command>` with command as one arg). Ensure the command string is passed as a single argument so that `dir "C:\Program Files"`-style quoting works.
- **Process**: Use `exec.CommandContext(runCtx, name, args...)` with `cmd.Dir = b.root`, `cmd.Stdout` and `cmd.Stderr` set to the same buffer (combined output). When runCtx is cancelled or times out, kill the process (CommandContext does this). Parse timeout from args and create a context with timeout; merge with ctx using a context that cancels when either cancels (e.g. short-lived context with the lesser of ctx.Deadline and timeout).

**Output truncation**

- After capturing output, if rune count > 30_000, return `output[:30_000] + "\n(output truncated; total N characters)"` (N = full rune count). Reuse the same pattern as in webfetch (truncateContent-style) for consistency if desired; no need to import webfetch, just the same logic.

**Failure handling**

- Non-zero exit: return a string like `"exit code 1\n" + truncatedOutput` (no error return, so the agent gets the message as tool content and can interpret it). Alternatively return an error so the agent prefixes "error: "; the task says "the tool's returned string should be self-explanatory" and "return a clear message (e.g. exit code N plus truncated output)". So returning a descriptive string (and no error) for non-zero exit is acceptable; the LLM sees the content. If we return error, the agent will send "error: exit code 1\n...". Either way is fine; design choice: **return (msg, nil)** for non-zero exit so the tool message is the single source of truth (e.g. "Command failed with exit code 2:\n" + output). If the process could not be started or was killed by timeout, return (msg, err) or (msg, nil) with msg explaining timeout/kill; prefer (msg, nil) for consistency so LLM always gets a string.
- Recommendation: On non-zero exit or timeout/kill, return **(message string, nil)** with a clear message; reserve **error** for argument validation (missing/empty command, invalid args). That way the LLM always receives a readable tool result; the agent does not add "error: " for exit-code failures, and the tool message can say "Command failed with exit code 2" or "Command timed out after 2s".

## How they work together

**Data/control flow**

1. User sends a message in TUI or prompt mode. Agent runs processLoop; LLM may return tool_calls including `Bash` with arguments `{"command": "git status", "timeout": 60000}`.
2. Agent resolves tool by name "Bash", parses args, calls `bashTool.Execute(ctx, args)`.
3. Bash.Execute parses command and timeout, builds shell argv, runs `exec.CommandContext` with Dir = root; context is bounded by both request ctx and timeout. Stdout and stderr are combined into one buffer.
4. On success (exit 0): Bash returns truncated output (if > 30k runes) and nil error. On non-zero exit or timeout: Bash returns a clear message string and nil (or error only for bad args). Agent appends one tool message (Role: tool, Content: result) to the session.
5. Agent continues the loop; LLM sees the tool result and may reply to the user or issue further tool calls.

**Dependencies**

- **internal/tools** depends on **internal/agent** only for the Tool interface (Name, Description, Parameters, Execute). No dependency on llm or config.
- **cmd/buildmax** depends on **internal/tools** (NewBash), **internal/agent** (NewAgent), and passes cwd into NewBash.

**Key data structures**

- **args** (map[string]any): From LLM tool call JSON; Bash expects `command` (string), optional `timeout` (number). Agent passes this to Execute after unmarshalling.
- **Bash.root**: Set once by NewBash(cwd); used as process working directory for every Execute.

## Changes for review

- **New**: `internal/tools/bash.go` — Type `Bash` with field `root string`. `NewBash(root string) (*Bash, error)`. Methods `Name`, `Description`, `Parameters`, `Execute` implementing `agent.Tool`. Execute: parse args; build shell command (bash/sh on Unix, cmd on Windows); run with `exec.CommandContext`, Dir = root, combined stdout+stderr; apply timeout and ctx; truncate output at 30k runes; on success return output; on non-zero exit or timeout return descriptive message (and nil error per above).
- **New**: `internal/tools/bash_test.go` — Tests for NewBash (empty root, valid root); Execute success (e.g. echo); Execute non-zero exit; Execute timeout; Execute output truncation; missing/empty command; invalid timeout clamped to default.
- **Modified**: `cmd/buildmax/root.go` — In `setupAgentAndSession`, after `todoWriteTool`: add `bashTool, err := tools.NewBash(cwd)`; on error log and return; pass `readFileTool, writeFileTool, webFetchTool, todoWriteTool, bashTool` to `NewAgent`.
