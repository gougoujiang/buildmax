# Design 023 - Add Skill Tool

## Goal

Add a `Skill` tool to `internal/tools` that discovers `SKILL.md` files from configurable search paths at construction time and, when invoked by the LLM, reads and returns the requested skill's content so the agent can follow specialized instructions.

## Modules

| Module (package) | Responsibility | Owns |
|------------------|----------------|------|
| **internal/tools** | Skill tool implementation: discovery, description, execution. | `SkillTool`, `SkillEntry`, `skill.go`, `skill_test.go` |
| **cmd/buildmax** | Constructs the skill tool with search paths and wires it into the agent. | Changes to `root.go` (`setupAgentAndSession`). |

## Structure

**Directory / files**

- `internal/tools/` — tool implementations
  - `skill.go` — `SkillEntry`, `SkillTool`, constructor, discovery helpers, interface methods
  - `skill_test.go` — table-driven unit tests
- `cmd/buildmax/`
  - `root.go` — add skill tool construction and wiring (modified)

**Main types**

- **SkillEntry** (tools): Data holder for one discovered skill. Fields: `Name string`, `Description string`, `Path string` (absolute path to the SKILL.md file).
- **SkillTool** (tools): Implements `agent.Tool`. Fields: `skills []SkillEntry` (ordered list for deterministic description), `byName map[string]SkillEntry` (fast lookup by name).

## Method design

| Receiver / Pkg | Method | Signature | Responsibility |
|----------------|--------|-----------|----------------|
| (pkg-level) | `discoverSkills` | `(searchPaths []string) []SkillEntry` | Scan each search path one level deep for subdirs containing `SKILL.md`. First-path-wins on name conflicts. Missing dirs silently skipped. Returns sorted slice. |
| (pkg-level) | `extractDescription` | `(content []byte) string` | Return the first non-empty, non-heading (`#`) line from SKILL.md content, trimmed. If none found, return `"(no description)"`. |
| `*SkillTool` | `NewSkill` | `(searchPaths []string) (*SkillTool, error)` | Calls `discoverSkills`, builds `byName` map, returns the tool. Never returns error (discovery is best-effort), but signature includes `error` for consistency with other constructors. |
| `*SkillTool` | `Name` | `() string` | Returns `"Skill"`. |
| `*SkillTool` | `Description` | `() string` | Static preamble + dynamic listing of each skill (name: description). If no skills discovered, notes that no skills are available. |
| `*SkillTool` | `Parameters` | `() any` | Returns JSON schema: `skill` (string, required), `args` (string, optional). |
| `*SkillTool` | `Execute` | `(ctx context.Context, args map[string]any) (string, error)` | Validate `skill` arg; look up in `byName`; read the SKILL.md file at `entry.Path`; if `args` param is non-empty, prepend `"Arguments: <args>\n\n"` to the content; return content. Error on unknown skill (listing available names). |

## How they work together

**Data / control flow**

1. **Startup** (`cmd/buildmax/root.go` → `setupAgentAndSession`):
   - Build search paths: `[]string{<cwd>/.buildmax/skills, <cwd>/.cursor/skills, <home>/.buildmax/skills}`.
   - Call `tools.NewSkill(searchPaths)` → returns `*SkillTool`.
   - Add to `baseTools` slice (before Task tool creation, so sub-agents also get it).

2. **Discovery** (`NewSkill` → `discoverSkills`):
   - For each search path, call `os.ReadDir`. Skip if `os.ErrNotExist`.
   - For each subdirectory entry, check if `<subdir>/SKILL.md` exists (`os.Stat`).
   - If it exists, read the file, call `extractDescription` for the short description.
   - If a skill with the same name was already found from an earlier search path, skip it (first wins).
   - Sort the final list alphabetically by name for deterministic Description output.

3. **Agent registration** (`agent.NewAgent`):
   - The `SkillTool` is in the tools slice. `NewAgent` calls `Name()`, `Description()`, `Parameters()` to build `llm.ToolDef` and stores the tool in `toolsByName["Skill"]`.

4. **LLM invokes Skill** (agent `processOneToolCall`):
   - LLM sends `tool_call` with `name: "Skill"`, `arguments: {"skill": "vibe", "args": "start add-logging"}`.
   - Agent looks up `toolsByName["Skill"]`, calls `Execute(ctx, args)`.
   - `Execute` finds `"vibe"` in `byName`, reads `SKILL.md` from disk, prepends args line, returns the full content.
   - Agent appends the content as a tool-role message. LLM now has the skill instructions in context.

**Dependencies**

- `internal/tools` depends on `internal/agent` for the `Tool` interface (already the case for all tools).
- `cmd/buildmax` depends on `internal/tools` for `NewSkill` (already imports the package).
- No new external dependencies. Only `os`, `path/filepath`, `strings`, `sort`, `context`, `errors`, `fmt`, `log/slog` from stdlib.

**Key data structures**

- `SkillEntry{Name, Description, Path}`: Created during discovery, stored in `SkillTool.skills` and `SkillTool.byName`. Consumed by `Description()` for listing and by `Execute()` for file path lookup.

## Changes for review

- **New**: `internal/tools/skill.go` — `SkillEntry` struct, `SkillTool` struct, `NewSkill` constructor, `discoverSkills` and `extractDescription` helpers, `Name`/`Description`/`Parameters`/`Execute` methods.
- **New**: `internal/tools/skill_test.go` — Table-driven tests for discovery, execute, priority, description extraction, and edge cases.
- **Modified**: `cmd/buildmax/root.go` — In `setupAgentAndSession()`, add `skillTool` construction with 3 search paths and append to `baseTools`.
