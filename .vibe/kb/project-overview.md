# Project Overview

## Purpose

BuildMax is a general-purpose AI Agent CLI written entirely in Go. It provides an interactive TUI and a prompt mode for LLM-powered conversations with tool calling. The goal is a single-binary, zero-dependency agent that runs out of the box.

## Key Types and Interfaces

| Name | Kind | Role |
|------|------|------|
| **agent.Tool** | interface | Contract for all tools: Name, Description, Parameters, Execute |
| **agent.LLMCaller** | interface | Abstraction over LLM API calls (ChatWithTools) |
| **agent.Agent** | struct | Runs the agent loop: LLM call → tool calls → repeat |
| **llm.Client** | struct | OpenAI-compatible API client (implements LLMCaller) |
| **session.Session** | struct | In-memory conversation state with persistence |
| **tui.Model** | struct | Root Bubble Tea model for the terminal UI |

## Architecture

```
User
  │
  ├─ TUI mode ──► tui.Model ──► agent.Agent ──► llm.Client ──► OpenRouter API
  │                                  │
  └─ Prompt mode ──► agent.Agent ────┤
                                     ▼
                              agent.Tool (read, write, edit, bash, glob, grep, webfetch, todo)
```

The system has three layers:

1. **CLI layer** (`cmd/buildmax`): Cobra commands, flag parsing, dispatches to TUI or prompt mode.
2. **Agent layer** (`internal/core/agent`): The core loop — calls the LLM, processes tool calls, manages the conversation via Session.
3. **Infrastructure** (`internal/infra/llm`, `internal/config`, `internal/session`, `internal/execution/agenttool`, `internal/infra/log`): LLM client, configuration, session persistence, tool implementations, logging.

## Directory Layout

```
cmd/buildmax/          CLI entry point (main.go, root.go)
internal/
  agent/               Agent loop, Tool interface, LLMCaller interface
  llm/                 LLM client (OpenAI-compatible), message types
  config/              Env-based config (LLM settings, data dir)
  session/             Session state, JSON persistence, session list index
  tools/               8 tool implementations (read, write, edit, webfetch, bash, glob, grep, todo)
  tui/                 Bubble Tea TUI (model, viewport, input, formatting)
  app/                 TUI bootstrap (NewModel wrapper)
  log/                 slog init, file-only rotating log
```

## How It Works

1. `main.go` initializes logging, creates the Cobra root command, and runs it.
2. The root command parses flags (`-p`, `-r`, `-c`) and dispatches to TUI or prompt mode.
3. `setupAgentAndSession()` loads config, creates the LLM client, initializes all 8 tools, builds the Agent, and loads/creates a Session.
4. In **prompt mode**: `agent.Process()` runs the loop once and prints the reply.
5. In **TUI mode**: Bubble Tea runs `tui.Model`, which sends user input to `agent.ProcessAfterUserAppended()` in a background goroutine.
6. After each reply, `session.PersistAfterReply()` saves the session to disk.

## Dependencies

- **Go standard library** for most functionality
- **github.com/sashabaranov/go-openai** — OpenAI API client
- **github.com/charmbracelet/bubbletea** + **bubbles** + **lipgloss** — TUI framework
- **github.com/spf13/cobra** — CLI framework
- **github.com/google/uuid** — Session IDs
- **gopkg.in/natefinsh/lumberjack.v2** — Log rotation

## Notes

- All code is in Go — no Python, Node, or other runtime dependencies.
- Single binary distribution via `go build ./cmd/buildmax`.
- Config is entirely environment-variable based (no config files yet).
- See also: [Agent Loop](agent-loop.md), [LLM Client](llm-client.md), [Tools](tools.md), [Session](session.md), [TUI](tui.md), [Configuration](config.md), [CLI](cli.md).
