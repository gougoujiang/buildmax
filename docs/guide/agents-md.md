# AGENTS.md

> **Audience:** users · **Status:** current

`AGENTS.md` is how you tell the agent things it cannot infer from the code:
conventions, build commands, what not to touch. Its contents are appended to
the system prompt on every run.

This is the [agents.md](https://agents.md/) convention, not a BuildMax
invention — the same file works with other tools that support it.

## Two Layers

| File | Scope | Use for |
|---|---|---|
| `<BUILDMAX_HOME>/AGENTS.md` | Every run on this machine | Your personal preferences |
| `<workspace>/AGENTS.md` | That project | Project conventions, checked into git |

Both are appended when present, **global first, then workspace** — so project
rules come last and take precedence in the model's reading.

The split matters: your preference for a commit message style belongs in the
global file, not in a repository every contributor shares.

## What To Put In It

Things that change what the agent *does*:

```markdown
# Project

Go service, single binary. Run `./make test` after changes — it sets up the
test sandbox correctly, plain `go test` does not.

## Conventions

- Persisted JSON uses explicit snake_case tags
- Database tables are singular: `user`, not `users`
- Never edit files under `internal/gen/` — they are generated

## Before Finishing

Run `./make test` and `gofmt -l .`
```

What not to put in it: anything the agent can read from the code. Package lists,
type definitions, and directory trees go stale and cost context on every single
run. The value of this file is what is *not* discoverable.

## Remote Runs

For a Portal task run, the worker prepares an `AGENTS.md` in the run directory —
the run layout plus any `AGENTS.md` from the team's materialized files — so the
same convention applies when the shared runtime executes there. A project rule
you write once applies to local runs and background runs alike.

## Related

- [skills-and-subagents.md](skills-and-subagents.md) — instructions loaded on
  demand instead of on every run
- [start/quickstart.md](../start/quickstart.md)
