# Design 024 - Consolidate Agent Used Dirs

## Goal

Centralize skill and agent-definition search path construction into `internal/config`, add a multi-directory agent-def loader, and update `root.go` to use these helpers — replacing scattered, inconsistent path logic.

## Modules

| Module (package) | Responsibility | Owns |
|------------------|----------------|------|
| **internal/config** | Application configuration: LLM settings, data directory, and now search path construction for skills and agent defs. | `LLM`, `DataDir()`, new `SkillSearchPaths()`, `AgentDefsSearchPaths()` |
| **internal/tools** | Tool implementations (Skill, Task, agentdef loader, etc.). | `SkillTool`, `TaskTool`, `AgentDef`, `LoadAgentDefs`, new `LoadAgentDefsFromPaths` |
| **cmd/buildmax** | CLI entry, wiring. Calls config helpers and passes results to tool constructors. | `setupAgentAndSession()` |

## Structure

**Directory / files**

- `internal/config/` — application configuration
  - `config.go` — existing `LLM`, `LoadLLM()`, `DataDir()`; add `SkillSearchPaths()` and `AgentDefsSearchPaths()`
  - `config_test.go` — existing `DataDir` tests; add tests for the new path helpers
- `internal/tools/` — tool implementations
  - `agentdef.go` — existing `AgentDef`, `LoadAgentDefs(dir)`; add `LoadAgentDefsFromPaths(dirs)`
  - `agentdef_test.go` — existing tests; add tests for `LoadAgentDefsFromPaths`
- `cmd/buildmax/` — CLI wiring
  - `root.go` — modify `setupAgentAndSession()` to call new config helpers

**Main types and interfaces**

No new types or interfaces. The change is purely functional helpers and a multi-dir loader wrapper.

## Method design

| Receiver | Method | Signature | Responsibility |
|----------|--------|-----------|----------------|
| *(package func)* | `SkillSearchPaths` | `(workspace string) []string` | Return ordered skill search dirs: project `.buildmax/skills`, compat `.cursor/skills`, global `DataDir()/skills`. |
| *(package func)* | `AgentDefsSearchPaths` | `(workspace string) []string` | Return ordered agent-def search dirs: project `.buildmax/agents`, global `DataDir()/agents`. |
| *(package func)* | `LoadAgentDefsFromPaths` | `(dirs []string) ([]AgentDef, error)` | Iterate dirs in order, call `LoadAgentDefs` for each, merge with first-dir-wins on name conflict, return sorted. |

## How they work together

**Data/control flow**

1. `setupAgentAndSession()` in `root.go` calls `config.SkillSearchPaths(cwd)` → receives `[]string` of 3 paths.
2. Passes the result to `tools.NewSkill(searchPaths)` — no change to `NewSkill` or `discoverSkills`.
3. `setupAgentAndSession()` calls `config.AgentDefsSearchPaths(cwd)` → receives `[]string` of 2 paths.
4. Passes the result to `tools.LoadAgentDefsFromPaths(dirs)`.
5. `LoadAgentDefsFromPaths` iterates each dir, calls existing `LoadAgentDefs(dir)` per dir, collects results into a `seen` map (first-dir-wins by `Name`), and returns the merged, alphabetically sorted slice.
6. The rest of `setupAgentAndSession()` proceeds as before — iterating defs and resolving tool names.

**Dependencies**

- `cmd/buildmax` depends on `internal/config` for `SkillSearchPaths` and `AgentDefsSearchPaths`.
- `cmd/buildmax` depends on `internal/tools` for `LoadAgentDefsFromPaths`.
- `internal/tools.LoadAgentDefsFromPaths` calls `internal/tools.LoadAgentDefs` (same package).
- `internal/config.SkillSearchPaths` and `AgentDefsSearchPaths` call `config.DataDir()` (same package).

**Key data structures**

- `[]string` (search paths): Created by config helpers, consumed by `NewSkill` and `LoadAgentDefsFromPaths`. Ordered by priority (first = highest).
- `[]AgentDef` (agent definitions): Produced by `LoadAgentDefsFromPaths`, consumed by `setupAgentAndSession` to build `agentTypes` map. Unchanged struct.

### `SkillSearchPaths` implementation

```go
func SkillSearchPaths(workspace string) []string {
    return []string{
        filepath.Join(workspace, ".buildmax", "skills"),
        filepath.Join(workspace, ".cursor", "skills"),
        filepath.Join(DataDir(), "skills"),
    }
}
```

### `AgentDefsSearchPaths` implementation

```go
func AgentDefsSearchPaths(workspace string) []string {
    return []string{
        filepath.Join(workspace, ".buildmax", "agents"),
        filepath.Join(DataDir(), "agents"),
    }
}
```

### `LoadAgentDefsFromPaths` implementation

```go
func LoadAgentDefsFromPaths(dirs []string) ([]AgentDef, error) {
    seen := make(map[string]bool)
    var merged []AgentDef

    for _, dir := range dirs {
        defs, err := LoadAgentDefs(dir)
        if err != nil {
            return nil, err
        }
        for _, d := range defs {
            if seen[d.Name] {
                slog.Debug("skip duplicate agent def", "name", d.Name, "dir", dir)
                continue
            }
            seen[d.Name] = true
            merged = append(merged, d)
        }
    }

    sort.Slice(merged, func(i, j int) bool {
        return merged[i].Name < merged[j].Name
    })
    return merged, nil
}
```

### `root.go` changes (before → after)

**Before:**
```go
skillSearchPaths := []string{
    filepath.Join(cwd, ".buildmax", "skills"),
    filepath.Join(cwd, ".cursor", "skills"),
    filepath.Join(config.DataDir(), "skills"),
}
skillTool, err := tools.NewSkill(skillSearchPaths)
// ...
agentDefsDir := filepath.Join(cwd, ".agents", "agents")
defs, err := tools.LoadAgentDefs(agentDefsDir)
```

**After:**
```go
skillTool, err := tools.NewSkill(config.SkillSearchPaths(cwd))
// ...
defs, err := tools.LoadAgentDefsFromPaths(config.AgentDefsSearchPaths(cwd))
```

## Changes for review

- **New**: `internal/config.SkillSearchPaths(workspace string) []string` — returns ordered skill search directories.
- **New**: `internal/config.AgentDefsSearchPaths(workspace string) []string` — returns ordered agent-def search directories.
- **New**: `internal/tools.LoadAgentDefsFromPaths(dirs []string) ([]AgentDef, error)` — multi-directory loader with first-dir-wins dedup.
- **New**: Tests in `internal/config/config_test.go` for `SkillSearchPaths` and `AgentDefsSearchPaths`.
- **New**: Tests in `internal/tools/agentdef_test.go` for `LoadAgentDefsFromPaths`.
- **Modified**: `cmd/buildmax/root.go` `setupAgentAndSession()` — replace inline path construction with calls to config helpers; replace `LoadAgentDefs` with `LoadAgentDefsFromPaths`; remove old `.agents/agents` path.
