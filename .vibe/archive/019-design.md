# Design 019 - Glob tool

## Goal

Enable the agent to find files by name or extension under a configurable root via a new Glob tool: required pattern, optional search path under root, results sorted by modification time (newest first), with clear success/error messages for the LLM.

## Modules

| Module (package) | Responsibility | Owns |
|------------------|----------------|------|
| **internal/tools** | Concrete agent tools (Read, Write, WebFetch, TodoWrite, Bash). | Tool types, New* constructors, Execute logic; glob.go, glob_test.go. |
| **internal/agent** | Agent loop, tool interface, dispatch by name. | Tool interface; no change in this task. |
| **cmd/buildmax** | CLI entry, setup of LLM client and tools, TUI/prompt mode. | root.go: setupAgentAndSession creates tools and passes them to NewAgent. |

## Structure

**Directory / files**

- `internal/tools/`
  - `glob.go` — Glob tool: struct, NewGlob, Tool implementation (Name, Description, Parameters, Execute).
  - `glob_test.go` — Unit tests for NewGlob and Execute (root behaviour, pattern match, optional path, errors).

**Main types and interfaces**

- **Glob** (internal/tools): Tool that lists files matching a glob pattern under a root. Holds `root string` (absolute path; all resolved paths must be under this). Implements `agent.Tool`.
- **agent.Tool** (existing): `Name() string`, `Description() string`, `Parameters() any`, `Execute(ctx, args) (string, error)`.

## Method design

| Receiver | Method | Signature | Responsibility |
|----------|--------|-----------|-----------------|
| **(package)** | **NewGlob** | `(root string) (*Glob, error)` | If root empty, use `os.Getwd()`. Absolutize and clean root; return `&Glob{root: ...}` or error. Same pattern as NewReadFile/NewBash. |
| **Glob** | Name | `() string` | Return `"Glob"`. |
| **Glob** | Description | `() string` | Short description for LLM: fast file pattern matching; supports patterns like `**/*.js` or `src/**/*.ts`; returns matching paths sorted by modification time; use to find files by name/extension; prefer multiple parallel tool calls when useful; for open-ended search consider other strategies. |
| **Glob** | Parameters | `() any` | OpenAI-style schema: `pattern` (string, required), `path` (string, optional). For `path`: directory to search in; if omitted, tool's default root is used; omit field—do not pass "undefined" or "null"; must be a valid path under the workspace if provided. |
| **Glob** | Execute | `(ctx context.Context, args map[string]any) (string, error)` | See Execute flow below. |

**Execute flow**

1. **Parse args**: Read `pattern` (required); trim space. If missing or empty after trim, return error (e.g. "missing pattern" / "pattern is empty"). Read optional `path` (string); if present and empty after trim, treat as "use root".
2. **Resolve search directory**: If `path` omitted or empty, search dir = `g.root`. Else: join `g.root` and `path`, clean and absolutize; ensure result is under `g.root` (same under-root check as ReadFile: `filepath.Rel(root, resolved)`, reject if `..` or prefix `..`). If not under root, return error "path outside allowed root". Stat resolved path; if not exist or not a directory, return appropriate error ("file not found", "path is not a directory", "permission denied").
3. **Collect matches**: Walk from search directory; for each visited entry, include only regular files (skip dirs). Match each file's path (relative to search dir or absolute, consistently) against `pattern`. Pattern must support `*` (any non-separator sequence) and `**` (any directory depth). Implementation options: (a) `filepath.WalkDir` + custom matcher that interprets `**` (e.g. split pattern by `**`, match path segments), or (b) use `github.com/bmatcuk/doublestar/v4` (e.g. `doublestar.Glob` or walk + `doublestar.PathMatch`) for Bash-style globstar. Document symlink behaviour in code (e.g. WalkDir does not follow symlinks by default).
4. **Sort**: Sort matched file paths by modification time, newest first (`os.Stat` or info from walk). Use absolute paths for consistent ordering; output format can be one path per line (absolute or relative to search root—choose one and document).
5. **Return**: If no matches, return string `"No files matched the pattern."`. If matches, return a single string (e.g. one path per line). On validation or system errors, return error so the agent sends `error: ...` to the LLM.

**Path-under-root check** (same as ReadFile)

- `joined := filepath.Join(g.root, path)`; `resolved, err := filepath.Abs(filepath.Clean(joined))`.
- `rel, err := filepath.Rel(g.root, resolved)`; if `err != nil` or `rel == ".."` or `strings.HasPrefix(rel, "..")` then "path outside allowed root".

## How they work together

**Data/control flow**

1. On startup, `setupAgentAndSession` in `cmd/buildmax/root.go` gets `cwd`, creates `globTool` via `tools.NewGlob(cwd)`, and passes it in the tool slice to `agent.NewAgent(client, []agent.Tool{..., bashTool, globTool})`.
2. Agent builds `toolDefs` and `toolsByName`; when the LLM returns a tool_call with name `"Glob"` and arguments `{"pattern": "**/*.go", "path": "internal/tools"}` (path optional), the agent looks up the tool and calls `Execute(ctx, args)`.
3. Glob resolves the search directory (root or root+path), walks and matches files, sorts by mtime, and returns a single string (or error). The agent appends the result as a tool-role message and continues the loop.

**Dependencies**

- **cmd/buildmax** depends on **internal/tools** (NewGlob) and **internal/agent** (NewAgent). No new dependency from tools to other internal packages; optional external dependency: doublestar if used for `**` (pure Go).

**Key data structures**

- **Glob.root**: Set once by NewGlob(cwd); used as the base for resolving optional `path` and as the default search directory.
- **args in Execute**: `pattern` (string, required), `path` (string, optional). Same shape as LLM tool call arguments.

## Out of scope (this task)

- Grep tool (search file contents).
- Configurable result limit or sort order.
- Symlink following policy is an implementation detail; document in code.

## Changes for review

- **New**: `internal/tools/glob.go` — Type `Glob` with field `root string`. `NewGlob(root string) (*Glob, error)`. Methods `Name`, `Description`, `Parameters`, `Execute` implementing `agent.Tool`. Execute: parse args; resolve search dir under root; walk + match (with `**` support); sort by mtime (newest first); return path list or "No files matched..." or error.
- **New**: `internal/tools/glob_test.go` — Tests for NewGlob (empty root, valid root, invalid root); Execute success (simple and recursive patterns), no matches, optional path, path outside root, path to non-directory, missing/empty pattern.
- **Modified**: `cmd/buildmax/root.go` — In `setupAgentAndSession`, after creating `bashTool`: add `globTool, err := tools.NewGlob(cwd)`; on error log and return. Pass `globTool` in the slice to `NewAgent`: `[]agent.Tool{readFileTool, writeFileTool, webFetchTool, todoWriteTool, bashTool, globTool}`.
