# Design 021 - Grep tool

## Goal

Define the structure and APIs for an agent tool **Grep** that searches file contents by regex pattern under a configurable root, with file filtering (glob, type), three output modes, context lines, and pagination — implemented in pure Go using `regexp` and `doublestar`.

## Modules

| Module (package) | Responsibility | Owns |
|------------------|----------------|------|
| **internal/tools** | Concrete agent tools (Read, Write, Edit, Glob, **Grep**). Path resolution under root, `agent.Tool` implementation, regex search and output formatting. | `readfile.go`, `writefile.go`, `editfile.go`, `glob.go` (existing); **new** `grep.go`, `grep_test.go` |
| **internal/agent** | Agent loop, `Tool` interface, tool invocation. | `agent.go` (unchanged) |
| **cmd/buildmax** | CLI entry, agent/session setup, tool construction. | `root.go` (wiring only) |

## Structure

**Directory / files**

- `internal/tools/` — agent tools
  - `readfile.go` — Read tool (existing, unchanged)
  - `writefile.go` — Write tool (existing, unchanged)
  - `editfile.go` — Edit tool (existing, unchanged)
  - `glob.go` — Glob tool (existing, unchanged)
  - **`grep.go`** — Grep tool: `Grep` type, `NewGrep`, `Tool` implementation with regex search, file filtering, context lines, output formatting, pagination
  - **`grep_test.go`** — Unit tests for Grep

- `cmd/buildmax/` — CLI
  - `root.go` — **Edit** `setupAgentAndSession`: add `NewGrep(cwd)`, pass `grepTool` to `NewAgent`

**Main types and interfaces**

- **Grep** (internal/tools): Tool that searches file contents by regex pattern under a root. Holds `root string` (absolute path). Implements `agent.Tool` (Name, Description, Parameters, Execute). Path resolution mirrors ReadFile/Glob: join root + path, clean, absolutize, then ensure resolved path is under root. Accepts both files and directories as `path` (file → search single file; directory → walk recursively). Filters candidate files by glob pattern (doublestar) and/or type extension map. Searches file lines with compiled `regexp.Regexp`. Formats output in one of three modes: `content` (ripgrep-style grouped lines with optional context), `files_with_matches` (file paths only), `count` (match counts per file). Supports pagination with `offset` and `head_limit`.
- **Tool** (internal/agent): Unchanged. `Name()`, `Description()`, `Parameters() any`, `Execute(ctx, args) (string, error)`.

## Method design

| Receiver | Method | Signature | Responsibility |
|----------|--------|-----------|----------------|
| (package) | **NewGrep** | `(root string) (*Grep, error)` | If root is empty, use `os.Getwd()`. Absolutize and clean root; return `&Grep{root: ...}` or error. Same pattern as `NewGlob`. |
| **Grep** | **Name** | `() string` | Return `"Grep"`. |
| **Grep** | **Description** | `() string` | Concise description: searches file contents by regex pattern; supports glob/type file filters; three output modes (content, files\_with\_matches, count); context lines; case-insensitive and multiline flags; use for finding code by pattern. |
| **Grep** | **Parameters** | `() any` | JSON schema: `type: "object"`, properties for all 13 parameters (see below), `required: ["pattern"]`. Snake\_case keys. |
| **Grep** | **Execute** | `(ctx context.Context, args map[string]any) (string, error)` | Orchestrator: parse args, compile regex, resolve path, collect files, search, format output. See detailed flow below. |
| **Grep** | **compilePattern** | `(pattern string, caseInsensitive, multiline bool) (*regexp.Regexp, error)` | Private. Prepend `(?i)` if case\_insensitive, `(?ms)` if multiline, then `regexp.Compile`. Return compiled regex or clear error ("invalid regex pattern: ..."). |
| **Grep** | **resolvePath** | `(args map[string]any) (resolved string, isFile bool, err error)` | Private. Parse optional `path` from args. If absent, return `(g.root, false, nil)`. Else resolve under root (join, clean, abs, Rel check for `..` prefix). Stat: if file → `(resolved, true, nil)`; if dir → `(resolved, false, nil)`. Return error for not-found, permission-denied, path-outside-root. |
| **Grep** | **collectFiles** | `(searchDir string, isFile bool, globPattern, typeFilter string) ([]string, error)` | Private. If `isFile`, return `[]string{searchDir}` (single file, after checking glob/type filters). Otherwise `filepath.WalkDir(searchDir, ...)`: skip directories, skip files failing glob/type filter, collect absolute paths. Glob filter uses `doublestar.Match` against the file's path relative to `searchDir`. Type filter checks file extension against `typeExtensions` map. Skip files that fail to read (log and continue). Return sorted list of absolute paths (alphabetical for deterministic output). |
| **Grep** | **searchFile** | `(filePath string, re *regexp.Regexp, multiline bool) ([]fileMatch, error)` | Private. Read file with `os.ReadFile`. If multiline=false: split into lines, test each line with `re.MatchString`, record matching line numbers (1-based). If multiline=true: use `re.FindAllStringIndex` on full content, map byte offsets to line numbers, collect the set of matched line numbers. Return `[]fileMatch` (one per matching line: `{lineNum int, text string}`). Return nil slice if no matches (not an error). On read failure, return error. |
| **Grep** | **formatContent** | `(results []fileResult, before, after int, showLineNumbers bool, offset, limit int) string` | Private. For each file with matches: compute context ranges (merge overlapping), emit file header, emit context/match lines with `NUM:` (match) or `NUM-` (context) prefix (ripgrep style). Separate non-adjacent groups within a file with `--`. Separate files with blank line. Apply `offset` and `limit` on match entries (not lines). If `showLineNumbers` is false, omit line-number prefix. Return formatted string or "No matches found." |
| **Grep** | **formatFilesWithMatches** | `(results []fileResult, offset, limit int) string` | Private. Collect file paths. Apply `offset` and `limit`. Return one path per line or "No matches found." |
| **Grep** | **formatCount** | `(results []fileResult, offset, limit int) string` | Private. For each file: `filepath: N`. Apply `offset` and `limit`. Return or "No matches found." |

