# Bash Sandbox

> **Audience:** users and operators · **Status:** current — **off by default on
> every surface**
>
> Design rationale, backend details, and remaining gaps:
> [design/032-sandbox-and-execution-boundaries.md](../design/032-sandbox-and-execution-boundaries.md)

The sandbox confines the subprocesses started by the `Bash` tool: which paths
they may read and write, and which network destinations they may reach. It is
the strongest boundary BuildMax offers around model-chosen shell commands.

Two things to be clear about before you rely on it:

- **It covers `Bash` only.** Other tools (`Read`, `Write`, `Edit`, `Glob`,
  `Grep`) keep their own path checks — the workspace root boundary — and
  are not affected by these settings.
- **It is disabled by default,** including on workers. Turning it on is a
  deliberate act.

## Availability

| Platform | Backend | Status |
|---|---|---|
| macOS | Seatbelt (`sandbox-exec`) | Supported |
| Linux, WSL2 | `bwrap` (bubblewrap) | Supported |
| Native Windows | — | Unavailable |

Check your host before configuring anything:

```bash
buildmax sandbox deps      # are bwrap / sandbox-exec / socat present?
buildmax sandbox status    # resolved config, and which layer set each value
```

## Turn It On

```bash
buildmax sandbox enable
buildmax sandbox mode auto_allow
```

Or edit the `sandbox:` block in `<BUILDMAX_HOME>/settings.yaml` directly. When
the sandbox is active the TUI footer shows the mode.

### Modes

| Mode | Behavior |
|---|---|
| `auto_allow` | A command that runs sandboxed skips the approval prompt — the boundary is the confinement, not your attention |
| `regular` | Normal approval behavior is kept even when sandboxed |

`auto_allow` is the mode that actually changes how the agent feels to use: you
stop approving every command because the blast radius is already bounded.

## Boundaries

```yaml
# <BUILDMAX_HOME>/settings.yaml
sandbox:
  enabled: true
  fail_if_unavailable: false          # true = refuse to run bash unsandboxed
  auto_allow_bash_if_sandboxed: true
  allow_unsandboxed_commands: false   # gate for the per-call escape hatch
  excluded_commands: []               # commands that never get sandboxed

  filesystem:
    allow_write: ["."]
    deny_write:  ["~/.ssh", "~/.aws"]
    allow_read:  ["."]
    deny_read:   ["~/.ssh"]

  network:
    allowed_domains: ["api.github.com", "proxy.golang.org"]
    denied_domains:  []
    allow_local_binding: false
    allow_all_unix_sockets: false
```

Network control works by routing egress through a Go-side HTTP/SOCKS proxy, so
domain rules apply to ordinary tools inside the sandbox without per-tool
support. Environment variables that look like secrets (`*_TOKEN`, `*_KEY`,
`*_SECRET`, and BuildMax's own credentials) are scrubbed from the child
environment unless you list them explicitly.

## Operator Policy

`<BUILDMAX_HOME>/policy.yaml` holds a sandbox block with the same shape that
**overrides** `settings.yaml`. `BUILDMAX_SANDBOX_ENABLED` overrides both. Use
the policy file when the machine's owner and the machine's user are different
people.

Two keys make the policy layer authoritative rather than merely additive:
`allow_managed_read_paths_only` and `allow_managed_domains_only` cause lower
layers' `allow_read` and `allowed_domains` entries to be ignored.

## The Escape Hatch

A single call can request `dangerously_disable_sandbox`. It is honored **only**
when `allow_unsandboxed_commands: true`. Leave that false and the flag is inert
— which is the point of having it be config-gated rather than a runtime
decision.

## Known Gaps

The sandbox is genuinely useful today, but it is not finished:

- workers do not yet default to the stricter profile
- process rlimits are not wired
- hook transports are not themselves sandboxed

Track the remaining work in
[design/032 §13.1](../design/032-sandbox-and-execution-boundaries.md). Do not
treat the sandbox as a substitute for reviewing what a deployment is allowed to
reach.

## Related

- [guide/hooks.md](hooks.md) — blocking a command instead of confining it
- [deploy/overview.md](../deploy/overview.md) — deployment-level boundaries
