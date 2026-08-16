# Documentation Conventions

> **Audience:** contributors · **Status:** current

## Organized By Reader, Not By Artifact

`docs/` is split by the question someone is trying to answer:

| Directory | Reader | Contains |
|---|---|---|
| `start/` | Someone who just installed it | Install, quickstart, concepts |
| `guide/` | Someone using it | One document per capability, task-oriented |
| `deploy/` | Someone running it for a team | Topology, authentication, local cluster |
| `reference/` | Someone looking something up | Configuration, CLI, webhook — tables, not prose |
| `contribute/` | Someone changing the code | Layout, architecture, these conventions |
| `design/` | Someone asking "why is it like this" | Semantic design records indexed by lifecycle |

The test for where a document belongs is **who is stuck without it**, not what
kind of document it is.

## Design Documents

Use a stable semantic filename such as `sandbox-boundaries.md`. The filename
describes the subject, not chronology, roadmap priority, or lifecycle, so code
comments and other documents can cite it without inheriting planning metadata:

```go
// Mirrors the design in docs/design/sandbox-boundaries.md.
```

`design/README.md` separates three kinds of entry:

- **Product direction** — durable decisions spanning roadmap phases.
- **Active roadmap plans** — planned or partly implemented work under a
  `ROADMAP.md` priority. These expire when the work lands or changes direction.
- **Subsystem specifications** — durable records of how an implemented or
  partly implemented subsystem is designed. These stay current.

A design document is **rationale, not user documentation**. When a design ships
a user-configurable feature, the user-facing half belongs in `guide/` or
`reference/`, and the design document links to it and keeps the trade-offs and
open gaps.

## Retiring A Document

There is no archive directory. A document that no longer describes the current
direction is **deleted** — git history keeps it, and a stale document in the
tree costs more than the history is worth. Recover one with:

```bash
git log --diff-filter=D --oneline -- docs/
git show <commit>^:docs/path/to/file.md
```

If a retired document contains something still true and still needed, move that
content to `guide/` or `reference/` first, verified against the code, then
delete the original.

## Document Header

Every document opens with its audience and status:

```markdown
> **Audience:** operators · **Status:** current
```

`Status` is one of `current`, `planned`, or a specific caveat such as
`current — this describes a known gap`. A reader must be able to tell within one
line whether a document can be trusted.

## Single Sources Of Truth

Repeating a fact in two documents guarantees that one of them becomes wrong.

| Fact | Lives in | Everything else |
|---|---|---|
| Repository tree | [repo-layout.md](repo-layout.md) | links |
| Environment variables | `internal/config/env_spec.go` → [reference/configuration.md](../reference/configuration.md) | links |
| Config file fields | `config-examples/*.example.yaml` → [reference/configuration.md](../reference/configuration.md) | links |
| HTTP routes | `internal/server/handlers/routes.go` → `/openapi.json` | links |
| Roadmap priorities | [ROADMAP.md](../ROADMAP.md) | links |

## What Is Enforced

`internal/architecture/docs_test.go` runs with the normal test suite and fails
the build on the three ways documentation rots silently:

| Test | Fails when |
|---|---|
| `TestDocsLinksResolve` | A relative markdown link points at a file that does not exist |
| `TestEnvVarsDocumented` | `config.EnvVars` gains a variable missing from [reference/configuration.md](../reference/configuration.md) |
| `TestToolNamesDocumented` | A tool name constant is missing from [guide/tools.md](../guide/tools.md) |

The tool-name check exists because those strings are user-visible contract —
they appear in hook `matcher` regexes and subagent `tools:` fields, so renaming
a tool without updating the docs breaks working configuration silently.

Everything else is convention, upheld in review.

## Updating Docs With Code

| Change | Update |
|---|---|
| Package boundary or runtime contract | The matching document in [architecture/](architecture/README.md), same pull request |
| User-visible behavior or configuration | `guide/`, `reference/`, and `config-examples/` |
| Direction | Add or update a semantic record in [../design/](../design/README.md) |
| A package moves | [repo-layout.md](repo-layout.md) — and nowhere else |

## Style

- Written in English so every contributor can read it.
- Cite documents by repository-relative path so links survive being moved.
- Prefer a table to a bulleted list when the content is a lookup.
- State the gap. A document that quietly omits what does not work yet is worse
  than no document — say "off by default", "not wired yet", "development only".
