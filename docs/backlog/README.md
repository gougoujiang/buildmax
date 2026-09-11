# BuildMax Backlog

> **Audience:** maintainer and agents · **Status:** current

The backlog is the layer between an approved design and a pull request. It holds
the decomposed, ready-to-execute units of the project's own main-line work, one
file per task, so any session — a person or an agent — can pick one up with no
prior context and carry it to a merged change.

It is deliberately not GitHub issues. GitHub issues are the surface for external
contributors; this backlog is the maintainer-and-agent execution queue. See
"One item, one place" below.

## Where It Sits

```text
proposal (discussion) → design (approved rationale) → backlog task → pull request → current-state / changelog
```

- [`docs/ROADMAP.md`](../ROADMAP.md) owns theme-level priority, sequencing, and
  release gates. It does not hold executable units.
- [`docs/design/`](../design/README.md) owns rationale for an approved shape.
- This backlog owns the decomposition of that shape into independently
  executable tasks.
- [`docs/current-state.md`](../current-state.md) records what shipped, after a
  task merges.

The concept originates in
[`docs/proposals/single-maintainer-agent-development.md`](../proposals/single-maintainer-agent-development.md)
as the "Ready For Agent" queue.

## The Queue Is The Directory

Each task is one file named `NN-slug.md`. The two-digit `NN` prefix is the
priority: the directory sorted by name is the queue, highest priority first.
Leave gaps (`10`, `20`, `30`) so a task can be inserted without renaming its
neighbours. Reordering is a rename; there is no separate index to keep in sync.

Only `NN-slug.md` files are live tasks. [`TEMPLATE.md`](TEMPLATE.md) is the
starting point for a new one and is not itself a task.

## Lifecycle

- **Create** a task when its source design is approved. A planning session may
  draft it, but the maintainer's priority decides its `NN` and whether it enters
  the queue at all.
- **Claim** a task by setting `claim` in its frontmatter before starting work,
  so two sessions do not pick the same one. Clear it if the work is abandoned.
  At most one live claim per task.
- **Delete** the file when the work merges. The backlog holds only pending work;
  the permanent record of a finished task is its pull request, its changelog
  entry, and the update to [`current-state.md`](../current-state.md). There is no
  archive here — git history keeps deleted tasks.
- **Refresh:** readiness is not permanent. A task whose scope, acceptance, or
  cited design has gone stale leaves the queue (delete it, or drop it back to a
  draft) until it is made ready again.

## Entry Gate

- Main-line work goes through a design record first, and the task's `source`
  links the approved design section it derives from.
- Small, low-risk, self-evident work (a doc fix, a narrow test gap) may enter
  the backlog directly with `source: direct` and no design record.

## One Item, One Place

A unit of work lives in exactly one tracking surface. Main-line and
agent-executed work is a backlog task. Externally contributable work is a GitHub
issue (`agent-ready`, `good first issue`, `help wanted`). Do not mirror the same
work into both.

## What A Ready Task Must Answer

A task is ready only when a session with no prior context can act on it. See
[`TEMPLATE.md`](TEMPLATE.md) for the exact shape. It must state the outcome and
why it matters, the scope and what is explicitly out of scope, concrete
acceptance criteria, and the verification scopes to run — selected from
[`docs/contribute/testing.md`](../contribute/testing.md).
