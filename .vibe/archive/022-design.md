# Design 022 - Task tool

## Goal

Define the structure and APIs for an agent tool **Task** that lets the LLM spawn a sub-agent to handle complex subtasks autonomously, supporting both built-in agent types (general, explore, shell) defined in code and user-defined agent types loaded from `<workspace>/.agents/agents/` files with YAML frontmatter + system prompt body.

## Modules

| Module (package) | Responsibility | Owns |
|------------------|----------------|------|
| **internal/agent** | Agent loop, `Tool` interface, `LLMCaller` interface. Injects session ID into context. **Add** `SystemPrompt` option. | `agent.go` (**edit**: add `systemPrompt` field, `SystemPrompt` option, use field in `processLoop`) |
| **internal/tools** | Concrete agent tools. Agent definition loading. Task tool. | `todowrite.go`, `readfile.go`, etc. (existing, unchanged); **new** `agentdef.go`, `agentdef_test.go`, `task.go`, `task_test.go` |
| **internal/session** | Session identity, context helpers, persistence. | `session.go` (unchanged) |
| **internal/llm** | LLM client, message types. | `client.go`, `types.go` (unchanged) |
| **cmd/buildmax** | CLI entry, agent/session setup, tool construction, agent type wiring. | `root.go` (**edit**: build tool-by-name map, create built-in + user-defined agent type configs, create TaskTool, pass to NewAgent) |

## Structure

**Directory / files**

- `internal/agent/` — agent loop
  - `agent.go` — **Edit**: add `systemPrompt string` field to `Agent` struct (default `DefaultSystemPrompt`); add `SystemPrompt(string) Option`; in `processLoop` use `a.systemPrompt` instead of the hardcoded constant.
  - `agent_test.go` — **Edit**: add test for `SystemPrompt` option override.

- `internal/tools/` — agent tools
  - **`agentdef.go`** — `AgentDef` struct, `LoadAgentDefs(dir) ([]AgentDef, error)` — parses agent definition files with frontmatter + body. No external YAML dependency; uses a simple manual parser (split on `---`, parse `key: value` lines).
  - **`agentdef_test.go`** — Unit tests for `LoadAgentDefs`.
  - **`task.go`** — `AgentTypeConfig` struct, `TaskTool` struct, `NewTask`, `Name`, `Description`, `Parameters`, `Execute` implementing `agent.Tool`. Dynamic description and enum from registered agent types.
  - **`task_test.go`** — Unit tests for `TaskTool` using a mock `LLMCaller`.

- `cmd/buildmax/` — CLI
  - `root.go` — **Edit** `setupAgentAndSession`: build `toolsByName` map, create built-in agent type configs, call `LoadAgentDefs`, resolve tool names, merge configs, create `TaskTool`, pass in tool slice.

**Main types and interfaces**

- **AgentDef** (internal/tools, exported): Parsed representation of one user-defined agent file. Fields: `Name`, `Description`, `ToolNames []string`, `SystemPrompt`, `Model`, `Color`. Created by `LoadAgentDefs`, consumed by wiring code in `root.go` to build `AgentTypeConfig` entries.
- **AgentTypeConfig** (internal/tools, exported): Configuration for one agent type. Fields: `Tools []agent.Tool`, `SystemPrompt string`, `Description string`. Used by both built-in (constructed in `root.go`) and user-defined (resolved from `AgentDef`) types. Passed to `NewTask` in the `agentTypes` map.
- **TaskTool** (internal/tools): Tool that spawns sub-agents. Holds `caller agent.LLMCaller` and `agentTypes map[string]AgentTypeConfig` plus `typeOrder []string` (for deterministic description/enum ordering). Implements `agent.Tool`. On Execute: validates args, looks up type config, creates ephemeral session and Agent with `SystemPrompt` option, calls `Process`, returns the reply.
- **Agent.systemPrompt** (internal/agent): New field on `Agent` struct. Defaults to `DefaultSystemPrompt` in `NewAgent`. Overridden by `SystemPrompt(string) Option`. Used in `processLoop` when building the messages slice.
- **Tool** (internal/agent): Unchanged. `Name()`, `Description()`, `Parameters() any`, `Execute(ctx, args) (string, error)`.
- **LLMCaller** (internal/agent): Unchanged. `ChatWithTools(ctx, messages, tools) (content, toolCalls, err)`.