**Helper types** (unexported, in `grep.go`):

- **fileMatch**: `struct { lineNum int; text string }` — one matching line in a file.
- **fileResult**: `struct { path string; lines []string; matches []fileMatch }` — all matches for one file, plus the full line slice (for context retrieval).

**Type extension map** (package-level `var`):

```go
var typeExtensions = map[string][]string{
    "go":   {".go"},
    "js":   {".js"},
    "ts":   {".ts"},
    "tsx":  {".tsx"},
    "py":   {".py"},
    "java": {".java"},
    "rust": {".rs"},
    "c":    {".c"},
    "cpp":  {".cpp", ".cc", ".cxx"},
    "h":    {".h"},
    "css":  {".css"},
    "html": {".html", ".htm"},
    "json": {".json"},
    "yaml": {".yaml", ".yml"},
    "md":   {".md"},
    "sh":   {".sh"},
    "sql":  {".sql"},
    "xml":  {".xml"},
    "toml": {".toml"},
    "rb":   {".rb"},
}
```

**Parameters schema** (returned by `Parameters()`):

| Parameter | Type | Required | Default | JSON description (mentions rg equivalent) |
|-----------|------|----------|---------|---------------------------------------------|
| `pattern` | string | yes | — | "Regex pattern to search for in file contents" |
| `path` | string | no | root | "File or directory to search in (equivalent to rg PATH). Defaults to workspace root." |
| `glob` | string | no | — | "Glob pattern to filter files (e.g. `*.go`, `*.{ts,tsx}`). Equivalent to rg --glob." |
| `type` | string | no | — | "File type to search (e.g. go, js, py). Equivalent to rg --type. Common types: go, js, ts, py, java, rust, c, cpp, html, json, yaml, md." |
| `output_mode` | string | no | `"files_with_matches"` | "Output mode: `content` shows matching lines with context, `files_with_matches` shows file paths only, `count` shows match counts per file. Defaults to `files_with_matches`." |
| `before_context` | number | no | 0 | "Lines to show before each match in content mode (equivalent to rg -B)." |
| `after_context` | number | no | 0 | "Lines to show after each match in content mode (equivalent to rg -A)." |
| `context` | number | no | 0 | "Lines to show before and after each match in content mode (equivalent to rg -C). Overrides before\_context and after\_context if set." |
| `line_numbers` | boolean | no | true | "Show line numbers in content mode output (equivalent to rg -n). Defaults to true." |
| `case_insensitive` | boolean | no | false | "Case-insensitive search (equivalent to rg -i)." |
| `multiline` | boolean | no | false | "Multiline mode: `.` matches newlines, `^`/`$` match line boundaries (equivalent to rg -U --multiline-dotall)." |
| `head_limit` | number | no | 0 | "Limit output entries. In content mode limits match entries; in files\_with\_matches/count limits files. 0 means unlimited." |
| `offset` | number | no | 0 | "Skip first N entries before applying head\_limit (pagination)." |

## How they work together

**Data/control flow**

1. **Setup**: `setupAgentAndSession` in `root.go` gets `cwd`, creates all existing tools plus **`grepTool, err := tools.NewGrep(cwd)`**. Passes `grepTool` in the tool slice to `agent.NewAgent`. Agent builds `toolDefs` and `toolsByName` including `"Grep"`.

2. **Agent loop** (unchanged): User message → `Process` → `processLoop` → `ChatWithTools(messages, toolDefs)` → LLM may return a tool\_call with name `"Grep"` and arguments like `{"pattern": "func.*Handler", "path": "internal/", "output_mode": "content", "before_context": 2, "after_context": 2}`.

