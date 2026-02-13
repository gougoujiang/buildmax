# Design 025 - Refactor root.go — smell proposals

## Goal

Restructure `cmd/buildmax/root.go` by extracting tool construction and agent-type wiring into dedicated functions, introducing a result struct, and fixing inconsistent error handling — without changing external behavior.

## Modules

| Module (package) | Responsibility | Owns |
|------------------|----------------|------|
| **cmd/buildmax** | CLI entry, Cobra commands, wiring of agent/session/tools | `root.go`, `main.go` |

No other packages are modified.

## Structure

**Directory / files**

- `cmd/buildmax/` — CLI entry point
  - `root.go` — refactored: new types and helper functions (all changes here)
  - `main.go` — unchanged

**Main types and interfaces**

- **setupResult** (main): Result struct returned by `setupAgentAndSession`. Fields: `Agent *agent.Agent`, `Session *session.Session`, `SessionsDir string`, `CWD string`, `ModelName string`.

## Method design

| Receiver | Method | Signature | Responsibility |
|----------|--------|-----------|----------------|
| — | buildBaseTools | `(client *llm.Client, cwd string, skillPaths []string) ([]agent.Tool, map[string]agent.Tool, error)` | Construct all 9 base tools (ReadFile, WriteFile, WebFetch, TodoWrite, Bash, Glob, EditFile, Grep, Skill). Return the tool slice and a name→tool lookup map. Log and wrap each constructor error. |
| — | buildAgentTypes | `(baseTools []agent.Tool, toolsByName map[string]agent.Tool, cwd string) (map[string]tools.AgentTypeConfig, error)` | Define built-in agent types (general, explore, shell). Load user-defined agent defs from `config.AgentDefsSearchPaths(cwd)`, resolve tool names, merge into map. Return the combined agent-type map. Never returns error from user-def loading (warns only), but signature keeps error for future use. |
| — | setupAgentAndSession | `(resumeID string) (setupResult, error)` | Orchestrate: LoadLLM → validate key → getwd → NewClient → buildBaseTools → buildAgentTypes → NewTask → NewAgent → ensure sessions dir → load or create session. Return `setupResult`. |
| — | runRoot | `(cmd *cobra.Command, _ []string) error` | Parse flags. If `--continue`, resolve last session. If `--prompt`, call `runPromptMode` and **return its error** (no `os.Exit`). Otherwise call `runTUI`. |
| — | runTUI | `(resumeID string) error` | Call `setupAgentAndSession`. Use `res.ModelName` for TUI opts (no second `config.LoadLLM()`). |
| — | runPromptMode | `(prompt, resumeID string) error` | Call `setupAgentAndSession`. Use `res.CWD` and `res.SessionsDir` from the result struct. |

## How they work together

**Data/control flow**

1. `runRoot` parses CLI flags, dispatches to `runTUI` or `runPromptMode`.
2. Both callers invoke `setupAgentAndSession(resumeID)` which returns `setupResult`.
3. `setupAgentAndSession` calls `buildBaseTools` to get `(baseTools, toolsByName)`.
4. `setupAgentAndSession` calls `buildAgentTypes(baseTools, toolsByName, cwd)` to get `agentTypes`.
5. `setupAgentAndSession` creates the Task tool via `tools.NewTask(client, agentTypes)`, builds the agent via `agent.NewAgent(client, append(baseTools, taskTool))`, sets up session dir and session.
6. `runTUI` reads `res.ModelName` directly — no second config load.
7. `runPromptMode` reads `res.CWD` and `res.SessionsDir` from the struct.

**Dependencies**

- No dependency changes. All imports remain the same.

**Key data structures**

- `setupResult`: Created by `setupAgentAndSession`, consumed by `runTUI` and `runPromptMode`. Carries everything both callers need.

## Changes for review

- **New**: `setupResult` struct — replaces the 5-value return tuple, adds `ModelName`.
- **New**: `buildBaseTools(client, cwd, skillPaths)` — encapsulates 9 tool constructors.
- **New**: `buildAgentTypes(baseTools, toolsByName, cwd)` — encapsulates built-in + user-defined agent type wiring.
- **Modified**: `setupAgentAndSession` — slimmed to ~30–40 lines, returns `setupResult`.
- **Modified**: `runRoot` — removes `os.Exit(1)`, returns error from `runPromptMode`.
- **Modified**: `runTUI` — uses `setupResult`, removes duplicate `config.LoadLLM()`.
- **Modified**: `runPromptMode` — uses `setupResult` fields.
