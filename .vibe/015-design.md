# Design 015 - Write new file

## Goal

Define the structure and APIs for an agent tool **Write** that writes UTF-8 content to a local file under a configurable root, so the LLM can create or overwrite files within the allowed workspace.

## Modules

| Module (package) | Responsibility | Owns |
|------------------|----------------|------|
| **internal/tools** | Concrete agent tools (Read, Write). Path resolution under root, `agent.Tool` implementation. | `readfile.go`, `readfile_test.go`; **new** `writefile.go`, `writefile_test.go` |
| **internal/agent** | Agent loop, `Tool` interface, tool invocation. | `agent.go` (unchanged) |
| **cmd/buildmax** | CLI entry, agent/session setup, tool construction. | `root.go` (wiring only) |

## Structure

**Directory / files**

- `internal/tools/` — agent tools
  - `readfile.go` — Read file tool (existing)
  - `readfile_test.go` — (existing)
  - **`writefile.go`** — Write file tool: `WriteFile` type, `NewWriteFile`, `Tool` implementation
  - **`writefile_test.go`** — Unit tests for WriteFile

- `cmd/buildmax/` — CLI
  - `root.go` — **Edit** `setupAgentAndSession`: add `NewWriteFile(cwd)`, pass both tools to `NewAgent`

**Main types and interfaces**

- **WriteFile** (internal/tools): Tool that writes content to a file under a root. Holds `root string` (absolute path). Implements `agent.Tool` (Name, Description, Parameters, Execute). Path resolution mirrors ReadFile: join root + path, clean, absolutize, then ensure resolved path is under root via `filepath.Rel` and reject if `rel == ".."` or `strings.HasPrefix(rel, "..")`.
- **Tool** (internal/agent): Unchanged. `Name()`, `Description()`, `Parameters() any`, `Execute(ctx, args) (string, error)`.

## Method design

| Receiver   | Method       | Signature | Responsibility |
|-----------|--------------|-----------|-----------------|
| (package) | **NewWriteFile** | `(root string) (*WriteFile, error)` | If root is empty, use `os.Getwd()`. Absolutize and clean root; return `&WriteFile{root: ...}` or error. Same pattern as `NewReadFile`. |
| **WriteFile** | **Name** | `() string` | Return `"Write"`. |
| **WriteFile** | **Description** | `() string` | One sentence: write or overwrite a local file with the given content; path must be under the allowed workspace. |
| **WriteFile** | **Parameters** | `() any` | JSON schema: `type: "object"`, `properties`: `file_path` (string), `content` (string), `required`: `["file_path", "content"]`. Snake_case keys. |
| **WriteFile** | **Execute** | `(ctx context.Context, args map[string]any) (string, error)` | Extract and validate `file_path` and `content` (both required, non-empty strings). Resolve path: `filepath.Join(w.root, filePath)` → `filepath.Abs(filepath.Clean(...))`; ensure under root with `filepath.Rel(w.root, resolved)` and reject `..` prefix. If resolved path exists, `os.Stat` must show a file (not a directory); else return error. Create parent dir with `os.MkdirAll(filepath.Dir(resolved), 0755)`. Write content with `os.WriteFile(resolved, []byte(content), 0644)`. Return e.g. `"Written N bytes to <resolved>"` or error. |

Path-under-root check (reuse ReadFile logic):

- `rel, err := filepath.Rel(w.root, resolved)`; if `err != nil` or `rel == ".."` or `strings.HasPrefix(rel, "..")` → return `errors.New("path outside allowed root")`.

Existing directory check:

- After path resolution, if `os.Stat(resolved)` succeeds and `info.IsDir()` → return `errors.New("path is a directory, not a file")`.

## How they work together

**Data/control flow**

1. **Setup** (unchanged except tool list): `setupAgentAndSession` in `root.go` gets `cwd`, creates `readFileTool` and **writeFileTool** with the same root, passes `[]agent.Tool{readFileTool, writeFileTool}` to `agent.NewAgent`. Agent builds `toolDefs` and `toolsByName` including `"Write"`.
2. **Agent loop** (unchanged): User message → `Process` / `ProcessAfterUserAppended` → `processLoop` → `ChatWithTools(messages, toolDefs)` → LLM may return a tool_call with name `"Write"` and arguments `{"file_path": "...", "content": "..."}`.
3. **Tool execution**: `processOneToolCall` looks up `a.toolsByName["Write"]`, unmarshals arguments, calls `tool.Execute(ctx, args)`. WriteFile resolves path under root, creates parent dir if needed, writes file, returns success string or error. Result is appended as tool-role message; loop continues or returns final reply.

**Dependencies**

- **internal/tools** depends on **internal/agent** only for the `Tool` interface (via method implementation). No dependency from agent to tools except at construction in cmd.
- **cmd/buildmax** imports **internal/tools** and **internal/agent**; constructs concrete tools and passes them to the agent.

**Key data structures**

- **args** for Write: `map[string]any` with `file_path` (string), `content` (string). Produced by LLM JSON, consumed by `WriteFile.Execute`.
- **WriteFile.root**: Set once by `NewWriteFile`; used in `Execute` for path resolution and under-root check.

## Changes for review

- **New**: `internal/tools/writefile.go` — `WriteFile` struct with `root string`; `NewWriteFile(root string) (*WriteFile, error)`; `Name()`, `Description()`, `Parameters()`, `Execute()` implementing `agent.Tool`. Path resolution and under-root check aligned with ReadFile; create parent dirs with `MkdirAll(0755)`; write with `os.WriteFile(0644)`; reject if target exists and is a directory.
- **New**: `internal/tools/writefile_test.go` — Unit tests: write new file under root; overwrite existing file; path outside root rejected; path traversal (`../outside`) rejected; missing or empty `file_path`/`content` error; parent directory auto-created; target is directory returns error. Use `t.TempDir()` as root.
- **Modified**: `cmd/buildmax/root.go` — In `setupAgentAndSession`, after `NewReadFile(cwd)`: call `writeFileTool, err := tools.NewWriteFile(cwd)`; on error log and return; pass `[]agent.Tool{readFileTool, writeFileTool}` to `NewAgent` instead of only `readFileTool`.
