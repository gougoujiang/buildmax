# Design 020 - Edit file tool

## Goal

Define the structure and APIs for an agent tool **Edit** that performs exact string replacements in files under a configurable root, enabling the LLM to modify existing files by replacing content, inserting text, or making targeted edits without overwriting entire files.

## Modules

| Module (package) | Responsibility | Owns |
|------------------|----------------|------|
| **internal/tools** | Concrete agent tools (Read, Write, **Edit**). Path resolution under root, `agent.Tool` implementation, string replacement logic. | `readfile.go`, `writefile.go` (existing); **new** `editfile.go`, `editfile_test.go` |
| **internal/agent** | Agent loop, `Tool` interface, tool invocation. | `agent.go` (unchanged) |
| **cmd/buildmax** | CLI entry, agent/session setup, tool construction. | `root.go` (wiring only) |

## Structure

**Directory / files**

- `internal/tools/` — agent tools
  - `readfile.go` — Read file tool (existing)
  - `writefile.go` — Write file tool (existing)
  - **`editfile.go`** — Edit file tool: `EditFile` type, `NewEditFile`, `Tool` implementation with string replacement and uniqueness checking
  - **`editfile_test.go`** — Unit tests for EditFile

- `cmd/buildmax/` — CLI
  - `root.go` — **Edit** `setupAgentAndSession`: add `NewEditFile(cwd)`, pass `editFileTool` to `NewAgent`

**Main types and interfaces**

- **EditFile** (internal/tools): Tool that performs exact string replacements in a file under a root. Holds `root string` (absolute path). Implements `agent.Tool` (Name, Description, Parameters, Execute). Path resolution mirrors ReadFile/WriteFile: join root + path, clean, absolutize, then ensure resolved path is under root via `filepath.Rel` and reject if `rel == ".."` or `strings.HasPrefix(rel, "..")`. Reads file, counts occurrences of `old_string`, validates uniqueness when `replace_all=false`, performs replacement(s), writes modified content back.
- **Tool** (internal/agent): Unchanged. `Name()`, `Description()`, `Parameters() any`, `Execute(ctx, args) (string, error)`.

## Method design

| Receiver   | Method       | Signature | Responsibility |
|-----------|--------------|-----------|-----------------|
| (package) | **NewEditFile** | `(root string) (*EditFile, error)` | If root is empty, use `os.Getwd()`. Absolutize and clean root; return `&EditFile{root: ...}` or error. Same pattern as `NewReadFile` and `NewWriteFile`. |
| **EditFile** | **Name** | `() string` | Return `"Edit"`. |
| **EditFile** | **Description** | `() string` | One or two sentences: performs exact string replacements in files; must read file first; preserves indentation; fails if old_string is not unique (unless replace_all=true). |
| **EditFile** | **Parameters** | `() any` | JSON schema: `type: "object"`, `properties`: `file_path` (string, required), `old_string` (string, required), `new_string` (string, required), `replace_all` (boolean, optional, default false). `required`: `["file_path", "old_string", "new_string"]`. Snake_case keys. |
| **EditFile** | **Execute** | `(ctx context.Context, args map[string]any) (string, error)` | Extract and validate `file_path`, `old_string`, `new_string` (all required, non-empty strings); extract `replace_all` (boolean, default false). Resolve path: `filepath.Join(e.root, filePath)` → `filepath.Abs(filepath.Clean(...))`; ensure under root with `filepath.Rel(e.root, resolved)` and reject `..` prefix. Verify file exists and is not a directory via `os.Stat`. Read file content with `os.ReadFile`. Count occurrences of `old_string` using `strings.Count`. If `replace_all=false` and count != 1, return error (0: "old_string not found"; >1: "old_string is not unique; use replace_all=true to replace all occurrences or provide more context to make old_string unique"). Perform replacement: if `replace_all=true`, use `strings.ReplaceAll(content, old_string, new_string)`; else use `strings.Replace(content, old_string, new_string, 1)`. Write modified content back with `os.WriteFile(resolved, []byte(modified), 0644)`. Return success message (e.g. "File edited successfully. Replaced 1 occurrence." or "File edited successfully. Replaced N occurrences.") or error. |

Path-under-root check (reuse ReadFile/WriteFile logic):

- `rel, err := filepath.Rel(e.root, resolved)`; if `err != nil` or `rel == ".."` or `strings.HasPrefix(rel, "..")` → return `errors.New("path outside allowed root")`.
- On Windows, also check: `cleanRoot := filepath.Clean(e.root)`; `resolvedClean := filepath.Clean(resolved)`; if `resolvedClean != cleanRoot && !strings.HasPrefix(resolvedClean, cleanRoot+string(filepath.Separator))` → return error (same as WriteFile).

