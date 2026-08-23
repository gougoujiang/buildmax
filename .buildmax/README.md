# `.buildmax/` — This Repository's Own Agent Configuration

BuildMax runs on itself. When you run `buildmax` with this repository as the
workspace, the agent picks up everything in this directory: it is the
**workspace-level** half of the configuration described in
[docs/guide/skills-and-subagents.md](../docs/guide/skills-and-subagents.md) and
[docs/guide/mcp.md](../docs/guide/mcp.md), the other half being your personal
`~/.buildmax/`.

It is checked in on purpose — as working configuration for anyone hacking on the
project, and as a real, non-toy example of what these files look like.

| Path | What it is |
|---|---|
| `mcp.json` | An MCP server definition pointing at `cmd/local-test-mcp-server`, so `/mcp` in the TUI has something real to connect to. Discovery order and the `${WORKSPACE_ROOT}` expansion are in [docs/guide/mcp.md](../docs/guide/mcp.md). |
| `agents/sample-researcher.md` | A read-only subagent (`Glob`, `Grep`, `Read`, `WebFetch`) the `Task` tool can delegate to. Also the reference for the frontmatter format. |
| `skills/smoke/` | `/smoke [level]` — the manual tool smoke test `./make agent-smoke` drives. Levels 0–3 go from read-only tools and session state, to file I/O, to edge cases and the permission boundary, to delegation, background jobs, and MCP. `agent-smoke` runs level 0; the higher levels are manual, and level 3 needs the TUI or Desktop. The report is written by the model, so check it against the run's `Tool calls:` count — a small model will fabricate a clean PASS in one call. |
| `skills/vibe/` | `/vibe` — a local development-lifecycle workflow (start, clarify, design, code, done), with commit and push available only as explicit commands. Optional; it is one maintainer's working style, not a project requirement. |

## Why This Is Committed While `.claude/` And `.vibe/` Are Not

`.gitignore` excludes `.claude/`, `.cursor/`, and `.vibe/` because those are
*your* local state: your assistant's settings, and the scratch notes a workflow
produces. `.buildmax/` is different — it is configuration **for this project's
own agent**, it affects how the repository behaves for everyone, and it doubles
as documentation. `/vibe` writes its notes to `.vibe/` at the repository root,
which stays gitignored: anything worth keeping belongs in `docs/`.

Nothing here is required to build, test, or contribute. Ignore the whole
directory and every `./make` command still works.
