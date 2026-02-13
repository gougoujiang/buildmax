# Design 026 - Split internal/cmd/root.go by concern

## Goal

Redistribute `internal/cmd/root.go` into four focused files within the same package, introduce a `toolBuilder` helper, and extract a `resolveResumeID` function — all without changing any exported API.

## Modules

| Module (package) | Responsibility | Owns |
|------------------|----------------|------|
| **internal/cmd** | CLI command definitions, flag handling, setup wiring, run modes | `root.go`, `version.go`, `setup.go` (new), `prompt.go` (new), `tui.go` (new) |

No other packages are affected. All changes are within `internal/cmd`.

## Structure

**Directory / files**

- `internal/cmd/` — CLI package
  - `root.go` — Root command definition, flag routing, `Version`, `resolveResumeID` (~60 lines)
  - `version.go` — Version subcommand (unchanged)
  - `setup.go` — `toolBuilder`, `setupResult`, `buildBaseTools`, `buildAgentTypes`, `setupAgentAndSession` (~180 lines)
  - `prompt.go` — `runPromptMode` (~25 lines)
  - `tui.go` — `runTUI` (~25 lines)

**Main types and interfaces**

- **toolBuilder** (cmd): Accumulates tools and short-circuits on first error; fields: `tools []agent.Tool`, `byName map[string]agent.Tool`, `err error`.
- **setupResult** (cmd): Unchanged struct holding Agent, Session, SessionsDir, CWD, ModelName.

## Method design

| Receiver | Method/Func | Signature | Responsibility |
|----------|-------------|-----------|----------------|
| `*toolBuilder` | `add` | `(t agent.Tool, err error)` | Append tool if no prior error; record first error |
| `*toolBuilder` | `result` | `() ([]agent.Tool, map[string]agent.Tool, error)` | Return accumulated tools/map or the stored error |
| (package) | `newToolBuilder` | `() *toolBuilder` | Construct a toolBuilder with initialized map |
| (package) | `resolveResumeID` | `(resumeID string, cont bool) (string, error)` | If `cont && resumeID==""`, load session list and return last session ID |
| (package) | `buildBaseTools` | `(client *llm.Client, cwd string, skillPaths []string) ([]agent.Tool, map[string]agent.Tool, error)` | Same signature, refactored body using toolBuilder |

All other functions (`buildAgentTypes`, `setupAgentAndSession`, `runTUI`, `runPromptMode`, `NewRootCommand`, `runRoot`) keep their current signatures — only their file location changes.

## How they work together

**Data/control flow** (unchanged from current behavior)

1. `NewRootCommand` registers flags and returns the cobra command.
2. Cobra calls `runRoot` → `resolveResumeID` resolves the `--continue` flag → dispatches to `runPromptMode` or `runTUI`.
3. Both `runPromptMode` and `runTUI` call `setupAgentAndSession` (in `setup.go`).
4. `setupAgentAndSession` calls `buildBaseTools` → `buildAgentTypes` → constructs agent and session → returns `setupResult`.
5. `buildBaseTools` uses `toolBuilder.add(...)` for each tool, then `toolBuilder.result()` to get the final slice or error.

**Dependencies (imports per file)**

- `root.go`: `cobra`, `config`, `session`, `fmt`, `log/slog`, `os`
- `setup.go`: `agent`, `config`, `llm`, `session`, `tools`, `fmt`, `log/slog`, `os`, `path/filepath`, `time`
- `prompt.go`: `agent` (indirectly via setupResult), `session`, `context`, `fmt`, `log/slog`, `os`
- `tui.go`: `app`, `tui`, `bubbletea`, `log/slog`, `fmt`

**Key data structures**

- `toolBuilder`: Created in `buildBaseTools`, consumed locally. Tools are added via `add`, final slice retrieved via `result`.
- `setupResult`: Created in `setupAgentAndSession`, consumed by `runTUI` and `runPromptMode`.

## Changes for review

- **New**: `internal/cmd/setup.go` — `toolBuilder` (type + `add` + `result` + `newToolBuilder`), plus moved `setupResult`, `buildBaseTools` (refactored), `buildAgentTypes`, `setupAgentAndSession`.
- **New**: `internal/cmd/prompt.go` — moved `runPromptMode`.
- **New**: `internal/cmd/tui.go` — moved `runTUI`.
- **Modified**: `internal/cmd/root.go` — remove moved symbols; add `resolveResumeID`; simplify `runRoot` to use it. Shrinks from ~309 to ~60 lines.
- **Unchanged**: `internal/cmd/version.go`, `cmd/buildmax/main.go`, all other packages.