## Method design

### internal/agent (edit agent.go)

| Receiver | Method | Signature | Responsibility |
|----------|--------|-----------|----------------|
| (package) | **SystemPrompt** | `(prompt string) Option` | Return an `Option` that sets `a.systemPrompt = prompt`. |

**Struct change**: Add `systemPrompt string` to `Agent`. In `NewAgent`, initialize `a.systemPrompt = DefaultSystemPrompt` before applying options.

**processLoop change**: Line 114 currently has:
```go
messages := append([]llm.Message{{Role: "system", Content: DefaultSystemPrompt}}, sess.Messages()...)
```
Change to:
```go
messages := append([]llm.Message{{Role: "system", Content: a.systemPrompt}}, sess.Messages()...)
```

### internal/tools/agentdef.go (new)

| Receiver | Method | Signature | Responsibility |
|----------|--------|-----------|----------------|
| (package) | **LoadAgentDefs** | `(dir string) ([]AgentDef, error)` | Read all files from `dir`. For each file, call `parseAgentDef`. If `dir` does not exist (`os.IsNotExist`), return `(nil, nil)`. On `ReadDir` error, return the error. Individual files that fail to parse are skipped with `slog.Warn`. Return valid defs sorted alphabetically by `Name`. |
| (package) | **parseAgentDef** | `(data []byte) (AgentDef, error)` | Private. Split content on `---` delimiters. Extract frontmatter key-value pairs. Extract body (system prompt). Validate required fields. Return `AgentDef` or error. |
| (package) | **parseFrontmatter** | `(block string) map[string]string` | Private. Split the frontmatter block into lines. For each line containing `:`, split into key and value, trim both. Return `map[string]string`. |

**AgentDef struct**:
```go
type AgentDef struct {
    Name         string
    Description  string
    ToolNames    []string
    SystemPrompt string
    Model        string
    Color        string
}
```

**File format parsing logic** (`parseAgentDef`):
1. Convert `data` to string, trim leading/trailing whitespace.
2. Content must start with `---`. Find the second `---` to delimit frontmatter.
3. Split: `frontmatter` = text between first and second `---`; `body` = text after second `---`, trimmed.
4. Parse frontmatter as key-value: call `parseFrontmatter(frontmatter)` → `map[string]string`.
5. Extract fields:
   - `name` (required): `kv["name"]`, trimmed. Error if empty.
   - `description` (required): `kv["description"]`, trimmed. Error if empty.
   - `tools` (required): `kv["tools"]`, split by `,`, trim each. Error if no entries after trimming.
   - `model` (optional): `kv["model"]`, trimmed.
   - `color` (optional): `kv["color"]`, trimmed.
6. `SystemPrompt` = body. If body is empty, use `description` as the system prompt (fallback so sub-agent always has a prompt).
7. Return `AgentDef`.

**parseFrontmatter logic**:
- Split block on `\n`.
- For each line: find first `:`, split into key (before `:`) and value (after `:`), trim both.
- Skip lines without `:` or empty keys.
- Return map.

This avoids adding `gopkg.in/yaml.v3` as a dependency. The frontmatter fields are all simple strings, so manual parsing is sufficient and aligned with the project's minimal-dependency principle.

### internal/tools/task.go (new)

| Receiver | Method | Signature | Responsibility |
|----------|--------|-----------|----------------|
| (package) | **NewTask** | `(caller agent.LLMCaller, agentTypes map[string]AgentTypeConfig) (*TaskTool, error)` | Validate: `caller` non-nil, `agentTypes` non-empty. Build `typeOrder []string` — collect keys, sort built-in first (`general`, `explore`, `shell` in that order), then remaining keys alphabetically. Store fields. Return `*TaskTool`. |
| **TaskTool** | **Name** | `() string` | Return `"Task"`. |
| **TaskTool** | **Description** | `() string` | Build description dynamically from `typeOrder` and `agentTypes`. See format below. |
| **TaskTool** | **Parameters** | `() any` | Build JSON schema dynamically. `subagent_type` enum from `typeOrder`. See format below. |
| **TaskTool** | **Execute** | `(ctx context.Context, args map[string]any) (string, error)` | Parse and validate args. Create ephemeral session and sub-agent. Call `Process`. Return reply. See detailed flow below. |