3. **Tool execution** (`Grep.Execute`):
   - **Parse args**: Extract `pattern` (required, non-empty string), optional `path`, `glob`, `type`, `output_mode` (default `"files_with_matches"`), `before_context`/`after_context`/`context` (default 0; if `context` > 0, override both before and after), `line_numbers` (default true), `case_insensitive` (default false), `multiline` (default false), `head_limit` (default 0), `offset` (default 0).
   - **Compile regex**: `compilePattern(pattern, caseInsensitive, multiline)` → `*regexp.Regexp` or error.
   - **Resolve path**: `resolvePath(args)` → `(searchPath string, isFile bool, err)`. Uses same path-under-root check as other tools.
   - **Collect candidate files**: `collectFiles(searchPath, isFile, glob, type)` → `[]string`. Walk directory (or single file), apply glob filter via `doublestar.Match`, apply type filter via extension check against `typeExtensions`.
   - **Search each file**: For each candidate file, `searchFile(filePath, re, multiline)` → `[]fileMatch`. Read file, find matching lines (line-by-line for normal mode, full-content match with byte-offset-to-line mapping for multiline). Skip files that error on read (log warning, continue).
   - **Build results**: Collect `[]fileResult` (only files with at least one match).
   - **Format output**: Based on `output_mode`:
     - `"files_with_matches"` → `formatFilesWithMatches(results, offset, limit)` — one absolute path per line.
     - `"count"` → `formatCount(results, offset, limit)` — `filepath: N` per line.
     - `"content"` → `formatContent(results, before, after, lineNumbers, offset, limit)` — ripgrep-style grouped output.
   - **Return**: Formatted string. "No matches found." if results are empty.

4. **Result**: Agent appends tool-role message with the formatted string (or `"error: ..."` on failure). LLM continues with next iteration or returns final reply.

**Context line merging algorithm** (in `formatContent`):

For each file's matches, compute display ranges:
1. For each match line `m`, the desired range is `[m - before, m + after]` (clamped to `[1, totalLines]`).
2. Sort ranges by start line.
3. Merge overlapping or adjacent ranges into contiguous groups.
4. For each group: emit lines in order. Lines whose number is in the match set get `NUM:line` (colon separator). Context-only lines get `NUM-line` (dash separator). Between non-adjacent groups in the same file, emit `--` separator.
5. Between files, emit a blank line.

Offset and head\_limit in `content` mode apply to **match entries** (not output lines). A match entry = one matching line across all files. Offset skips the first N match entries; head\_limit caps how many are shown. Context lines are included around the surviving matches but don't count toward the limit.

**Dependencies**

- **internal/tools** depends on **internal/agent** only for the `Tool` interface (via method implementation). Depends on `regexp` (stdlib), `doublestar` (already in `go.mod` for Glob). No new external dependencies.
- **cmd/buildmax** imports **internal/tools** and **internal/agent**; constructs concrete tools and passes them to the agent.
- No dependency from **internal/agent** to **internal/tools**.

**Key data structures**

- **args** for Grep: `map[string]any` with `pattern` (string), and optionally `path`, `glob`, `type`, `output_mode`, `before_context`, `after_context`, `context`, `line_numbers`, `case_insensitive`, `multiline`, `head_limit`, `offset`. Produced by LLM JSON, consumed by `Grep.Execute`.
- **Grep.root**: Set once by `NewGrep`; used in `Execute` for path resolution and under-root check.
- **fileMatch**: `{lineNum int, text string}` — one matched line; created by `searchFile`, consumed by format functions.
- **fileResult**: `{path string, lines []string, matches []fileMatch}` — per-file search result; `lines` is the full file split for context retrieval; created after `searchFile`, consumed by format functions.
- **typeExtensions**: Package-level `map[string][]string`; consulted in `collectFiles` to filter by file type.

## Changes for review

- **New**: `internal/tools/grep.go` — `Grep` struct with `root string`; `NewGrep(root string) (*Grep, error)`; `Name()`, `Description()`, `Parameters()`, `Execute()` implementing `agent.Tool`. Private helpers: `compilePattern`, `resolvePath`, `collectFiles`, `searchFile`, `formatContent`, `formatFilesWithMatches`, `formatCount`. Unexported types `fileMatch`, `fileResult`. Package-level `typeExtensions` map. Uses `regexp` (stdlib) and `doublestar` (existing dep).
- **New**: `internal/tools/grep_test.go` — Unit tests using `t.TempDir()`: NewGrep (empty root, valid root, invalid root); Execute pattern validation (missing, empty, invalid regex); path validation (outside root, non-existent, file vs directory, omitted); output modes (content, files\_with\_matches, count); filters (glob, type, both); context lines (before, after, combined, overlap merging); flags (case\_insensitive, line\_numbers=false, multiline); pagination (head\_limit, offset, combined); no matches message.
- **Modified**: `cmd/buildmax/root.go` — In `setupAgentAndSession`, after `editFileTool`: add `grepTool, err := tools.NewGrep(cwd)`; on error log and return. Update tool slice: `[]agent.Tool{readFileTool, writeFileTool, webFetchTool, todoWriteTool, bashTool, globTool, editFileTool, grepTool}`. Update function doc comment to include "Grep".
