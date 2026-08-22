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
id, err := util.NewPublicID()        // → "ivyoh5qcfu6ypfkhyedq"
raw, ok := util.ParsePublicID(id)    // → 12 bytes
text := util.FormatPublicID(raw)     // → the canonical text again
```

A server entity's public identifier is 96 bits of crypto-random data, stored as
`BINARY(12)` and written as 20 lowercase base32 characters (`a-z2-7`). It is
the only handle that leaves the process; the numeric primary key is the
relational key and stays inside `internal/infra/db`. Why base32 rather than a
shorter base64url, and which tables have a public ID at all, is in
[../../design/entity-identity.md](../../design/entity-identity.md).

`NewPublicID` returns an error rather than panicking: entropy failure must be
one failed create, not a process abort inside a request. `ParsePublicID` accepts
either case and rejects everything that is not canonical, so one value has one
spelling — which is why a canonical ID always ends in `a` or `q`, and why a
hand-written fixture usually is not one.

`NewPrefixedID` remains for four identifiers that name something other than a
row: `as_` a login chain of refresh tokens, `jb_` a local background job, `rt_`
a trace file, and `p_` a Desktop project. Agent session IDs are UUIDs.

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
