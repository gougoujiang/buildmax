# Layering And Import Rules

> **Audience:** contributors · **Status:** current
>
> For what lives where, see [repo-layout.md](../repo-layout.md). This document
> is about which direction dependencies may point.

## The Rule

```text
bootstrap ──▶ interface / server / service / agentapp / infra ──▶ core
```

Dependencies point inward. `internal/core` is pure domain — entities, shared contracts,
and the agent loop — and knows nothing about databases, HTTP, the filesystem, or
configuration. Everything else may depend on it.

This is what makes one agent loop serve four surfaces: the loop cannot reach for
a config file or a database, so CLI, Desktop, worker, and Portal all supply the
same dependencies through explicit options.

## What Is Enforced

`internal/architecture/architecture_test.go` fails the build on a violating
import. Three rules, checked by parsing every file under `internal/`:

| Layer | May not import |
|---|---|
| `internal/core` | `agentapp`, `bootstrap`, `config`, `infra`, `interface`, `server`, `service` |
| `internal/infra` | `bootstrap`, `interface`, `server` |
| `internal/server` | `bootstrap`, `config`, `interface` |
| `internal/service` | `agentapp`, `bootstrap`, `interface`, `server` |
| `internal/agentapp` | `bootstrap`, `interface`, `server` |

Two consequences worth internalizing:

- **`core` may not import `config`.** Configuration is resolved at the edge and
  passed in. A core package that wants a setting takes it as a field.
- **`server` may not import `config`.** The server receives resolved values from
  `internal/bootstrap`; it never reads `server.yaml` itself.

A fourth rule bans **type aliases anywhere under `internal/`**
(`TestNoInternalTypeAliases`). Aliases make a package look like it owns a type it
merely re-exports, which is exactly how a boundary quietly dissolves during a
refactor. Import the defining package directly.

A fifth rule bans exported mutable package variables except error sentinels and
the two build metadata values written through linker flags. Shared inventories
are functions returning copies (`config.EnvVars()`,
`tool.BuiltinSubAgentDefs()`), so one importer cannot rewrite another run's
behavior.

## Where Things Belong

| Kind of code | Package |
|---|---|
| Domain entities and cross-service repository contracts | `internal/core/model` |
| A persistence port used by one orchestrator | The consuming `internal/service/*` package |
| LLM contracts, tool contract, tool policy | `internal/core/llm` |
| The tool-calling loop | `internal/core/agent` |
| Business rules that coordinate stores and runs | `internal/service/*` |
| Assembling a runnable agent for a surface | `internal/agentapp` |
| Talking to an external system | `internal/infra/*` |
| HTTP transport and handlers | `internal/server` |
| A local user entry point | `internal/interface/*` |
| Process startup and dependency wiring | `internal/bootstrap` |

`internal/tool` is intentionally not pure: tools are implementations, and they
import infra (MCP, git) as needed.

There is no `internal/app` package, and adding one would blur the
interface/service split these rules exist to keep. Startup wiring belongs in
`internal/bootstrap`.

## When A Rule Fights You

The import is the problem, not the test. In practice a violation means one of:

- a core package wants configuration → take the value as a parameter or field
- a core package wants to do I/O → define an interface in core, implement it in
  infra, inject it; if only one service consumes the capability, let that
  service own the interface
- the server wants something from `interface/` → it belongs in `service/` or
  `core/` if both need it

## Related

- [repo-layout.md](../repo-layout.md) — the full tree
- [agent-loop.md](agent-loop.md) — what purity buys in practice
- [overview.md](overview.md) — how the layers fit together at runtime
