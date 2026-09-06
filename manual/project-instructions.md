# Project instructions & memory

Two ways to shape how the agent works in a project: `AGENTS.md`, the instructions
you write and it always follows, and project memory, the notes the agent keeps
for itself between sessions. Instructions are rules; memory is recall.

## AGENTS.md project instructions

`AGENTS.md` is how you tell the agent things it cannot infer from the code:
conventions, build commands, what not to touch. Its contents are appended to
the system prompt on every run.

This is the [agents.md](https://agents.md/) convention, not a BuildMax
invention — the same file works with other tools that support it.

### Two layers

| File | Scope | Use for |
|---|---|---|
| `<BUILDMAX_HOME>/AGENTS.md` | Every run on this machine | Your personal preferences |
| `<workspace>/AGENTS.md` | That project | Project conventions, checked into git |

Both are appended when present, **global first, then workspace** — so project
rules come last and take precedence in the model's reading.

The split matters: your preference for a commit message style belongs in the
global file, not in a repository every contributor shares.

### What to put in it

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

### Remote runs

For a Portal task run, the worker prepares an `AGENTS.md` in the run directory —
the run layout plus any `AGENTS.md` from the space's materialized files — so the
same convention applies when the shared runtime executes there. A project rule
you write once applies to local runs and background runs alike.

## Project memory

The agent keeps a small set of notes about each project: a preference you stated
once, a decision and why, a library that turned out not to work. Each one is its
own Markdown file. What the agent carries into every turn is only the *index* —
a name and a one-line description per memory — and it opens a memory's body when
that line suggests it is worth reading.

It is **recall, not instruction**. The agent may have got it wrong, and what you
say now overrides it. For rules that must always be followed, use `AGENTS.md`
instead — the agent never writes that file itself.

### What a project is

One project is one Git repository, including every worktree of it, or one plain
folder. A session started in a worktree reads and writes the same memories as one
started in the main checkout, because they are the same work.

Two clones of the same repository are two projects. So are two unrelated folders.
Nothing is grouped by a remote URL.

`buildmax doctor` names the project the current directory belongs to;
`buildmax project list` shows them all. In the TUI, `/info` has a `memory` tab
that lists this project's memories and opens one with `enter`; `buildmax info`
prints the same listing; and in Desktop the **Memory** button beside the message
box opens the same list, with the body of whichever you select.

### Where they live

```text
<BUILDMAX_HOME>/projects/<project_id>/memory/
  MEMORY.md                  # generated index — edit a memory, not this
  rejected-sse-transport.md
  fixture-layout.md
```

Each file is frontmatter plus a body, the same shape skills and subagent
definitions use:

```markdown
---
name: rejected-sse-transport
description: SSE was rejected for the event stream; it cannot resume mid-turn
type: project
session_id: 0f1e...
updated_at: 2026-08-29T10:00:00Z
---

The event stream uses WebSocket, not SSE.

**Why:** SSE was tried in the worker-stream spike and dropped because a
reconnect cannot resume a turn already in flight.

**How to apply:** do not re-propose SSE for streaming without addressing
resume. Related: [[worker-stream-contract]].
```

They are yours: open one, edit it, or delete the file. The next run reads
whatever the files say, and regenerates `MEMORY.md` from them. `buildmax doctor`
prints the directory, the count, and the size of the index, and `buildmax info`
lists what each one is about.

A project holds at most **20 memories**, each with a description of at most 100
characters and a body of at most 2,000. The description is the part that is
always loaded, which is why it is short; the body is not, which is why it has
room for the reason.

Links between memories (`[[slug]]`) are a reading aid. Nothing resolves or
validates them, and a link to a memory that does not exist yet is fine — it
marks something worth writing.

### What goes in one

The agent is asked to keep stable preferences, decisions and their reasons,
corrections that have come up more than once, conventions that are not obvious
from the tree, and approaches already ruled out. The test is that the fact stays
true on any branch: a conclusion that only holds inside the branch that reached
it is session state, not project memory.

It is asked *not* to keep anything a file or a command would answer cheaply, the
state of the task in flight, narration, raw tool output, or credentials.

Each memory has a type:

| Type | Holds |
|---|---|
| `feedback` | Guidance you gave about how to work in this project |
| `project` | Ongoing work, goals, decisions, and constraints |
| `reference` | Pointers to dashboards, tickets, specifications |

A `feedback` memory records what you **want**, never what you **are**. "Prefers
the recommendation before the survey" is a preference you can correct;
"is unfamiliar with X" is a judgement about a person that nothing can check and
that would be acted on for months. The agent is told not to write the second
kind, and to cite the occasions any inference rests on so you can disagree with
it on evidence.

Saying "remember this for the project" is the clearest way to put something in.
Telling the agent to forget one works the same way; so does deleting the file.

### Turning it off

```bash
buildmax --no-project-memory            # this run only
buildmax -p "..." --no-project-memory

buildmax project forget <name>          # delete one
buildmax project forget --all           # delete them all
```

The run then carries no index and is offered neither memory tool. Deleting the
files has the same effect permanently, and `buildmax project forget` is the same
operation with the index regenerated for you. If the whole directory becomes
unreadable, the run says so at the start and carries neither the index nor the
tools — it will not add to a store it cannot see.

Clearing a project's sessions does not touch its memories, and deleting a memory
does not touch its sessions.

### What to know before using it

- **The index goes to your model on every call.** Every memory's name and
  description, on every turn, to whichever provider that session uses. A body is
  sent only when the agent opens it, which most turns will not do for most
  memories — a real reduction in what leaves the machine, but not the same as
  nothing.
- **The agent refuses to write anything that looks like a credential** — in the
  description as well as the body, since the description is the part sent every
  turn — but no check proves text is safe. Do not put secrets in a memory, and
  look through the files now and then if the project is sensitive.
- **A memory cannot grant anything.** Nothing written in one changes tool
  permissions, sandbox policy, hooks, or which plugins load. A file, a web page,
  or a tool result cannot ask to be remembered either.
- **Changing a memory means reading it first.** A replacement the agent has not
  read is refused, and so is one whose file changed since it read it — your own
  edit included. Two sessions recording different things never collide at all,
  because they are different files.
- **Subagents get none of it.** They are the highest-volume runs in a session; a
  parent that needs a delegate to know something says so in the task, which is
  more precise than handing over a catalogue.

## Related

- [CLI & TUI reference](cli.md) — `buildmax project` and `buildmax info`
- [Sessions & traces](sessions-and-traces.md) — what a run loaded is recorded
  in its `context_sources` trace record, by count and size only
- [Skills & subagents](skills-and-subagents.md) — instructions loaded on demand
  instead of on every run
