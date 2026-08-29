# Project Memory

> **Audience:** users · **Status:** current

The agent keeps one short document per project of things worth carrying into
later sessions: a preference you stated once, a decision and why, a library that
turned out not to work. It is shown to the model at the start of every turn, in
every session of that project.

It is **recall, not instruction**. The agent may have got it wrong, and what you
say now overrides it. For rules that must always be followed, use
[`AGENTS.md`](agents-md.md) instead — the agent never writes that file itself.

## What A Project Is

One project is one Git repository, including every worktree of it, or one plain
folder. A session started in a worktree reads and writes the same memory as one
started in the main checkout, because they are the same work.

Two clones of the same repository are two projects. So are two unrelated
folders. Nothing is grouped by a remote URL.

`buildmax doctor` names the project the current directory belongs to.

## Where It Lives

```text
<BUILDMAX_HOME>/projects/<project_id>/memory/MEMORY.md
```

It is ordinary Markdown, at most 8,192 characters, and it is yours: open it,
edit it, or empty it whenever you like. The next run reads whatever the file
says. `buildmax doctor` prints the exact path along with the current size.

A typical one is short:

```markdown
# Project Memory

## Preferences
- Prefer narrow table-driven Go tests when several cases share one contract.

## Decisions
- Sessions stay top-level on disk; project membership is a logical id.

## Dead ends
- Keying project identity by folder path splits worktrees into separate
  projects. Do not try it again.
```

## What Goes In It

The agent is asked to keep stable preferences, decisions and their reasons,
corrections that have come up more than once, conventions that are not obvious
from the tree, and approaches already ruled out.

It is asked *not* to keep anything a file or a command would answer cheaply, the
state of whatever task is in flight, narration, raw tool output, or credentials.

Saying "remember this for the project" is the clearest way to put something in.
Telling the agent to forget something works the same way; so does deleting the
line yourself.

## Turning It Off

```bash
buildmax --no-project-memory            # this run only
buildmax -p "..." --no-project-memory
```

The run then neither reads the document nor writes it, and the agent is not
offered the tool at all. Emptying `MEMORY.md` has the same effect permanently,
without deleting anything else.

Clearing a project's sessions does not clear its memory, and clearing its memory
does not touch its sessions.

## What To Know Before Using It

- **It is sent to your model on every call.** Whatever the document holds goes
  to whichever provider that session is configured against.
- **The agent refuses to write anything that looks like a credential**, but no
  check proves text is safe. Do not put secrets in it, and check it now and then
  if the project is sensitive.
- **It cannot grant anything.** Nothing written in the document changes tool
  permissions, sandbox policy, hooks, or which plugins load. A file, a web page,
  or a tool result cannot ask to be remembered either — only you and the agent's
  own judgment put lines in.
- **Two sessions cannot overwrite each other.** A write that raced another one
  is refused, and the agent merges into what it is shown next.

## Related

- [AGENTS.md](agents-md.md) — instructions, which memory never replaces
- [Sessions and traces](sessions-and-traces.md) — what a run loaded is recorded
  in its `context_sources` trace record, by size and revision only
