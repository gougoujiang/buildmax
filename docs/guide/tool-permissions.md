# Tool Permissions

> **Audience:** users · **Status:** current
>
> Design rationale and the layering that produces these answers:
> [design/tool-permissions.md](../design/tool-permissions.md)

BuildMax asks before a tool call that changes something, and only where there
is somebody to ask. Reads run unannounced; writes stop and wait.

This page is about changing that: quieting a prompt you do not need, adding one
you do, and finding out why a call was refused.

## What Prompts, Out Of The Box

Run this to see the answer for your machine rather than reading it here:

```bash
buildmax tools status
```

```text
TOOL       ACCESS     ACTION  SOURCE
Read       read-only  allow   derived
Write      write      ask     derived
Edit       write      ask     derived
Bash       write      allow   derived
Task       write      ask     derived
TodoWrite  write      allow   derived
```

Three columns worth reading carefully:

- **ACCESS** is what the tool says the call does. Read-only tools never prompt.
- **ACTION** is the category answer, with a person present. Tools that inspect
  their arguments can still differ per call — see the two exceptions below.
- **SOURCE** is `derived` when BuildMax worked it out, or `settings` when you
  configured it.

Two entries in that table are not what they look like:

- **`Bash` says `allow`** because it judges each command instead of the
  category. An ordinary `ls` runs; a risky command asks; a catastrophic one is
  refused outright. It would otherwise prompt for every `git status`.
- **`TodoWrite` and `NoteWrite` say `allow`** although they are writes. What
  they write is the agent's own scratch state, not your files.

## Answering A Prompt

```text
Tool: Write
  file_path: internal/server/routes.go

Allow once(y)  Allow session(a)  Deny(n)    ←→ select  enter: confirm
```

`a` is the one to reach for. It stops asking about that tool for the rest of
the session, and it is forgotten when BuildMax exits — nothing is written to
disk. For an MCP call it covers that one server and tool, not every MCP tool
you have configured.

## Making It Permanent

When a grant is one you would give every session, put it in
`<BUILDMAX_HOME>/settings.yaml`:

```yaml
tools:
  permissions:
    Write: allow                        # stop asking before file writes
    Edit: allow
    Bash: deny                          # no shell at all
    "CallMcpTool:github/*": allow       # trust one server's tools
    "CallMcpTool:jira/delete_issue": deny
```

Keys are tool names, or a tool plus the target it dispatches to, with an
optional trailing `*`. Case does not matter. The most specific rule wins: an
exact target, then the longest matching pattern, then the bare tool name.

`buildmax tools status` will then show `settings` in the SOURCE column, and
will list any rule it had to ignore.

### `allow` quiets the prompt, not the safety checks

`Read: allow` means "stop asking me about reads". It does **not** mean "open
`~/.ssh/id_rsa` without telling me" — a sensitive path still prompts, and so
does a risky shell command. Only `deny` outranks those checks.

If you want a tool gone, `deny` is the setting. It refuses the call and tells
the model why, which is usually more useful than the tool quietly not existing.

## MCP Calls

An MCP server describes each of its tools, and can mark one read-only. BuildMax
uses that: a tool the server advertises as read-only runs unprompted, anything
else asks.

Two things follow from it being the server's own claim:

- **A server that omits the annotation is treated as writing.** The protocol
  cannot distinguish "not read-only" from "did not say", so both prompt. A
  well-behaved read-only server that forgot the annotation will ask every time
  — allow it for the session, or write a rule.
- **The claim decides whether you are asked, never whether the call is
  trusted.** A rule you write is the only thing that grants anything.

## Unattended Runs

Print mode (`buildmax -p`), task runs on a worker, and Portal conversations
have nobody to ask. They raise no category prompt at all: `Write` and `Edit`
behave exactly as they always have.

Two things do change there:

- **`ask` in your settings means a person must look**, so where there is no
  person the call is refused rather than run. Use `allow` if you meant "let it
  through unattended".
- **An MCP call the server does not advertise as read-only is refused.** This is
  the one place unattended behavior tightened. There is currently no way to
  override it for a worker — see
  [design/tool-permissions.md](../design/tool-permissions.md) section 7.

Risky shell commands were already refused on these surfaces and still are.

## When A Call Is Refused

The model is told why, in the tool result, and the reason names the layer:

| Message | Means |
|---|---|
| `denied by policy` | a `deny` rule, or `ask` with nobody to ask |
| `denied by user` | you pressed `n` |
| `denied by hook` | a `PreToolUse` hook blocked it — see [hooks.md](hooks.md) |
| `blocked — repeated identical call` | the loop guard, not permissions |

`buildmax tools status` shows which rule is in force; `/tools` in the TUI marks
any tool that does not simply run.

## Related

- [tools.md](tools.md) — what each built-in tool does
- [hooks.md](hooks.md) — block calls with your own logic, after permissions pass
- [sandbox.md](sandbox.md) — confine what `Bash` can reach, rather than whether it runs
- [reference/configuration.md](../reference/configuration.md) — the full `tools.permissions` reference
