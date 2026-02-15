# Design 005 - Read File Tool

## Goal

Define the structure and behavior of the first agent tool, **read_file**, so it reads a local file under a configurable root and returns its text contents for the LLM, with path safety and clear errors.

## Modules

| Module (package) | Responsibility | Owns |
|------------------|----------------|------|
| **internal/agent** | Agent loop, tool interface, execution of tool calls | `Tool` interface, `Agent`, `NewAgent`, `Process` |
| **internal/llm** | LLM client, tool definitions and tool-call responses | `ToolDef`, `ToolCall`, `Client.ChatWithTools` |
| **internal/tools** (new) | Concrete tools the agent can invoke | `ReadFile` type, `NewReadFile`, tool implementation |
| **cmd/buildmax** | CLI entry, prompt mode, agent construction | `main`, `runPromptMode`, wiring of LLM + tools |

## Structure

**Directory / files**

- `internal/tools/` — concrete agent tools (this task: read_file only)
  - `readfile.go` — ReadFile type, constructor, and `agent.Tool` implementation
  - `readfile_test.go` — unit tests for path resolution, success, not found, outside root
- `cmd/buildmax/` — no new files; `main.go` is modified to construct and pass the read_file tool

**Main types and interfaces**

- **ReadFile** (internal/tools): struct holding a single field `root string` (absolute path). Implements **agent.Tool** (Name, Description, Parameters, Execute). Created via **NewReadFile(root string)**. No other exported types in this task.
- **agent.Tool** (unchanged): `Name() string`, `Description() string`, `Parameters() any`, `Execute(ctx, args map[string]any) (string, error)`.

## Method design

| Receiver | Method | Signature | Responsibility |
|----------|--------|-----------|-----------------|
| **ReadFile** | Name | `() string` | Return `"read_file"`. |
| **ReadFile** | Description | `() string` | Return a short description so the LLM knows when to use it (e.g. read local files to see source code, config, or text). Use wording that encourages passing a path; absolute path can be documented in description if desired. |
| **ReadFile** | Parameters | `() any` | Return an OpenAI-style JSON schema (e.g. `map[string]any`) with `type: "object"`, `properties: { "file_path": { "type": "string", "description": "path to the file to read (absolute or relative to root)" } }`, `required: ["file_path"]`. Compatible with `llm.ToolDef.Parameters` passed to the API. |
| **ReadFile** | Execute | `(ctx context.Context, args map[string]any) (string, error)` | Read file at `args["file_path"]`; resolve path against `ReadFile.root`; ensure resolved path is under root; read entire file as UTF-8 text and return contents; on any error (not found, permission, is directory, outside root, invalid path) return clear error. |
| **(func)** | NewReadFile | `(root string) (*ReadFile, error)` | Normalize and absolutize `root` (e.g. `filepath.Abs`); if empty use `os.Getwd()`. Return a *ReadFile with that root, or error if root cannot be resolved. Caller (main) passes e.g. `os.Getwd()` or a configurable base path. |

**Path resolution and safety (inside Execute)**

1. Obtain `file_path` from `args["file_path"]`; if missing or not string, return error.
2. Join root and path: `filepath.Join(r.root, filePath)` then `filepath.Clean` and `filepath.Abs` to get resolved absolute path.
3. Ensure resolved path is under root: e.g. `filepath.Rel(r.root, resolved)` and check that the result does not start with `".."` (and is not `".."`). On Windows, compare normalized absolute paths (e.g. `filepath.ToSlash` and `strings.HasPrefix`) to avoid bypasses.
4. If not under root, return error (e.g. `errors.New("path outside allowed root")` or similar).
5. Open file; if `Stat` shows directory, return error; read contents (e.g. `os.ReadFile`); return string contents or error.

**Read behavior**

- Read entire file into memory (task scope: no streaming). Assume UTF-8; invalid UTF-8 can be replaced with a replacement character or return error—document choice in code.
- No optional offset/limit in this task (reference had offset/limit for large files; can be a follow-up).
- No line-number prefix in this task (reference had cat -n style; can be a follow-up). Return raw file contents.

## How they work together

**Data/control flow**

1. User runs prompt mode with a prompt like "read the file foo.go". `main.runPromptMode` builds `client` (LLM), `readFileTool := tools.NewReadFile(cwd)` (or equivalent), and `agent.NewAgent(client, []agent.Tool{readFileTool})`.
2. User message is sent to `agent.Process(ctx, userMessage)`. Agent sends messages and tool definitions (including read_file’s Name, Description, Parameters) to `client.ChatWithTools`.
3. LLM may return a tool call `{ Name: "read_file", Arguments: "{\"file_path\": \"/some/path/foo.go\"}" }`. Agent looks up tool by name, unmarshals Arguments into `map[string]any`, calls `readFileTool.Execute(ctx, args)`.
4. ReadFile resolves path against root; if allowed, reads file and returns content string; otherwise returns error. Agent appends tool result as a tool-role message and calls the LLM again.
5. When LLM returns no tool calls, agent returns final reply to main; main prints it.

**Dependencies**

- **internal/tools** depends on **internal/agent** (only the `Tool` interface; no dependency on agent’s concrete types) and stdlib (`os`, `path/filepath`, etc.). No dependency on **internal/llm** or **internal/config** for this task (root is passed by main).
- **cmd/buildmax** depends on **internal/agent**, **internal/llm**, **internal/config**, and **internal/tools**; it constructs the ReadFile tool and passes it into the agent.

**Key data structures**

- **args** in Execute: `map[string]any` with key `"file_path"` (string). LLM fills this from the JSON schema.
- **Tool result**: plain string (file contents or error message). Agent sends it as `llm.Message{ Role: "tool", Content: result, ToolCallID: tc.ID }`.

## Out of scope / future

- Optional **offset** and **limit** parameters (as in the reference) for reading a range of lines; full-file read is sufficient for this task.
- Line-number formatting (e.g. cat -n style) in the returned string.
- Maximum file size limit; can be added in a follow-up.
- Binary files, images, PDFs, notebooks (reference mentioned these; this task is UTF-8 text only).
- Configurable root via config file or env; main passes root (e.g. CWD) explicitly.

## Changes for review

- **New**: `internal/tools/readfile.go` — type `ReadFile` with field `root string`; `NewReadFile(root string) (*ReadFile, error)`; methods `Name() string`, `Description() string`, `Parameters() any`, `Execute(ctx context.Context, args map[string]any) (string, error)` implementing path resolution under root, then `os.ReadFile`, returning contents or error.
- **New**: `internal/tools/readfile_test.go` — unit tests: Execute with valid path under root returns file contents; non-existent file returns error; path outside root (or with `..`) returns error; optional: path that is a directory returns error.
- **Modified**: `cmd/buildmax/main.go` — in `runPromptMode`, obtain CWD (e.g. `os.Getwd()`), create read_file tool with `tools.NewReadFile(cwd)`, pass `[]agent.Tool{readFileTool}` to `agent.NewAgent(client, tools)`.
- **Unchanged**: `internal/agent`, `internal/llm` — no API changes; agent already accepts `[]Tool` and invokes Execute; LLM already passes Parameters through.
