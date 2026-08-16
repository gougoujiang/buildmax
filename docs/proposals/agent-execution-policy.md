# Agent Execution Policy

> **Audience:** contributors, operators, and security reviewers · **Status:** proposal — under discussion

Related: [../ROADMAP.md](../ROADMAP.md) P0.5, [trust harness design](../design/trust-harness.md), [sandbox boundaries](../design/sandbox-boundaries.md), and [sandbox guide](../guide/sandbox.md).

## Problem

Workers execute model-selected tasks against team data. The current sandbox is
an opt-in Bash boundary and worker hardening remains incomplete. A private
enterprise deployment needs a policy that explains what a worker can read,
write, execute, connect to, and retain, as well as which actions need a human
decision.

The policy must describe the effective boundary, not merely expose more
configuration. A failed sandbox, hook, or policy lookup must have explicit
semantics; silently running an unconstrained task is not an acceptable default
for a deployment that selects a restricted profile.

## Goals

- Define a small set of named worker execution profiles with explicit default
  behavior.
- Control filesystem access, network egress, environment exposure, process
  lifetime, and CPU/memory limits for a TaskRun.
- Make policy resolution visible in the task trace and operator diagnostics.
- Identify high-risk operations that may require approval before execution.
- Keep direct local CLI use possible without imposing a Server policy.

## Non-Goals

- Claiming that every MCP tool, provider call, or host process is sandboxed.
- A general policy language or arbitrary policy code execution.
- Replacing operating-system, Kubernetes, cloud, or network controls.
- Perfect isolation on platforms where the required primitives do not exist.

## Options To Evaluate

| Option | Strength | Main concern |
|---|---|---|
| Per-user runtime settings only | Familiar local configuration | Cannot give a team operator an authoritative worker boundary |
| One deployment-wide locked-down profile | Simple to explain and test | Too rigid for trusted internal automations and different workloads |
| Layered operator, team, and task profiles | Matches BuildMax ownership boundaries | Requires deterministic precedence and good diagnostics |
| Delegate all control to Kubernetes manifests | Strong infrastructure controls | Does not cover local-process workers or task-level approvals |

The likely direction is a small, layered profile model: an operator baseline
sets non-bypassable limits; a team may select from narrower approved profiles;
the Worker resolves and records one effective profile before starting. The
existing sandbox and hook contracts remain implementation inputs, not a reason
to duplicate them here.

## Questions To Resolve

- Which profile should be the default for Kubernetes workers, and should a
  missing sandbox dependency fail closed?
- Which tool categories, paths, domains, and environment variables require an
  explicit policy decision?
- What approval model is useful without blocking unattended scheduled work?
- How are policy changes versioned and attached to an existing TaskRun trace?
- Which controls belong in BuildMax versus Kubernetes security context,
  NetworkPolicy, and cloud IAM?

## Evidence Needed For A Decision

- A threat model for a malicious prompt, compromised model response, and
  accidental access to a credential or adjacent workspace.
- Cross-platform tests proving profile resolution and fail-closed behavior.
- A Kubernetes smoke path that demonstrates resource and egress restrictions.
- An operator review of the trace fields needed during an incident.

## Likely Destination If Accepted

The outcome belongs in the P0.5 trust-harness plan and sandbox specification,
with operator-facing configuration in `docs/reference/` and worker integration
issues scoped by execution profile.