**AgentTypeConfig struct** (exported):
```go
type AgentTypeConfig struct {
    Tools        []agent.Tool
    SystemPrompt string
    Description  string
}
```

**TaskTool struct**:
```go
type TaskTool struct {
    caller     agent.LLMCaller
    agentTypes map[string]AgentTypeConfig
    typeOrder  []string // for deterministic description/enum ordering
}
```

**`typeOrder` construction** (in `NewTask`):
1. Collect all keys from `agentTypes`.
2. Separate into two lists: built-in names (`general`, `explore`, `shell`) in that fixed order, and remaining names sorted alphabetically.
3. Concatenate: built-in first, then user-defined. This ensures the LLM always sees built-in types listed first.

**Description() format**:
```
Launch a sub-agent to handle a complex subtask autonomously. The sub-agent processes the prompt using its own session and tools, then returns the final reply. Available agent types:
- general: General-purpose agent with all tools for multi-step tasks.
- explore: Read-only agent for fast codebase exploration (Read, Glob, Grep).
- shell: Command execution specialist (Bash only).
- code-architect: Designs feature architectures by analyzing existing codebase patterns.
```
Built from iterating `typeOrder` and formatting `"- %s: %s"` with name and `Description` from the config.

**Parameters() schema**:
```go
map[string]any{
    "type": "object",
    "properties": map[string]any{
        "description": map[string]any{
            "type":        "string",
            "description": "Short 3-5 word summary of the task",
        },
        "prompt": map[string]any{
            "type":        "string",
            "description": "Detailed task description for the sub-agent. Be specific about what you want the sub-agent to do and what information to return.",
        },
        "subagent_type": map[string]any{
            "type":        "string",
            "description": "The type of sub-agent to use",
            "enum":        typeNames, // []string built from typeOrder
        },
    },
    "required": []string{"description", "prompt", "subagent_type"},
}
```

**Execute() detailed flow**:
1. Extract `description` from args. Must be a non-empty string. Error: `"description is required"`.
2. Extract `prompt` from args. Must be a non-empty string. Error: `"prompt is required"`.
3. Extract `subagent_type` from args. Must be a non-empty string and a key in `agentTypes`. Error: `"unknown subagent_type %q; available types: ..."`.
4. Look up `config := t.agentTypes[subagentType]`.
5. Create ephemeral session: `sess := session.NewSession(description)`.
6. Create sub-agent: `subAgent := agent.NewAgent(t.caller, config.Tools, agent.SystemPrompt(config.SystemPrompt))`.
7. Log: `slog.Info("task: spawning sub-agent", "type", subagentType, "description", description)`.
8. Call: `reply, err := subAgent.Process(ctx, sess, prompt)`.
9. If `err != nil`: return `"", fmt.Errorf("sub-agent failed: %w", err)`.
10. Log: `slog.Info("task: sub-agent completed", "type", subagentType, "reply_len", len(reply))`.
11. Return `reply, nil`.

### internal/agent/agent_test.go (edit)

| Test | What it verifies |
|------|-----------------|
| **TestSystemPromptOption** | Create agent with `SystemPrompt("Custom prompt")`. Use `recordingLLMCaller`. Call `Process`. Verify `firstMsg[0].Content == "Custom prompt"`. |
| **TestSystemPromptDefault** | Create agent without `SystemPrompt` option. Use `recordingLLMCaller`. Call `Process`. Verify `firstMsg[0].Content == DefaultSystemPrompt`. (Already covered by existing `TestProcessWithSession_SystemPromptPrepend`, but can add explicitly for clarity.) |

### internal/tools/agentdef_test.go (new)

