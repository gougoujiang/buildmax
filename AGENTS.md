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

## 4. Core Capabilities (Planned)

- **LLM integration**: Support mainstream APIs (OpenAI-compatible, local models, etc.) with a unified abstraction
- **Conversation and history**: Multi-turn dialogue, context management, optional persistence
- **Tools/plugins**: Extensible tool calls (e.g. search, run commands, read files); interfaces defined in Go
- **TUI experience**: Bubble Tea–based chat UI, status bar, simple menus and config entry points

## 5. Project Directory Structure

Following common Golang project conventions, the current structure is:

```
buildmax/
├── cmd/
│   └── buildmax/          # Executable entry point
│       └── main.go
├── internal/              # Private packages (this project only)
│   ├── app/               # App bootstrap and TUI program entry
│   ├── tui/               # Bubble Tea models and views
│   ├── agent/             # Core Agent logic (planning/tools/conversation)
│   ├── llm/               # LLM client abstraction and implementations
│   └── config/            # Config loading and defaults
├── configs/               # Config files (examples, etc.)
├── task/                  # Task documents
├── scripts/               # Build, install, and other scripts
├── go.mod
├── go.sum
└── README.md
```

- **cmd/**: Each subdirectory corresponds to one executable; `cmd/buildmax` is the only CLI for now.
- **internal/**: Packages not exposed externally; can be split or partially moved to **pkg/** later.

## 6. Documentation and Repository

- Task docs: e.g `task/001.md`, and design doc 'task/001-design.md'
- Code and scripts: repository root, managed with Go modules

## 7. Build&Test
- Use ./make build to build the project after code change
- Use ./make test to run go test
- Do NOT use ./make run, as it is for maunal testing

---

*This document is updated as the project evolves.*
