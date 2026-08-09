# Skills and Subagents

> **Audience:** users · **Status:** current

Two ways to give the agent more capability without making every prompt longer.

|  | Skill | Subagent |
|---|---|---|
| Is | Instructions loaded on demand | A separate agent run |
| Costs | Nothing until invoked | Its own context and tokens |
| Use for | A workflow you want done consistently | Work that would flood the main context |
| Invoked by | The `Skill` tool | The `Task` tool |

## Where They Live

Both are discovered from two layers, workspace first:

| | Skills | Subagents |
|---|---|---|
| Workspace | `<workspace>/.buildmax/skills/` | `<workspace>/.buildmax/agents/` |
| Global | `<BUILDMAX_HOME>/skills/` | `<BUILDMAX_HOME>/agents/` |

Checking `.buildmax/skills/` into a repository is how a team ships a workflow to
everyone who runs the agent there.

## Skills

One directory per skill, containing `SKILL.md` with frontmatter:

```markdown
---
name: release-check
description: "Pre-release verification: changelog, version bump, and green CI."
---

# Release Check

1. Confirm the version in the tag matches the built binary.
2. Verify the changelog has an entry for this version.
3. Check that CI is green on the release commit.
...
```

The `description` is what the model sees in the tool list, so it decides whether
the skill gets invoked at all. Write it as a trigger — when should this be used —
not as a title. The body is only loaded once the skill is actually invoked,
which is what makes skills cheap to have available.

Run `/skills` in the TUI to see what was discovered.

## Subagents

One markdown file per agent type, frontmatter plus a system prompt body:

```markdown
---
name: sample-researcher
description: Read-only research sub-agent for exploring the repo and fetching URLs.
tools: Glob, Grep, Read, WebFetch
---

You are a focused research sub-agent. Use Glob and Grep to locate relevant
files, Read to inspect them, and WebFetch only when the user needs information
from the public web. Summarize findings with paths and short quotes. Do not
modify files or run shell commands.
```

| Field | Meaning |
|---|---|
| `name` | The `subagent_type` value the main agent passes to `Task` |
| `description` | How the main agent decides to delegate here — write it as a trigger |
| `tools` | Comma-separated tool names. **Omit to grant every tool.** |
| `model` | Optional model override for this agent type |
| `max_iterations` | Optional cap; defaults to 50 |

Tool names must match exactly — `Glob, Grep, Read, WebFetch`, not `glob` or
`read_file`. See [tools.md](tools.md) for the list.

### Why Restrict Tools

The `tools` field is the useful part. A researcher with `Glob, Grep, Read`
cannot edit a file or run a command no matter what it is asked to do, so
delegating exploration to it is safe by construction rather than by prompt
wording.

### What Delegation Costs

A subagent is a full agent run: its own context, its own iterations, its own
token spend. What comes back to the main agent is only its final reply. That is
the point — a subagent can read forty files and return a paragraph — but it also
means the main agent cannot see the work, only the conclusion.

Subagents inherit the parent's hooks, and every hook event they raise is stamped
with `is_subagent` and `agent_type`, so a `pre_tool_use` hook applies to them
too.

## Related

- [tools.md](tools.md) — exact tool names for the `tools:` field
- [agents-md.md](agents-md.md) — instructions that apply to every run instead
- [hooks.md](hooks.md) — `subagent_start` / `subagent_stop` events
