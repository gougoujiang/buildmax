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

- **CLI/TUI (primary)**: Command-line program with TUI (Text User Interface).
  - **Implementation**: Based on [Bubble Tea](https://github.com/charmbracelet/bubbletea)
  - Pure Go TUI framework, aligned with the project’s “all-Go stack”
  - Supports multiple components, message-driven flow, and keyboard/mouse interaction.
  - Users get a full Agent TUI experience in the terminal by running a single binary; no Node dependency for normal CLI use.
- **Portal (web)**: A separate web-based entry point under `portal/` — a minimal React (Vite + TypeScript) app that builds and runs independently. It provides a "BuildMax Portal" landing as a frontend foundation; chat, sessions, and API integration are planned for later. See `portal/README.md` for install, build, and dev commands.

### 2.3 Portal product vision (design reference)

High-level direction for the Portal / Nexus-style workspace (detailed design: **[design/001-about-portal.md](design/001-about-portal.md)** — read that doc on demand for full context):

- **Intent over tools**: User states goals; agent operates on a versioned text workspace (Markdown, CSV, JSON, YAML). Flow: Human → Agent → Workspace → Versioned state.
- **Agent loop**: Observe → Plan → Act → Observe. Agent reads/edits files, runs code, commits; user does not interact with files directly.
- **Workspace model**: Workspace (context) → Project (work unit) → Task (single run). Git is the hidden version engine; user sees timeline + restore, not commits/branches.
- **Principles**: Intent first; text as primary representation; state versioned and reversible; mechanisms hidden, meaning visible; workspace as the agent’s body.
- **Mental model**: User feels “I describe what I want” and “I can always go back,” not “I am operating software” or “I am managing versions.”

## 3. Goals and Principles

| Goal | Description |
|------|-------------|
| Generality | Agent can be configured to use different LLMs and tools, not tied to a single service |
| Ease of use | Runs with default configuration; advanced users can extend models and tools |
| Portability | Single binary or few files, easy to deploy on servers, local machines, or containers |
| All-Go implementation | Core and surrounding code in Go; call external APIs from Go when needed; no Python/Node runtime dependencies for CLI/TUI. The portal is an optional, separate frontend (React/Node tooling for dev and build). |

## 4. Core Capabilities

### 4.1 Implemented

- **LLM integration**: OpenAI-compatible client (OpenRouter default); env-based config (`OPENROUTER_API_KEY`/`BUILDMAX_API_KEY`, `BUILDMAX_BASE_URL`, `BUILDMAX_MODEL`)
- **Agent loop**: Single-turn flow with tool calling (LLM → tool_calls → execute tools → re-call LLM → reply) in `internal/agent`
- **Application data folder**: `config.DataDir()` — default `~/.buildmax`, override via `BUILDMAX_HOME`; `make test` uses `testing-sandbox`
- **Logging**: `log/slog` via `internal/log`; level from `BUILDMAX_LOG_LEVEL`; file-only (rotated file under `DataDir()/logs`, Lumberjack); no stdout/stderr so TUI and prompt output stay clean
- **Read file tool**: `internal/tools` — `read_file` with path under a configurable root (e.g. CWD); used in prompt mode
- **Default system prompt**: Prepended in agent `Process` (e.g. "You are a helpful AI assistant.")
- **Chat session**: In-memory session in `internal/session` (id, title, created_at, message history); multi-turn via same session
- **Session persistence**: Save/load under `DataDir()/sessions/<id>.json`; prompt mode saves after each run; `--resume <id> -p PROMPT` to resume
- **TUI**: Bubble Tea entry via `internal/app` + `internal/tui`; default when running `buildmax` with no flags. Layout: scrollable area (banner "BUILDMAX" + version, then chat history), input at bottom, footer (model, workspace, ctrl+c: quit). Run `buildmax` to start a new session; run `buildmax --resume <id>` to start the TUI with that session loaded. Session is persisted after each assistant reply.
- **CLI**: Cobra in `internal/cmd` — root command (TUI or `-p`/`--resume` prompt mode), `buildmax version` subcommand; `cmd/buildmax/main.go` is the thin entry point
- **Portal**: Web UI under `portal/` — React + Vite + TypeScript app; minimal "BuildMax Portal" landing; independent of the Go binary (`cd portal && npm install && npm run dev` / `npm run build`). No backend or agent features in the portal yet.

### 4.2 Planned / Not yet implemented

- Session list/delete from CLI; TUI session picker
- Portal: chat UI, session list, API integration with the Go backend
- Config subcommand, Viper, or config-file binding
- Additional tools (e.g. search, run commands)
- Shell completion (e.g. `buildmax completion bash`)

## 5. Project Directory Structure

Following common Golang project conventions, the current structure is:

```
buildmax/
├── cmd/
│   └── buildmax/          # Executable entry point
│       └── main.go        # main(), log init, cmd.NewRootCommand().Execute()
├── internal/              # Private packages (this project only)
│   ├── cmd/               # Cobra root command, flags, version subcommand, prompt/TUI runners
│   ├── app/               # App bootstrap and TUI program entry
│   ├── tui/               # Bubble Tea models and views
│   ├── agent/             # Core Agent logic (Process, tools, system prompt)
│   ├── llm/               # LLM client (OpenAI-compatible), types, ChatWithTools
│   ├── config/            # Config: LoadLLM(), DataDir()
│   ├── log/               # slog init, BUILDMAX_LOG_LEVEL, rotated file
│   ├── session/           # Session (id, title, history), SaveToDir, LoadFromDir
│   └── tools/             # Tool implementations (e.g. readfile)
├── portal/                # Web UI (React + Vite + TypeScript); independent of Go binary
│   ├── package.json       # Scripts: dev, build, preview
│   ├── vite.config.ts     # Vite config (build out: dist/)
│   ├── index.html         # Vite entry HTML
│   ├── README.md          # Install, build, dev instructions
│   └── src/               # main.tsx, App.tsx, index.css
├── configs/               # Config file examples (e.g. config.example.yaml)
├── example/               # Example files for tools (e.g. shakespeare.txt)
├── .vibe/                 # Task documents and design docs (vibe lifecycle)
├── make.bat               # Windows: build, test, run (build uses cmd/buildmax)
├── go.mod
├── go.sum
└── README.md
```

- **cmd/buildmax**: Single CLI entry point; `main.go` only. Build with `go build -o buildmax.exe ./cmd/buildmax` or `make.bat build`.
- **internal/cmd**: Cobra root command, CLI flags, version subcommand, prompt mode and TUI runners.
- **internal/**: Packages not exposed externally; can be split or partially moved to **pkg/** later.
- **portal/**: Frontend app; run with `cd portal && npm install && npm run dev`; build with `npm run build` (output in `portal/dist/`). No change to `go.mod` or Go build/test.

## 6. Documentation and Repository

- **Task docs**: `.vibe/NNN.md` (e.g. `.vibe/001.md`); design docs `.vibe/NNN-design.md` (e.g. `.vibe/008-design.md`). TOC: `.vibe/000-TOC.md`.
- **Product design (Portal)**: [design/001-about-portal.md](design/001-about-portal.md) — agent-mediated AI-native workspace vision, concepts, principles, wireframe. Read on demand for Portal/Nexus context.
- Code and scripts: repository root, managed with Go modules

### 6.1 Persistence naming style

- **Use the same naming style for all persisted data** (e.g. session files, config, any JSON on disk).
- **Convention: snake_case** for JSON object keys (e.g. `created_at`, `tool_call_id`, `tool_calls`).
- Ensure structs that are serialized to disk have explicit `json:"snake_case"` tags so the on-disk format is consistent; do not rely on Go’s default (PascalCase) for persisted fields.

### 6.2 Database table naming

- **Use singular table names.** One table per entity type, named in the singular (e.g. `user`, `workspace`, `project`, `task`). Do not use plural names (e.g. `users`, `workspaces`). This applies to all database tables created or migrated by the project.

### 6.3 Tool output for LLM

- **Tools are built for the LLM.** The agent passes tool results back to the model as tool-role messages.
- **Output meaningful results on both success and failure** so the LLM can understand what happened and decide on next steps (e.g. retry, inform the user, or continue).
- Success: return a clear, concise message (e.g. what was done or what was returned).
- Failure: return a clear error message (e.g. "path outside allowed root", "file not found"); the agent prefixes tool errors with `error: ` when sending to the LLM.

## 7. Build & Test

- **Build**: `make.bat build` (Windows) or `go build -o buildmax.exe ./cmd/buildmax` — builds from `cmd/buildmax`.
- **Test**: `make.bat test` — sets `BUILDMAX_HOME` to `./testing-sandbox` and runs `go test ./...`. Use this after code changes.
- **Do not use** `make.bat run` for automated flows; it is for manual testing (build + run with sample prompt).

## 8. Run command
prefer powershell syntax to run command instead of batch on windows platform

---

*This document is updated as the project evolves.*
