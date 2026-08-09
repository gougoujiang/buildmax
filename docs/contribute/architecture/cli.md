# CLI

## Purpose

The `cmd/buildmax` package is the executable entry point. It uses Cobra for command-line parsing and dispatches to either the TUI or prompt mode based on flags.

## Key Types and Interfaces

| Name | Kind | Role |
|------|------|------|
| **Version** | var | Application version string (e.g. "0.0.1") |
| **newRootCommand()** | func | Creates the Cobra root command with all flags |
| **setupAgentAndSession()** | func | Bootstraps agent, tools, and session for both modes |

## How It Works

### Entry Point (`main.go`)

```go
func main() {
    log.Init()
    root := newRootCommand()
    root.Execute()
}
```

### Command Structure

```
buildmax                     Start TUI (new session)
buildmax -r ID               Start TUI with resumed session
buildmax -c                  Resume most recent session
buildmax -p PROMPT           Prompt mode (no TUI)
buildmax -r ID -p PROMPT     Resume session, send prompt
buildmax version             Print version
```

### Flags

| Flag | Short | Type | Description |
|------|-------|------|-------------|
| `--prompt` | `-p` | string | Send prompt to LLM, print response, exit |
| `--resume` | `-r` | string | Session ID to resume |
| `--continue` | `-c` | bool | Resume most recent session (by creation time) |

### Dispatch Logic (`runRoot`)

1. If `--continue` is set and no `--resume`, load the session list and pick the most recent session.
2. If `--prompt` is set → run prompt mode (`runPromptMode`).
3. Otherwise → run TUI mode (`runTUI`).

### Agent & Session Setup (`setupAgentAndSession`)

This function is shared by both modes:

1. `config.LoadLLM()` — load API key, base URL, model from env.
2. `os.Getwd()` — get current working directory (used as tool root).
3. Create all 8 tools: ReadFile, WriteFile, EditFile, WebFetch, TodoWrite, Bash, Glob, Grep — each with CWD as root.
4. `agent.NewAgent(client, tools)` — build the agent.
5. Ensure `DataDir()/sessions/` exists.
6. If `resumeID` provided → `session.LoadFromDir()`. Otherwise → `session.NewSession("")`.

### Prompt Mode (`runPromptMode`)

1. Call `agent.Process(ctx, sess, prompt)`.
2. Call `session.PersistAfterReply()` to save.
3. Print the reply to stdout.

### TUI Mode (`runTUI`)

1. Build `tui.TUIOpts` with agent, session, model name, workspace, version, sessions dir.
2. Start Bubble Tea program with `tea.WithAltScreen()`.

## Dependencies

- **Uses**: All `internal/` packages (agent, llm, config, session, tools, tui, app, log)
- **External**: `github.com/spf13/cobra` (CLI framework), `github.com/charmbracelet/bubbletea` (TUI runner)

## Notes

- Both modes share the same `setupAgentAndSession` — changing tool registration or config logic only requires one change.
- The `version` subcommand is added via `root.AddCommand(newVersionCommand())`.
- See also: [Project Overview](overview.md), [Agent Loop](agent-loop.md), [TUI](tui.md), [Configuration](config.md).
