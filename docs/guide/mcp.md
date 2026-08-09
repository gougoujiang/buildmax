# MCP Servers

> **Audience:** users · **Status:** current

[Model Context Protocol](https://modelcontextprotocol.io/) servers give the
agent tools BuildMax does not ship — your issue tracker, your database, your
internal services.

## Configuration

MCP servers are declared in `mcp.json`, in either or both of:

| File | Scope |
|---|---|
| `<BUILDMAX_HOME>/mcp.json` | Every workspace on this machine |
| `<workspace>/.buildmax/mcp.json` | That workspace only |

Both files are **merged**, with the workspace entry winning when the same server
id appears in both. That is the useful shape: shared servers global, project
specific ones checked into the project.

```json
{
  "mcpServers": {
    "github": {
      "type": "stdio",
      "command": "npx",
      "args": ["-y", "@modelcontextprotocol/server-github"],
      "env": { "GITHUB_TOKEN": "$GITHUB_TOKEN" }
    },
    "internal-api": {
      "type": "http",
      "url": "https://mcp.internal/api"
    }
  }
}
```

| Field | Meaning |
|---|---|
| `type` | Transport — `stdio` for a local subprocess, or an HTTP/SSE transport |
| `command`, `args` | The process to start, for `stdio` |
| `env` | Environment for that process |
| `url` | Endpoint, for HTTP transports |

## Variable Expansion

`$VAR` and `${VAR}` are expanded in `command`, `args`, `env` values, and `url`,
against the process environment plus one built-in:

| Variable | Resolves to |
|---|---|
| `${WORKSPACE_ROOT}` | The workspace directory for this run |

This is how you keep secrets out of a checked-in `mcp.json` — reference
`$GITHUB_TOKEN` and let the environment supply it.

Note that the name carries no `BUILDMAX_` prefix. An unrecognized variable
expands to an empty string rather than failing, so a misspelled name shows up as
a path that begins at `/` instead of an error.

## How The Agent Uses Them

MCP tools are not injected into the prompt one by one. Two gateway tools handle
them:

- `LoadMcpTools` — discover what a connected server offers
- `CallMcpTool` — invoke one

This keeps the tool list small no matter how many servers are connected, at the
cost of one extra round trip when the agent first reaches for a server.

## Checking It Works

Run `/mcp` in the TUI to see connected servers and their status. A server that
fails to start shows up there rather than failing silently mid-run.

A small test server ships with the repository at `cmd/local-test-mcp-server`,
supporting stdio, SSE, and streamable HTTP — useful for verifying an integration
without a real backend.

## Notes

- An MCP server is a process you are starting with your credentials. Treat
  adding one like adding a dependency.
- Hook matchers see MCP calls as `CallMcpTool`, not as the underlying tool name.
- If a web fetch MCP tool is available, the agent is instructed to prefer it
  over the built-in `WebFetch`.

## Related

- [tools.md](tools.md) — the built-in tool set
- [hooks.md](hooks.md) — `mcp_tool` is also available as a hook transport