| Test | What it verifies |
|------|-----------------|
| **TestLoadAgentDefs_ValidFile** | Create temp dir with one valid agent file. Call `LoadAgentDefs`. Verify `AgentDef` fields: Name, Description, ToolNames, SystemPrompt, Model, Color. |
| **TestLoadAgentDefs_MissingName** | File with no `name` field. Verify file is skipped, no error returned. |
| **TestLoadAgentDefs_MissingDescription** | File with no `description` field. Verify file is skipped. |
| **TestLoadAgentDefs_NonExistentDir** | Non-existent directory path. Verify returns `(nil, nil)`. |
| **TestLoadAgentDefs_MultipleFiles** | Two valid files. Verify two `AgentDef` entries returned, sorted by name. |
| **TestLoadAgentDefs_ToolSplitting** | File with `tools: Glob, Grep,  Read`. Verify `ToolNames = ["Glob", "Grep", "Read"]` (trimmed). |
| **TestLoadAgentDefs_BodyExtraction** | File with multi-line body. Verify `SystemPrompt` matches the body content, trimmed. |
| **TestLoadAgentDefs_EmptyBody** | File with empty body. Verify `SystemPrompt` falls back to `Description`. |
| **TestParseAgentDef_NoFrontmatter** | Content without `---` delimiters. Verify error. |

### internal/tools/task_test.go (new)

| Test | What it verifies |
|------|-----------------|
| **TestNewTask_NilCaller** | `NewTask(nil, types)` returns error. |
| **TestNewTask_EmptyTypes** | `NewTask(caller, empty)` returns error. |
| **TestTaskTool_Name** | `Name()` returns `"Task"`. |
| **TestTaskTool_Description** | `Description()` contains all registered type names and descriptions. |
| **TestTaskTool_Parameters** | `Parameters()` schema has `subagent_type` enum with all type names. |
| **TestTaskTool_InterfaceCompliance** | `var _ agent.Tool = (*TaskTool)(nil)` compiles. |
| **TestTaskTool_Execute_UnknownType** | Execute with `subagent_type: "nonexistent"` returns error. |
| **TestTaskTool_Execute_MissingPrompt** | Execute with empty/missing `prompt` returns error. |
| **TestTaskTool_Execute_MissingDescription** | Execute with empty/missing `description` returns error. |
| **TestTaskTool_Execute_BuiltinType** | Execute with a built-in type name. Mock LLMCaller returns canned reply. Verify reply is returned. |
| **TestTaskTool_Execute_UserDefinedType** | Execute with a user-defined type name. Mock LLMCaller returns canned reply. Verify reply is returned. |