File validation:

- After path resolution, `os.Stat(resolved)` must succeed and `info.IsDir()` must be false; else return `errors.New("path is a directory, not a file")` or `errors.New("file not found")`.

Uniqueness validation:

- Count occurrences: `count := strings.Count(content, old_string)`.
- If `replace_all=false`:
  - If `count == 0`: return `errors.New("old_string not found")`.
  - If `count > 1`: return `errors.New("old_string is not unique; use replace_all=true to replace all occurrences or provide more context to make old_string unique")`.
- If `replace_all=true`: proceed regardless of count (0 occurrences is acceptable; no-op).

Replacement logic:

- Use `strings.Replace` or `strings.ReplaceAll` from Go standard library. No regex or line-number awareness in this task.

## How they work together

**Data/control flow**

1. **Setup**: `setupAgentAndSession` in `root.go` gets `cwd`, creates `readFileTool`, `writeFileTool`, and **editFileTool** with the same root, passes `[]agent.Tool{readFileTool, writeFileTool, webFetchTool, todoWriteTool, bashTool, globTool, editFileTool}` to `agent.NewAgent`. Agent builds `toolDefs` and `toolsByName` including `"Edit"`.
2. **Agent loop** (unchanged): User message → `Process` / `ProcessAfterUserAppended` → `processLoop` → `ChatWithTools(messages, toolDefs)` → LLM may return a tool_call with name `"Edit"` and arguments `{"file_path": "...", "old_string": "...", "new_string": "...", "replace_all": false}`.
3. **Tool execution**: `processOneToolCall` looks up `a.toolsByName["Edit"]`, unmarshals arguments, calls `tool.Execute(ctx, args)`. EditFile resolves path under root, verifies file exists and is not a directory, reads file content, counts occurrences of `old_string`, validates uniqueness if `replace_all=false`, performs replacement(s), writes modified content back, returns success string or error. Result is appended as tool-role message; loop continues or returns final reply.

**Dependencies**

- **internal/tools** depends on **internal/agent** only for the `Tool` interface (via method implementation). No dependency from agent to tools except at construction in cmd.
- **cmd/buildmax** imports **internal/tools** and **internal/agent**; constructs concrete tools and passes them to the agent.

**Key data structures**

- **args** for Edit: `map[string]any` with `file_path` (string), `old_string` (string), `new_string` (string), `replace_all` (boolean, optional). Produced by LLM JSON, consumed by `EditFile.Execute`.
- **EditFile.root**: Set once by `NewEditFile`; used in `Execute` for path resolution and under-root check.
- **File content**: Read as `[]byte` via `os.ReadFile`, converted to `string` for `strings.Count` and `strings.Replace`/`strings.ReplaceAll`, converted back to `[]byte` for `os.WriteFile`.

## Changes for review

- **New**: `internal/tools/editfile.go` — `EditFile` struct with `root string`; `NewEditFile(root string) (*EditFile, error)`; `Name()`, `Description()`, `Parameters()`, `Execute()` implementing `agent.Tool`. Path resolution and under-root check aligned with ReadFile/WriteFile; file existence and directory validation; occurrence counting and uniqueness validation when `replace_all=false`; string replacement using `strings.Replace`/`strings.ReplaceAll`; write modified content back; return success message with replacement count or error.
- **New**: `internal/tools/editfile_test.go` — Unit tests: edit existing file (single replacement, replace_all=false) succeeds; edit existing file (all replacements, replace_all=true) succeeds; file not found returns error; path outside root rejected; path traversal (`../outside`) rejected; missing or empty `file_path`/`old_string`/`new_string` error; `old_string` not found (0 occurrences) returns error when replace_all=false; `old_string` not unique (multiple occurrences) returns error when replace_all=false; `old_string` not unique succeeds when replace_all=true; target is directory returns error; path resolution under root works correctly. Use `t.TempDir()` as root.
- **Modified**: `cmd/buildmax/root.go` — In `setupAgentAndSession`, after `NewWriteFile(cwd)`: call `editFileTool, err := tools.NewEditFile(cwd)`; on error log and return; pass `editFileTool` to `agent.NewAgent` in the tools slice: `[]agent.Tool{readFileTool, writeFileTool, webFetchTool, todoWriteTool, bashTool, globTool, editFileTool}`.
