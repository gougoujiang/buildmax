# Design: Task 112 — Variable interpolation in MCP config loading

**As built:** merge global + workspace `mcp.json` when `BUILDMAX_MCP_CONFIG` is unset; expansion uses `os.Expand` with `mcpExpandMapping` (`EnvKeyBuildmaxWorkspaceRoot` from loader, then `os.Getenv`). See `112.md`.

## Goal

Document post-parse, pre-validation expansion for MCP `mcp.json` strings inside `internal/config`, without changing `mcpservers` or `agentrun` call sites.

## Modules

| Package | Role |
|---------|------|
| `internal/config` | Owns expansion; `LoadMCPConfigForWorkspace` orchestrates parse → expand → validate. |
| `internal/infra/mcp` | Unchanged; continues to receive already-expanded `MCPServerConfig`. |
| `internal/execution/agentrun` | Unchanged; still calls `LoadMCPConfigForWorkspace` only. |

## Data flow

```mermaid
flowchart LR
  file[mcp.json] --> unmarshal[json.Unmarshal]
  unmarshal --> expand[expandMCPConfigEnv]
  expand --> validate[validate each server]
  validate --> root[MCPConfigRoot]
  root --> registry[mcpservers.NewRegistry]
```

## Structure

**File:** `internal/config/mcp.go` (same file as today; keeps MCP types and load path in one place).

**New private API:**

```go
// expandMCPConfigEnv replaces $VAR and ${VAR} in command, args, env values, and url
// for each mcpServers entry. Mutates root in place. Uses os.ExpandEnv (process env; $$ -> $).
func expandMCPConfigEnv(root *MCPConfigRoot)
```

**No new exported types or funcs** unless tests in another package need them (they do not—tests stay in `config`).

### Method design — `expandMCPConfigEnv`

- **Receiver / args:** `root *MCPConfigRoot`. If `root == nil` or `root.MCPServers == nil`, return immediately (caller already normalizes empty map after unmarshal; expansion can still run after that normalization for consistency).
- **Behavior:**
  - Iterate **by server id** over `root.MCPServers` (read struct, mutate fields, write back) because ranging `for id, s := range map` yields copies of `MCPServerConfig`.
  - For each `MCPServerConfig`:
    - `Command = os.ExpandEnv(Command)`
    - For each index `i` in `Args`: `Args[i] = os.ExpandEnv(Args[i])`
    - If `Env != nil`: for each key `k`, `Env[k] = os.ExpandEnv(Env[k])` (keys unchanged)
    - `URL = os.ExpandEnv(URL)`
  - Do **not** expand `Type` or map keys (per spec).
- **Idempotency:** Second expansion is safe for typical env values without `$`; strings that become new `$...` patterns could double-expand—document mentally as “don’t put expandable syntax in env values used only as substitution targets”; not a product requirement to special-case.

### Method design — `LoadMCPConfigForWorkspace`

Adjust sequence to:

1. Resolve path; if missing, return `(nil, nil)`.
2. Read file, `json.Unmarshal` into `root`.
3. If `root.MCPServers == nil`, set `root.MCPServers = map[string]MCPServerConfig{}` (unchanged).
4. **`expandMCPConfigEnv(&root)`** — new step.
5. Existing validation loop over `root.MCPServers` (unchanged).

Error messages remain attached to server `id` and field semantics as today.

## Tests (`internal/config/mcp_test.go`)

Add table-driven test(s) or focused subtests with `t.Setenv` / `t.Cleanup`:

| Case | Input snippet | Env | Expect |
|------|---------------|-----|--------|
| URL | `type: http`, `url: "http://$MCP_HOST/x"` | `MCP_HOST=example` | expanded URL before implicit connect tests N/A in config pkg—assert `cfg.MCPServers["x"].URL` |
| Arg | stdio, `args: ["$TOKEN"]` | `TOKEN=abc` | first arg `abc` |
| Env value | `env: {"K": "v-$S-v"}` | `S=m` | value `v-m-v` |
| Missing var | `url: "http://$NOTSET/path"` | unset | `http:///path` (empty host segment)—assert string equality |
| Literal dollar | `command: "echo$$"` | — | `echo$` per `os.ExpandEnv` |

Use a temp dir + `BUILDMAX_MCP_CONFIG` pointing at written JSON (same pattern as existing tests).

**Parallelism:** Prefer **distinct env var names per subtest** or serial tests to avoid cross-talk if subtests run in parallel.

## How pieces work together

1. Only `LoadMCPConfigForWorkspace` performs expansion; any future loader that builds `MCPConfigRoot` from JSON should call `expandMCPConfigEnv` before validation (YAGNI: single entry point today).
2. `mcpservers.newTransport` keeps using plain strings; no awareness of interpolation.

## Changes for review

| Area | Change |
|------|--------|
| `internal/config/mcp.go` | Add `expandMCPConfigEnv`; invoke from `LoadMCPConfigForWorkspace` after unmarshal / nil-map fix, before validation. |
| `internal/config/mcp_test.go` | New tests for expansion cases above; ensure existing path/validation tests still pass. |
| `go.mod` | No change. |

No edits to `internal/infra/mcp/*`, `internal/execution/agentrun/runtime.go`, or OpenAPI/docs for this task.
