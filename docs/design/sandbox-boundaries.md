# Sandbox And Execution Boundaries

## Status

- roadmap_priority: `P0.5`
- status: `phases A–E implemented` (§13; phase F worker hardening + docs still
  open, plus the gaps listed there — modelled on
  [Claude Code's sandbox docs](https://code.claude.com/docs/en/sandboxing))
- follows: [trust-harness.md](./trust-harness.md), [hook-system.md](./hook-system.md)
- roadmap: [../ROADMAP.md](../ROADMAP.md)
- created_at: `2026-05-23`

## 1. Purpose

P0.5 §3.2 asks for explicit sandbox modes for command and tool execution
with first-class boundaries on workspace filesystem, external directories,
network, env vars, process limits, and worker/container execution.

Today's Agent Core has:

- **Filesystem confinement** for in-process file tools via
  `internal/util.ResolvePath` (workspace root only).
- **Bash heuristics** in `internal/tool/safety.go` (catastrophic deny +
  risky ask). Commands still run with full parent env, full network,
  no rlimits, no FS isolation.

We adopt Claude Code's model verbatim where reasonable. Quoting the
[docs](https://code.claude.com/docs/en/sandboxing): "The sandbox
isolates Bash subprocesses. Other tools operate under different
boundaries." That is the design we mirror:

- **Sandbox = Bash subprocess isolation.** Bash commands and their
  children run under an OS sandbox (`bwrap` on Linux/WSL2,
  `sandbox-exec`/Seatbelt on macOS), bounded by a settings-driven
  filesystem and network policy. The bash sandbox is the only thing
  this doc adds to enforcement.
- **Non-bash tools keep using the permission system.** Read/Write/
  Edit/Glob/Grep/WebFetch keep their current Go-level checks
  (`ResolvePath`, `safety.go`) plus the approval policy in
  `internal/agentapp/policy.go`. They are not retrofitted onto a new
  guard layer.

This keeps blast radius small while delivering the capability users
expect when they say "sandbox."

## 2. Direction

Three principles, lifted from the Claude Code docs:

1. **Sandbox the shell, not the args.** Pattern-matching bash strings
   is unreliable; OS-level isolation is the durable answer. macOS uses
   Seatbelt; Linux/WSL2 uses `bubblewrap` + `socat`.
2. **Opt-in for users, opt-out for operators.** Default `enabled:
   false` on local CLI/Desktop (no regression). **Default `enabled:
   true` with `fail_if_unavailable: true` on the worker** to satisfy
   the trust harness's "stricter than trusted local."
3. **Visible failure beats silent fail-open.** When `enabled: true`
   but the backend can't start (missing `bwrap`, unsupported platform),
   show a clear startup message. On the worker, refuse to start.

We do **not** ship our own `@anthropic-ai/sandbox-runtime` clone. We
shell out to `bwrap` / `sandbox-exec` from Go, with a small Go-side
HTTP/SOCKS proxy for network egress filtering.

## 3. Architectural shape

Mirrors the hooks v2 layout:

| Layer | Hooks v2 | Sandbox |
|---|---|---|
| Domain contract | `core/agent/hook.go` | `core/agent/sandbox.go` — `SandboxConfig`, `SandboxView` interface |
| External-system impl | `infra/hook/*` | `infra/sandbox/*` — `manager.go` (façade), `bwrap_linux.go`, `seatbelt_darwin.go`, `unsupported_windows.go`, `proxy.go` (HTTP/SOCKS), `violation_store.go`, `deps.go` (dependency check) |
| Application assembly | `agentapp/hook_manager.go` | `agentapp/sandbox.go` — resolve+merge settings, inject manager into Bash tool, expose `Status` |
| Config | `config/hooks.go` + `<ws>/.buildmax/hooks.yaml` | `config/sandbox.go` — `sandbox:` block inside the existing `settings.yaml` (and optional `policy.yaml`) |

`infra/sandbox.Manager` mirrors Claude Code's `SandboxManager`:
- `Enabled() bool`, `Mode() string` (`"auto_allow"` | `"regular"`)
- `WrapBashCommand(ctx, cmd, shell) (wrapped string, error)`
- `ShouldSandboxCommand(cmd) bool` (honors `excluded_commands`)
- `Dependencies() DepsReport`, `UnavailableReason() string`
- `Refresh(cfg) error`, `Reset()`, `Close()`
- `Violations() *ViolationStore`

Only the Bash tool and the `command` hook transport depend on
`SandboxView` (so `command` hooks can't be used to escape).

## 4. Configuration

Sandbox config lives inline in the existing `settings.yaml`, the same
way Claude Code's `sandbox` block lives inside `settings.json`. Keys are
snake_case per CLAUDE.md §6.1 but the structure is identical to upstream.

### 4.1 Sources

| Scope | Path | Notes |
|---|---|---|
| User | `<BUILDMAX_HOME>/settings.yaml` under `sandbox:` | default location |
| Policy (operator) | `<BUILDMAX_HOME>/policy.yaml` under `sandbox:` | optional lock-out |
| Env | `BUILDMAX_SANDBOX_ENABLED` | per-process override |
| CLI | `dangerously_disable_sandbox: true` as a per-call bash arg | per-invocation escape hatch |

Resolution: env and policy override user. Policy file may set
`allow_managed_domains_only: true` and `allow_managed_read_paths_only:
true` to lock specific arrays — when set, lower sources can still
**add deny entries** but their **allow entries are ignored** for that
field (matches Claude Code's `allowManagedDomainsOnly` /
`allowManagedReadPathsOnly`).

For array keys without a managed-only flag (`excluded_commands`,
`allow_write`, etc.), entries from every scope are unioned — same as
Claude Code.

A future per-workspace `<workspace>/.buildmax/settings.yaml` is the
right place for project-specific overrides; we do not introduce it in
this doc, and we do not create a sandbox-only workspace file.

### 4.2 Full key reference

Mirrors `SandboxSettings` in the Claude Code SDK
(`src/entrypoints/sandboxTypes.ts`). Keys we drop from upstream are
called out at the end of this section.

```yaml
sandbox:
  enabled: false              # master switch
  fail_if_unavailable: false  # refuse to start if backend can't run
  auto_allow_bash_if_sandboxed: true   # skip approval prompt for sandboxed bash
  allow_unsandboxed_commands: true     # honor dangerously_disable_sandbox

  excluded_commands: []       # bash patterns to run *outside* the sandbox
                              # (convenience, not a security boundary)

  filesystem:
    allow_write: []           # extra writable paths beyond CWD
    deny_write: []            # explicit denials (override allow)
    allow_read: []            # re-allow within deny_read regions
    deny_read: []             # block reads
    allow_managed_read_paths_only: false  # policy-only knob

  network:
    allowed_domains: []       # host globs (e.g. "api.github.com", "*.npmjs.org")
    denied_domains: []
    allow_unix_sockets: []    # macOS only
    allow_all_unix_sockets: false
    allow_local_binding: false
    http_proxy_port: 0        # 0 = pick free port; non-zero = use custom proxy
    socks_proxy_port: 0
    allow_managed_domains_only: false  # policy-only knob

  ignore_violations: {}       # map[string][]string of violations to suppress
                              # (per-tool, claude-code parity)

  enable_weaker_nested_sandbox: false   # allow inside Docker w/o privileged ns
  enable_weaker_network_isolation: false # macOS: allow trustd for Go CLIs
```

Keys we **do not** ship from upstream `SandboxSettings`:
- `enabledPlatforms` (Claude Code's NVIDIA-rollout escape; we don't
  need it yet).
- `ripgrep` override (we don't bundle ripgrep yet).

### 4.3 Path prefix conventions

For `sandbox.filesystem.*` paths we match Claude Code exactly:

| Prefix | Meaning | Example |
|---|---|---|
| `/path` | absolute from filesystem root | `/tmp/build` |
| `~/path` | relative to home directory | `~/.cache` |
| `./path` or `path` | relative to settings file directory (project root for `<workspace>/.buildmax/settings.yaml` when it exists; `<BUILDMAX_HOME>` for user settings) | `./output` |

We deliberately do **not** copy upstream's `//path` permission-rule
convention (their special "absolute via permission rule" prefix).
BuildMax has no `Edit(/path)` permission-rule grammar yet, so the
ambiguity that motivated `//path` does not arise.

### 4.4 Defaults

Out of the box, with `enabled: true` and nothing else set, the
behavior matches Claude Code's defaults:

- **Filesystem write**: only the current working directory and its
  subtree.
- **Filesystem read**: entire computer, minus `deny_read`.
  *Note*: this default still allows reads of `~/.aws/credentials`,
  `~/.ssh/`, etc. Operators who care must add those to `deny_read`
  themselves. We will document this caveat in
  `config-examples/sandbox.example.yaml` and `CLAUDE.md`.
- **Network**: no domains pre-allowed. Each new domain prompts via
  the existing approval flow (interactive surfaces) or denies
  outright (non-interactive — `applyPolicyAndExecute` already
  collapses Ask→Deny when no ApprovalHandler is set).
- **Settings-file self-protection**: `<BUILDMAX_HOME>/settings.yaml`
  and `<BUILDMAX_HOME>/policy.yaml` are auto-added to `deny_write`
  at every scope.

## 5. Sandbox modes

Two modes (mirrors Claude Code's `/sandbox` Mode tab):

- **`auto_allow`** — sandboxed bash commands run without prompting.
  Auto-approval is conditional: explicit `deny` permission rules
  still apply, and bash invocations that match the protected paths in
  `tool/safety.go` (`rm -rf /`, etc.) still go through the regular
  approval flow.
- **`regular`** — sandboxed bash still routes through the regular
  approval flow. Sandbox is enforcement; approval is still required.

Mode is selected by `sandbox.auto_allow_bash_if_sandboxed`:
- `true` (default when `enabled: true`) → `auto_allow`
- `false` → `regular`

Both modes apply the same OS-level FS + network restrictions; only
the approval behavior differs.

## 6. The `dangerously_disable_sandbox` escape hatch

Mirrors Claude Code's `dangerouslyDisableSandbox`.

The Bash tool accepts an optional argument:

```json
{ "command": "docker ps", "dangerously_disable_sandbox": true }
```

Behavior:

- If `sandbox.enabled: false` → ignored; bash runs as today.
- If `sandbox.allow_unsandboxed_commands: false` → ignored; the call
  is sandboxed regardless. Surfaced as "Strict sandbox mode" in
  status (matches the `/sandbox` Overrides tab label).
- Otherwise → bash runs **outside** the sandbox and goes through the
  normal approval flow (Ask→user prompt; non-interactive→Deny).

The LLM is told about this knob in the Bash tool description so it
can retry self-recovery, per Claude Code's "Claude analyzes the
failure and may retry the command with the `dangerouslyDisableSandbox`
parameter."

Every disabled call writes a `Violation{kind: sandbox_disabled}` to
the violation store so audit hooks described in [hook-system.md](./hook-system.md) see it.

## 7. How the OS backend works

We follow Claude Code's platform support exactly: macOS, Linux,
WSL2. Native Windows is **not** supported; Windows users run
BuildMax inside WSL2 or with `enabled: false`.

### 7.1 macOS — Seatbelt

`infra/sandbox/seatbelt_darwin.go` generates a Seatbelt profile
(`.sb`) per session under `<BUILDMAX_HOME>/sandbox/profiles/`
(mode 0600), then invokes:

```
sandbox-exec -f <profile.sb> /bin/sh -c <wrapped-command>
```

The profile derives from `FSConfig` + `NetConfig` + `EnvConfig`,
using `(allow file-read*)`, `(allow file-write*)`,
`(deny network-outbound)` plus targeted allows for the proxy port.

`enable_weaker_network_isolation` opens `com.apple.trustd.agent` so
Go CLIs (`gh`, `gcloud`, `terraform`) can verify TLS through MITM
proxies — same trade-off Claude Code documents.

### 7.2 Linux / WSL2 — bubblewrap + socat

`infra/sandbox/bwrap_linux.go` builds `bwrap` argv from settings:
`--ro-bind`, `--bind`, `--dev`, `--proc`, `--unshare-net`,
`--setenv`, `--rlimit`. Network egress goes through the Go-side
HTTP/SOCKS proxy via `HTTP_PROXY` / `HTTPS_PROXY` / `ALL_PROXY` env
inside the sandbox; `socat` relays Unix-domain proxy sockets where
needed.

Required packages on the host:
- `bubblewrap` (the `bwrap` binary)
- `socat`

Optional:
- seccomp filter (for Unix-socket blocking) installed via the
  upstream `@anthropic-ai/sandbox-runtime` package or manually
  copying its `vendor/seccomp/*` files; `infra/sandbox/deps.go`
  reports it as "optional" in status.

### 7.3 Ubuntu 24.04 + AppArmor

Documented caveat from upstream: Ubuntu 24.04's default AppArmor
profile blocks unprivileged user namespaces. Detection: read
`/proc/sys/kernel/apparmor_restrict_unprivileged_userns`; if `1`,
report a clear remediation message in `buildmax sandbox` status
(quotes the upstream `tee` snippet for adding a `bwrap` profile).

### 7.4 Containers — `enable_weaker_nested_sandbox`

Mirrors upstream. When set, bwrap bind-mounts the container's
existing `/proc` instead of mounting a fresh one. Documented as a
weaker option; only use when the outer container already provides
isolation.

## 8. The `buildmax sandbox` command

Mirrors Claude Code's `/sandbox` panel.

Subcommands:

- `buildmax sandbox status` — Config tab equivalent. Prints
  resolved settings, source layers, mode, backend, dependency
  check, recent violations.
- `buildmax sandbox deps` — Dependencies tab equivalent. Shows
  whether `bwrap`, `socat`, ripgrep, seccomp filter are present
  with per-platform install hints.
- `buildmax sandbox mode <auto_allow|regular>` — writes
  `auto_allow_bash_if_sandboxed` to user settings.
- `buildmax sandbox overrides <strict|permissive>` — writes
  `allow_unsandboxed_commands` to user settings.
- `buildmax sandbox enable` / `disable` — writes
  `sandbox.enabled`.

When a required dependency is missing, `status` and `deps` are the
only useful commands (matches upstream's "the Dependencies tab is
the only tab shown until you install it").

## 9. Boundary enforcement summary

| Category | Enforced by | Where |
|---|---|---|
| Bash filesystem | OS backend (`bwrap` bind/ro-bind; Seatbelt `file-read*` / `file-write*`) | `infra/sandbox/bwrap_linux.go`, `seatbelt_darwin.go` |
| Bash network egress | Go HTTP/SOCKS proxy (allow_list/deny_list); sandbox forces `HTTP_PROXY` env | `infra/sandbox/proxy.go` |
| Bash env | Built via `BuildChildEnv`; secret-shaped vars (`*_TOKEN`, `*_KEY`, `*_SECRET`, plus `BUILDMAX_API_KEY`, `BUILDMAX_WORKER_TOKEN`, `BUILDMAX_JWT_SECRET`, `AWS_SECRET_ACCESS_KEY`) filtered out unless explicitly listed | `core/agent/sandbox.go` |
| Bash process limits | `syscall.Setrlimit` on Unix; `bwrap --rlimit` on Linux | `infra/sandbox/unix_rlimit.go` |
| `command` hook | Same wrap + env as Bash (so hooks can't escape) | `infra/hook/command.go` consults `SandboxView` |
| `http` hook | Same `allowed_domains` / `denied_domains` matcher | `infra/hook/http.go` consults `SandboxView` |
| Non-bash tools (Read/Write/Edit/Glob/Grep/WebFetch) | **Existing permission flow** (`policy.go`, `safety.go`, `util.ResolvePath`) — unchanged by this doc | `internal/tool/*` |

The two layers (sandbox for bash; permissions for everything else)
mirror Claude Code's docs exactly: "Built-in file tools: Read, Edit,
and Write use the permission system directly rather than running
through the sandbox."

## 10. Surface defaults

| Surface | Default | Locked? |
|---|---|---|
| CLI (`buildmax`) | `enabled: false` (no regression) | no |
| Desktop | `enabled: false` | no |
| Worker (`buildmax-worker`) | `enabled: true`, `fail_if_unavailable: true`, `allow_unsandboxed_commands: false`, narrow `allowed_domains`, `deny_read: ["~/.aws", "~/.ssh"]` | yes, via `policy.yaml` shipped with the worker container image |
| Portal in-process turn | inherits worker defaults | yes |
| TUI subagent | inherits parent | n/a |

The worker config is the concrete realization of [trust-harness.md](./trust-harness.md) §3.2's
"stricter default than trusted local." A worker that resolves to
`enabled: false` because of an override emits a startup `WARN`,
stamps the trace `sandbox.downgraded=true`, and exposes the
overriding source in `SessionStart` hook payload (see §11.2).

## 11. Visibility

### 11.1 Surface display

- **CLI / TUI footer**: `sandbox: on(auto)`, `sandbox: on(regular)`,
  `sandbox: off`, or `sandbox: missing` (enabled but backend
  unavailable — matches upstream warning behavior).
- **Desktop**: session info panel surfaces the same plus a "View
  boundaries" disclosure.
- **Portal task-run header**: shows enabled? + mode + count of
  allowed domains and write paths.
- **Worker startup logs**: full resolved config JSON + dependency
  check result.

### 11.2 Hook payload

Every `agent.HookInput` gains a `Sandbox` field:

```go
type SandboxInfo struct {
    Enabled     bool
    Mode        string   // "auto_allow" | "regular"
    Backend     string   // "seatbelt" | "bwrap" | "none"
    Sources     []string // ordered chain of overrides applied
    Downgraded  bool     // worker only; true if forced off by override
}
```

### 11.3 Violation store + `ignore_violations`

Bounded ring buffer keyed by session id:

```go
type Violation struct {
    Time     time.Time
    Kind     string  // "fs_deny" | "net_deny" | "sandbox_disabled" | "backend_unavailable"
    Tool     string
    Argument string  // redacted
    Reason   string
    Source   string  // "go_proxy" | "bwrap" | "seatbelt"
}
```

`sandbox.ignore_violations` is honored exactly as upstream: a
`map[string][]string` (e.g. `{ "Bash": ["net_deny:metrics.example.com"] }`)
suppresses matching violations from status/trace display. They are
still counted internally for debugging.

## 12. Hook integration

Sandbox and hooks are complementary:

- Sandbox is a *contract-level deny*: deterministic, OS-enforced,
  shaped for the LLM (CLAUDE.md §6.4).
- Hooks are *imperative checks*: policy callouts, audit, formatters
  ([hook-system.md](./hook-system.md)). They run **after** sandbox accepts. A sandbox deny
  fires `Notification{kind: sandbox_denied}` so audit hooks still
  see it.

Hook transports honor the sandbox so they can't be used to bypass it
(see §9 — `command` and `http` hooks consult `SandboxView`).

## 13. Implementation steps

Phased so each step is independently shippable. Each phase ends in a
working binary on Linux + macOS.

Phases A–E are implemented (commit `ef9617a`). Deviations from the plan
below, and everything still open, are collected in §13.1.

**Phase A — Config + contract, no behavior change.** ✅
- `core/agent/sandbox.go`, `core/agent/sandbox_defaults.go`.
- `config/sandbox.go` + `LoadPolicySandbox` + `ResolveSandbox` +
  tests.
- `agentapp.NewAgentApp` resolves and stores the config; exposes
  `Sandbox() agent.SandboxConfig`. Runtime unchanged.
- `buildmax sandbox status` shows resolved config + sources.

**Phase B — macOS Seatbelt + Linux bwrap backend.** ✅
- `infra/sandbox/{seatbelt_darwin.go, bwrap_linux.go,
  unsupported_windows.go, deps.go, manager.go}`.
- Bash tool wraps via `WrapBashCommand` when
  `Manager.Enabled() && Manager.ShouldSandboxCommand(cmd)`.
- Implement `excluded_commands` matching (mirrors upstream
  `bashPermissionRule` semantics).
- `sandbox-exec` profile golden tests; `bwrap` argv golden tests.
- `buildmax sandbox deps` subcommand.

**Phase C — Network proxy.** ✅
- `infra/sandbox/proxy.go` (HTTP + SOCKS5, allow/deny matcher,
  violation emission).
- `bwrap --unshare-net`, Seatbelt `(deny network-outbound)` +
  targeted allow to proxy port.
- Tests: denied host returns proxy-friendly 403; `Refresh()` picks
  up new allow_list without restart.

**Phase D — Env scrubbing, rlimits, dangerously_disable_sandbox,
ignore_violations.** ⚠️ (rlimits not implemented — see §13.1)
- `core/agent.BuildChildEnv` + secret denylist.
- `infra/sandbox/unix_rlimit.go` + windows noop.
- Bash tool: `dangerously_disable_sandbox` arg + status surfacing
  "strict sandbox mode."
- `ignore_violations` filter on display.

**Phase E — Auto-allow mode + violation store + surfaces.** ✅
- Bash `CheckArgs` returns `ToolActionAllow` when
  `Manager.Enabled() && mode==auto_allow && ShouldSandboxCommand(cmd)`
  unless the command matches the always-prompt list (`rm -rf /`,
  etc., kept from `safety.go`).
- TUI footer; `buildmax sandbox mode` / `enable` / `disable`.
- `SessionStart` hook payload populated with `SandboxInfo`.

**Phase F — Worker hardening + docs.** ❌ not started
- Worker bootstrap: hard-code `enabled: true,
  fail_if_unavailable: true, allow_unsandboxed_commands: false`
  unless explicitly overridden by `policy.yaml`.
- WARN + trace mark on downgrade.
- `config-examples/sandbox.example.yaml`, CLAUDE.md §4.1 update,
  ROADMAP.md update.

### 13.1 Implementation state

Landed in `ef9617a`: `core/agent/sandbox.go` (`SandboxView`, `NoopSandbox`,
`SandboxInfo`), `config/sandbox.go` (load + policy merge + `ResolveSandbox`
with per-surface defaults), `infra/sandbox/` (manager façade, Seatbelt and
bwrap backends, deps report, HTTP/SOCKS proxy, host matcher, excluded-command
matcher, env scrubber, violation store), `agentapp/sandbox.go`
(`SandboxStatus`, manager construction), bash-tool wrapping with auto-allow
demotion and `dangerously_disable_sandbox`, the TUI footer tag, and
`buildmax sandbox status|deps|mode|enable|disable`.

Still open — these block §15 acceptance:

1. **Worker default is not stricter than local.** `config.SandboxSurfaceWorker`
   exists and carries the hardened baseline, but nothing selects it:
   `agentapp/taskrun/runtime.go` builds its AgentApp with an empty
   `SandboxSurface`, which resolves to the CLI baseline (`enabled: false`).
   This is the single most load-bearing gap — [trust-harness.md](./trust-harness.md) §3.2's "stricter default
   on the worker" is currently not true.
2. **Process limits are absent.** No `infra/sandbox/unix_rlimit.go`, no
   `Setrlimit` call anywhere. Phase D shipped without it, so the "process
   execution limits" boundary in [trust-harness.md](./trust-harness.md) §3.2 is
   unenforced.
3. **Hook transports do not consult `SandboxView`.** §9 and §12 require the
   `command` and `http` drivers to honor the sandbox so hooks cannot be used
   to escape it; `infra/hook/command.go` and `infra/hook/http.go` have no
   sandbox reference today.
4. **`buildmax sandbox overrides <strict|permissive>`** (§8) is not
   implemented; `allow_unsandboxed_commands` can only be edited by hand.
5. **Docs from phase F**: no `config-examples/sandbox.example.yaml`, and
   AGENTS.md §4.1 documents the sandbox only as of this pass.

Naming deviations from the plan, harmless: the unsupported-platform stub is
`unsupported_other.go` (not `unsupported_windows.go`), and the env denylist
lives in `infra/sandbox/env_scrub.go` as `ScrubEnvList` rather than
`core/agent.BuildChildEnv`.

**Deferred (post-P0.5).**
- Native Edit/Read/WebFetch permission-rule grammar to feed
  `sandbox.filesystem.*` and `sandbox.network.allowed_domains` —
  unlocks the upstream "permission rules and sandbox merge" behavior
  (`Edit(/foo/**)` adds `/foo/**` to `allow_write`).
- Custom proxy port (`http_proxy_port` / `socks_proxy_port`) wiring
  to corporate MITM proxies.
- Container backend (k8s pod, Docker) via
  `infra/sandbox/container.go`.
- Per-request sandbox override on Portal conversations.
- `enabled_platforms` knob (mirror upstream once we need
  per-platform enablement).

## 14. Out of scope (explicit)

- A from-scratch Go reimplementation of
  `@anthropic-ai/sandbox-runtime`. We shell out to `bwrap` /
  `sandbox-exec`.
- Native Windows OS-level bash sandboxing (WSL2 only, same as
  upstream).
- Putting non-bash tools (Read/Write/Edit/Glob/Grep/WebFetch)
  inside the sandbox. Their boundary is the existing permission
  flow.
- TLS-inspecting proxy. The built-in proxy filters by hostname
  only; `enable_weaker_network_isolation` + custom proxy port are
  the upstream-documented path for TLS inspection.
- Dynamic profile switching mid-run; sandbox config is fixed at
  run start (a `Refresh()` call still requires the next run to
  pick it up).

## 15. Acceptance

§3.2 lands when:

- CLI, Desktop, Worker, and Portal task runs resolve and display a
  sandbox config (`enabled`, mode, backend, sources).
- Worker default is `enabled: true,
  fail_if_unavailable: true, allow_unsandboxed_commands: false`;
  workers refuse to start on platforms missing the backend.
  Downgrades are logged, traced, visible to hooks.
- Bash commands on Linux/macOS run inside `bwrap` /
  Seatbelt; child processes inherit the same boundaries.
- Network egress from inside the bash sandbox is filtered by the
  Go-side proxy using `allowed_domains` / `denied_domains`.
- `excluded_commands` opts specific commands out of the sandbox;
  `dangerously_disable_sandbox` honors `allow_unsandboxed_commands`.
- Secret-shaped env vars never leak into the sandbox unless
  explicitly listed.
- `command` and `http` hook transports honor the same boundaries.
- `buildmax sandbox` subcommand mirrors Claude Code's `/sandbox`
  panel (status, deps, mode, overrides, enable/disable).

---

*Phases A–E of this doc are implemented; see §13.1 for what is still open
before the §15 acceptance list is satisfied.*
