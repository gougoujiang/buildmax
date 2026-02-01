# BuildMax - Project Design Document

## 1. Overview

BuildMax is an **AI Agent project** aimed at building a **general-purpose Agent** so that users can:

- **Run quickly**: Out of the box with minimal configuration and dependencies
- **Get AI Agent capabilities**: Typical Agent features such as LLM interaction, task planning, and tool calling

The project targets users who want to deploy AI Agents locally or privately, providing a unified, extensible Agent runtime.

## 2. Technology Choices

### 2.1 Language and Ecosystem

- **Primary language: Golang (Go)**
- **Principle: Implement all components in Golang where possible**, including:
  - Core Agent logic
  - CLI and TUI interface
  - LLM communication and plugin/tool wrappers
  - Infrastructure such as config, logging, and persistence

Rationale: A single language reduces maintenance cost, enables cross-compilation and single-binary distribution, and facilitates collaboration and contribution.

### 2.2 User Interface

- **Form: Command-line program (CLI)**
- **Interaction: TUI (Text User Interface)**
- **Implementation: Based on [Bubble Tea](https://github.com/charmbracelet/bubbletea)**
  - Pure Go TUI framework, aligned with the project’s “all-Go stack”
  - Supports multiple components, message-driven flow, and keyboard/mouse interaction; suitable for Agent chat, status display, menus, etc.

Users get a full Agent TUI experience in the terminal by running a single command.

## 3. Goals and Principles

| Goal | Description |
|------|-------------|
| Generality | Agent can be configured to use different LLMs and tools, not tied to a single service |
| Ease of use | Runs with default configuration; advanced users can extend models and tools |
| Portability | Single binary or few files, easy to deploy on servers, local machines, or containers |
| All-Go implementation | Core and surrounding code in Go; call external APIs from Go when needed; no Python/Node runtime dependencies |

## 4. Core Capabilities

### 4.1 Implemented

- **LLM integration**: OpenAI-compatible client (OpenRouter default); env-based config (`OPENROUTER_API_KEY`/`BUILDMAX_API_KEY`, `BUILDMAX_BASE_URL`, `BUILDMAX_MODEL`)
- **Agent loop**: Single-turn flow with tool calling (LLM → tool_calls → execute tools → re-call LLM → reply) in `internal/agent`
- **Application data folder**: `config.DataDir()` — default `~/.buildmax`, override via `HOME_DIR`; `make test` uses `testing-sandbox`
- **Logging**: `log/slog` via `internal/log`; level from `BUILDMAX_LOG_LEVEL`; stderr + rotated file under `DataDir()/logs` (Lumberjack)
- **Read file tool**: `internal/tools` — `read_file` with path under a configurable root (e.g. CWD); used in prompt mode
- **Default system prompt**: Prepended in agent `Process` (e.g. "You are a helpful AI assistant.")
- **Chat session**: In-memory session in `internal/session` (id, title, created_at, message history); multi-turn via same session
- **Session persistence**: Save/load under `DataDir()/sessions/<id>.json`; prompt mode saves after each run; `--resume <id> -p PROMPT` to resume
- **TUI**: Bubble Tea entry via `internal/app` + `internal/tui`; default when running `buildmax` with no flags. Layout: scrollable area (banner "BUILDMAX" + version, then chat history), input at bottom, footer (model, workspace, ctrl+c: quit). Run `buildmax` to start a new session; run `buildmax --resume <id>` to start the TUI with that session loaded. Session is persisted after each assistant reply.
- **CLI**: Cobra in `cmd/buildmax` — root command (TUI or `-p`/`--resume` prompt mode), `buildmax version` subcommand

### 4.2 Planned / Not yet implemented

- Session list/delete from CLI; TUI session picker
- Config subcommand, Viper, or config-file binding
- Additional tools (e.g. search, run commands)
- Shell completion (e.g. `buildmax completion bash`)

## 5. Project Directory Structure

Following common Golang project conventions, the current structure is:

```
buildmax/
├── cmd/
│   └── buildmax/          # Executable entry point
│       ├── main.go        # main(), log init, root.Execute(), runPromptMode()
│       └── root.go        # Cobra root command, -p/--resume, version subcommand
├── internal/              # Private packages (this project only)
│   ├── app/               # App bootstrap and TUI program entry
│   ├── tui/               # Bubble Tea models and views
│   ├── agent/             # Core Agent logic (Process, tools, system prompt)
│   ├── llm/               # LLM client (OpenAI-compatible), types, ChatWithTools
│   ├── config/            # Config: LoadLLM(), DataDir()
│   ├── log/               # slog init, BUILDMAX_LOG_LEVEL, rotated file
│   ├── session/           # Session (id, title, history), SaveToDir, LoadFromDir
│   └── tools/             # Tool implementations (e.g. readfile)
├── configs/               # Config file examples (e.g. config.example.yaml)
├── example/               # Example files for tools (e.g. shakespeare.txt)
├── tasks/                 # Task documents and design docs
├── make.bat               # Windows: build, test, run (build uses cmd/buildmax)
├── go.mod
├── go.sum
└── README.md
```

- **cmd/buildmax**: Single CLI; `main.go` + `root.go` (Cobra). Build with `go build -o buildmax.exe ./cmd/buildmax` or `make.bat build`.
- **internal/**: Packages not exposed externally; can be split or partially moved to **pkg/** later.

## 6. Documentation and Repository

- **Task docs**: `tasks/NNN.md` (e.g. `tasks/001.md`); design docs `tasks/NNN-design.md` (e.g. `tasks/008-design.md`). TOC: `tasks/000-TOC.md`.
- Code and scripts: repository root, managed with Go modules

### 6.1 Persistence naming style

- **Use the same naming style for all persisted data** (e.g. session files, config, any JSON on disk).
- **Convention: snake_case** for JSON object keys (e.g. `created_at`, `tool_call_id`, `tool_calls`).
- Ensure structs that are serialized to disk have explicit `json:"snake_case"` tags so the on-disk format is consistent; do not rely on Go’s default (PascalCase) for persisted fields.

## 7. Build & Test

- **Build**: `make.bat build` (Windows) or `go build -o buildmax.exe ./cmd/buildmax` — builds from `cmd/buildmax`.
- **Test**: `make.bat test` — sets `HOME_DIR` to `./testing-sandbox` and runs `go test ./...`. Use this after code changes.
- **Do not use** `make.bat run` for automated flows; it is for manual testing (build + run with sample prompt).

---

*This document is updated as the project evolves.*