**Mock LLMCaller** for task tests (in `task_test.go`, separate from agent's internal mock):
```go
type mockCaller struct {
    content string
}

func (m *mockCaller) ChatWithTools(ctx context.Context, messages []llm.Message, tools []llm.ToolDef) (string, []llm.ToolCall, error) {
    return m.content, nil, nil
}
```
Returns a fixed reply with no tool calls, so the sub-agent completes in one iteration.

## How they work together

**Data/control flow**

### Startup (wiring in `setupAgentAndSession`)

1. Create all base tools as before: `readFileTool`, `writeFileTool`, `webFetchTool`, `todoWriteTool`, `bashTool`, `globTool`, `editFileTool`, `grepTool`.
2. Build `toolsByName map[string]agent.Tool` from the base tools (key = `tool.Name()`, e.g. `"Read"`, `"WriteFile"`, `"Bash"`, etc.). This map is used for resolving user-defined agent tool lists.
3. Build built-in agent type configs:
   ```
   agentTypes["general"] = AgentTypeConfig{
       Tools:        []agent.Tool{readFileTool, writeFileTool, webFetchTool, todoWriteTool, bashTool, globTool, editFileTool, grepTool},
       SystemPrompt: generalSubAgentPrompt,
       Description:  "General-purpose agent with all tools for multi-step tasks.",
   }
   agentTypes["explore"] = AgentTypeConfig{
       Tools:        []agent.Tool{readFileTool, globTool, grepTool},
       SystemPrompt: exploreSubAgentPrompt,
       Description:  "Read-only agent for fast codebase exploration (Read, Glob, Grep).",
   }
   agentTypes["shell"] = AgentTypeConfig{
       Tools:        []agent.Tool{bashTool},
       SystemPrompt: shellSubAgentPrompt,
       Description:  "Command execution specialist (Bash only).",
   }
   ```
4. Load user-defined agent definitions: `defs, err := tools.LoadAgentDefs(filepath.Join(cwd, ".agents", "agents"))`. On error, log warning and continue (do not fail startup). If `defs` is nil, no user-defined agents.
5. For each `AgentDef`:
   - If `def.Name` matches a built-in key (`general`, `explore`, `shell`), log warning and skip.
   - Resolve `def.ToolNames` to `[]agent.Tool` via `toolsByName`. For each unknown name, log warning and skip that tool. If no tools resolved, log warning and skip the entire agent def.
   - Add to `agentTypes[def.Name] = AgentTypeConfig{Tools: resolved, SystemPrompt: def.SystemPrompt, Description: def.Description}`.
6. Create task tool: `taskTool, err := tools.NewTask(client, agentTypes)`. On error, log and return.
7. Pass all tools including `taskTool` to parent agent: `agent.NewAgent(client, []agent.Tool{readFileTool, ..., grepTool, taskTool})`.

### Runtime (Execute flow)

1. **LLM returns tool_call**: Parent agent loop receives tool_call with `name: "Task"`, `arguments: {"description": "Explore auth code", "prompt": "Find all authentication...", "subagent_type": "explore"}`.
2. **Agent dispatches**: `processOneToolCall` looks up `"Task"` in `toolsByName`, parses JSON args, calls `TaskTool.Execute(ctx, args)`.
3. **TaskTool.Execute**:
   - Validates args (description, prompt, subagent_type).
   - Looks up `config = agentTypes["explore"]`.
   - Creates ephemeral session: `sess = session.NewSession("Explore auth code")`.
   - Creates sub-agent: `subAgent = agent.NewAgent(t.caller, config.Tools, agent.SystemPrompt(config.SystemPrompt))`.
   - The sub-agent has `Tools = [Read, Glob, Grep]` and `systemPrompt = exploreSubAgentPrompt`.
   - Calls `subAgent.Process(ctx, sess, "Find all authentication...")`.
4. **Sub-agent loop**: The sub-agent runs its own `processLoop` with its own tools and system prompt. It may make tool calls (Read, Glob, Grep) across multiple iterations. Eventually returns a final reply string.
5. **Result**: `TaskTool.Execute` returns the sub-agent's reply. The parent agent appends it as a tool-role message. The parent loop continues or returns the final reply to the user.

### User-defined agent file loading (LoadAgentDefs)

1. Check if `dir` exists. If not (`os.IsNotExist`), return `(nil, nil)`.
2. `os.ReadDir(dir)` to list entries.
3. For each regular file:
   - Read file content with `os.ReadFile`.
   - Call `parseAgentDef(data)`. On error, `slog.Warn` and skip.
   - Append to results.
4. Sort results by `Name` (alphabetical).
5. Return results.

**Dependencies**

- **internal/agent** — no new imports. Depends on `internal/llm` and `internal/session` (same as before). The `SystemPrompt` option is self-contained.
- **internal/tools** (`agentdef.go`) — depends on `os`, `path/filepath`, `strings`, `sort`, `log/slog` (all stdlib). No dependency on `internal/agent`.
- **internal/tools** (`task.go`) — depends on `internal/agent` (for `Tool`, `LLMCaller`, `NewAgent`, `SystemPrompt`), `internal/session` (for `NewSession`), `internal/llm` (for `Message`). Same dependency direction as existing tools.
- **cmd/buildmax** — depends on `internal/agent`, `internal/tools`, `internal/session`, `internal/llm`, `internal/config` (same as before, plus uses `tools.LoadAgentDefs` and `tools.NewTask`).

No new external dependencies. The frontmatter parser is manual (stdlib only).

**Key data structures**

- **AgentDef**: `{Name, Description, ToolNames, SystemPrompt, Model, Color}`. Created by `LoadAgentDefs` from file content. Consumed by wiring code to build `AgentTypeConfig`. `ToolNames` are raw strings — not yet resolved to tool instances.
- **AgentTypeConfig**: `{Tools []agent.Tool, SystemPrompt string, Description string}`. Created by wiring code (built-in) or resolved from `AgentDef` (user-defined). Consumed by `TaskTool` for spawning sub-agents.
- **agentTypes map**: `map[string]AgentTypeConfig` keyed by agent type name. Held by `TaskTool`. Used in `Execute` to look up config, in `Description` to list types, in `Parameters` to build enum.
- **typeOrder []string**: Ordered list of type names. Held by `TaskTool`. Ensures deterministic output in `Description()` and `Parameters()`. Built-in names first, user-defined names alphabetically after.
- **args for Task**: `map[string]any` with `description` (string), `prompt` (string), `subagent_type` (string). Produced by LLM JSON, consumed by `TaskTool.Execute`.

**Built-in system prompts** (defined as constants or variables in `root.go` or `task.go`):

```go
const generalSubAgentPrompt = `You are a general-purpose AI assistant sub-agent. You have access to tools for reading, writing, editing files, running commands, and more. Complete the task described in the user message thoroughly and return a clear, concise result.`

const exploreSubAgentPrompt = `You are a code exploration sub-agent. You have read-only access to the codebase via Read, Glob, and Grep tools. Explore the codebase to answer the question or find the information requested. Return a clear, organized summary of your findings.`

const shellSubAgentPrompt = `You are a command execution sub-agent. You have access to the Bash tool for running shell commands. Execute the requested commands and return the results. Be careful with destructive operations.`
```

These are defined in `task.go` as package-level constants so they are co-located with the TaskTool that uses them.

## Changes for review

- **Modified**: `internal/agent/agent.go` — Add `systemPrompt string` field to `Agent` struct; initialize to `DefaultSystemPrompt` in `NewAgent`; add `SystemPrompt(string) Option` function; in `processLoop` line 114, replace `DefaultSystemPrompt` with `a.systemPrompt`. No changes to `Tool`, `LLMCaller`, `Process`, `ProcessAfterUserAppended`, or `processOneToolCall`.
- **Modified**: `internal/agent/agent_test.go` — Add `TestSystemPromptOption`: create agent with `SystemPrompt("custom")`, use `recordingLLMCaller`, verify first message uses custom prompt.
- **New**: `internal/tools/agentdef.go` — `AgentDef` struct (Name, Description, ToolNames, SystemPrompt, Model, Color); `LoadAgentDefs(dir string) ([]AgentDef, error)` reads files from directory, parses YAML-like frontmatter manually, extracts body as system prompt, validates required fields, skips invalid files with warning, returns sorted defs; private helpers `parseAgentDef` and `parseFrontmatter`. No external dependencies.
- **New**: `internal/tools/agentdef_test.go` — Tests: valid file parsing, missing name/description skip, non-existent directory, multiple files, tool splitting, body extraction, empty body fallback, no frontmatter error.
- **New**: `internal/tools/task.go` — `AgentTypeConfig` struct (Tools, SystemPrompt, Description); `TaskTool` struct (caller, agentTypes, typeOrder); `NewTask(caller, agentTypes) (*TaskTool, error)` validates inputs and builds type order; `Name()` → `"Task"`; `Description()` dynamically lists all types; `Parameters()` dynamically builds enum; `Execute(ctx, args)` validates args, creates ephemeral session and sub-agent with `SystemPrompt` option, calls `Process`, returns reply. Built-in system prompt constants. No external dependencies.
- **New**: `internal/tools/task_test.go` — Tests: nil caller, empty types, name, description content, parameters enum, interface compliance, unknown type, missing prompt, missing description, valid execution with built-in type, valid execution with user-defined type. Uses `mockCaller` that returns fixed reply with no tool calls.
- **Modified**: `cmd/buildmax/root.go` — In `setupAgentAndSession`: build `toolsByName` map from base tools; create three built-in `AgentTypeConfig` entries; call `tools.LoadAgentDefs(filepath.Join(cwd, ".agents", "agents"))` and resolve tool names (skip conflicts, unknown names); merge into `agentTypes` map; create `taskTool` via `tools.NewTask(client, agentTypes)`; add `taskTool` to the tool slice passed to `NewAgent`. Update function doc comment.
