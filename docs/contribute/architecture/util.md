# Util

> **Audience:** contributors · **Status:** current

## Purpose

`internal/util` holds small, dependency-free helpers shared across layers. It is
deliberately unthemed, and deliberately thin: anything that grows a concept of
its own belongs in the package that owns that concept, not here.

Two things that used to live here now do not — git helpers are in
`internal/infra/git`, and tool argument parsing is in `internal/tool/argparse.go`.

## Entity IDs (`id.go`)

```go
util.NewPrefixedID(util.PrefixTask)   // → "t_9f3k2m8x1qwe7rt4zy0p"
```

Pass the prefix **without** the underscore — `NewPrefixedID` joins them.

The repository-wide ID format: a short type prefix, then 20 characters of
lowercase base36, derived from 128 bits of crypto-random entropy.

Every prefix is a constant in `id.go`; the full set is `u`, `tm`, `i`, `a`, `w`,
`wr`, `wsr`, `c`, `cm`, `t`, `r`, `ar`, `f`, `whk`. Session IDs are the one
exception — they are internal and use UUIDs.

IDs carry no ordering — sort by `created_at`, never by ID. `id.go` and its tests
are the reference for the format.

## Workspace Paths (`workspace.go`)

```go
root, err := util.ResolveWorkspaceRoot(dir)      // "" means current directory
abs,  err := util.ResolvePath(root, userPath)    // fails if it escapes root
```

These are free functions, not methods on a workspace object. `ResolvePath` is
the containment check behind every file tool: it resolves a model-supplied path
against the workspace root and returns an error rather than a path when the
result would fall outside. It is Windows-safe and does not stat the path.

This boundary is independent of the bash sandbox and applies whether or not the
sandbox is enabled.

## Helpers (`helpers.go`)

| Function | Role |
|---|---|
| `Ptr(v)` | `*T` from a value — for optional struct fields |
| `WithEnvVar(key, value, fn)` | Run `fn` with an env var set, then restore. Test-oriented. |
| `TruncateRunes` / `ClipRunes` | Rune-safe shortening, so multi-byte text is never split mid-character |
| `FormatDuration` / `FormatUnixMinute` | Display formatting for the TUI and Portal |
| `WorkerJobNameForTaskRun(id)` | Kubernetes Job name for a task run; the `At` variant takes an explicit time for tests |

## Test Helpers (`testing.go`)

`SignJWT` and `SignJWTWithExp` mint tokens for server handler tests. They live
in a non-test file so other packages' tests can use them.

## Dependencies

- **Uses**: standard library, plus a JWT library for the test helpers
- **Used by**: `internal/tool` (path resolution), `internal/server` and
  `internal/service` (ID generation), CLI and Portal display code

## Related

- [tools.md](tools.md) — how the workspace root reaches tool execution
- [ID format convention](../../../AGENTS.md) — section 6.3
