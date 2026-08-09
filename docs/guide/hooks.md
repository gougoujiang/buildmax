# Hooks

> **Audience:** users and operators · **Status:** current — shipped
>
> Design rationale and event payload details:
> [design/031-hook-system-v2.md](../design/031-hook-system-v2.md)

Hooks let you observe — and for some events, **block** — what the agent does.
They are the mechanism behind "never let it run `rm -rf`", "format after every
edit", "send tool failures to our audit service", and "reject prompts containing
customer data".

## Where They Live

| Layer | File | Applies to |
|---|---|---|
| Global | `<BUILDMAX_HOME>/settings.yaml`, `hooks:` block | Every run on this machine |
| Workspace | `<workspace>/.buildmax/hooks.yaml` | Runs in that workspace |

The two layers are **additive**: global hooks run first, then workspace hooks.
A workspace cannot remove a global hook — which is what makes the global layer
usable as an operator control.

## Events

Thirteen events ship today. Four of them can block; the rest are advisory.

| Event | Gating | Fires when |
|---|---|---|
| `session_start` / `session_end` | — | A session opens or closes |
| `user_prompt_submit` | **yes** | A user prompt is about to enter the agent |
| `pre_tool_use` | **yes** | Before a tool executes |
| `post_tool_use` | — | A tool succeeded |
| `post_tool_use_failure` | — | A tool returned an error |
| `notification` | — | Approval needed, or permission denied |
| `pre_compact` | **yes** | Before context compaction |
| `post_compact` | — | After context compaction |
| `subagent_start` / `subagent_stop` | — | Subagent lifecycle |
| `stop` / `stop_failure` | — | The main agent finished |

Subagents inherit the parent's hooks, and every event is stamped with
`is_subagent` and `agent_type` so a hook can tell the difference.

## Transports

Each hook entry picks a `type`. Omitting it means `command`.

| `type` | Behavior |
|---|---|
| `command` | Run a shell command; the event JSON arrives on stdin |
| `http` | POST the event JSON to a URL |
| `mcp_tool` | Invoke a tool on a connected MCP server |
| `prompt` | A single-turn LLM call; `$ARGUMENTS` expands to the event JSON |

## The Block Contract

A gating hook denies the action by any of:

- `command` — exit code **2** (stderr becomes the reason shown to the model)
- `http` — status **422** (body becomes the reason)
- any transport — a JSON response `{"decision":"block","reason":"..."}`

**Anything else fails open.** A hook that times out, crashes, or returns garbage
allows the action. This is deliberate: a broken hook must not brick the agent.
If you need a hard denial, make the failure mode explicit rather than relying on
the hook being reachable.

## Matching and Timeouts

`matcher` is a regex on the tool name and applies to `pre_tool_use`,
`post_tool_use`, and `post_tool_use_failure`. An empty matcher matches every
call. `timeout` is in seconds and defaults to 30.

## Examples

Block risky tool calls with a central policy service, and format after edits:

```yaml
# <BUILDMAX_HOME>/settings.yaml
hooks:
  pre_tool_use:
    - type: http
      matcher: "Bash|Write|Edit"
      url: "https://policy.internal/check"
      headers:
        Authorization: "Bearer $POLICY_TOKEN"
      allowed_env: [POLICY_TOKEN]     # only listed env vars are interpolated
      timeout: 5

  post_tool_use:
    - type: command
      matcher: "Write|Edit"
      command: "gofmt -w ."

  post_tool_use_failure:
    - type: mcp_tool
      server: "audit"
      tool: "record_failure"
      input:
        tool: "${tool_name}"
        error: "${tool_error}"
        path: "${tool_args.path}"
```

Scan prompts before the model ever sees them:

```yaml
hooks:
  user_prompt_submit:
    - type: command
      command: "./.buildmax/hooks/redact.sh"    # exit 2 blocks the prompt
      timeout: 5
```

Use an LLM as a judge on shell commands — pick a cheap model, this fires on
every matching call:

```yaml
hooks:
  pre_tool_use:
    - type: prompt
      matcher: "Bash"
      model: ""            # empty = default model from settings.yaml
      prompt: |
        You are reviewing a tool call. Reply with one JSON object:
        {"decision":"allow"|"block","reason":"..."}
        Call:
        $ARGUMENTS
```

Only environment variables listed in `allowed_env` are interpolated into a hook
definition, so a hook config cannot quietly exfiltrate the whole environment.

## Full Example File

[`config-examples/settings.example.yaml`](../../config-examples/settings.example.yaml)
carries a commented walkthrough of every event and transport;
[`config-examples/hooks.workspace.example.yaml`](../../config-examples/hooks.workspace.example.yaml)
is the workspace overlay.

## Related

- [guide/sandbox.md](sandbox.md) — confining `Bash` rather than gating it
- [guide/tools.md](tools.md) — the exact tool names a `matcher` must match
- [reference/configuration.md](../reference/configuration.md) — where the file lives
