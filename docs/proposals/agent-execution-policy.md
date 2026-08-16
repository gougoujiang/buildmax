# Agent Execution Policy

> **Audience:** contributors, operators, and security reviewers · **Status:** proposal — under discussion

Related: [../ROADMAP.md](../ROADMAP.md) P0.5, [trust harness design](../design/trust-harness.md), [sandbox boundaries](../design/sandbox-boundaries.md), [sandbox guide](../guide/sandbox.md), and [production deployment reference](../../deployment/production/README.md).

## What This Proposal Is Now About

The original paper asked a broad question about worker execution. Part of it has
since been answered by shipped work, so this narrows to what is still undecided:
**who chooses a worker's boundary, and what happens when the chosen one cannot
be applied.**

Settled, and out of scope here:

- **What a run holds.** A task run receives only the variables marked
  `WorkerNeeds` in `internal/config/env_spec.go`; an unrecognized `BUILDMAX_`
  variable is withheld. Object-storage keys are still passed, and narrowing that
  needs a server-issued run-scoped credential — separate work, not a policy
  question.
- **What a run runs inside.** Worker Job pods are non-root, drop all
  capabilities, mount no service-account token, use `RuntimeDefault` seccomp, and
  have a read-only root filesystem. None of it is configurable, because a worker
  executes model-chosen shell commands.
- **Resource bounds.** `worker.k8s.resources` sets optional CPU and memory
  limits.
- **Visibility.** A run records the boundary it actually ran under, including
  when it was not sandboxed. The Beta gate requires this and it is in place.

## Problem

What is left is that the boundary is *fixed by the deployment's manifests*, not
chosen. A cluster operator can harden every worker equally or not at all. There
is no way to say that one team's automations may reach the internet and
another's may not, and no defined behaviour when a requested constraint is
unavailable — the OS sandbox is off on every surface, so today the question does
not arise, and the moment it is switched on it will.

The gap this leaves is concrete: network egress. A worker pod can reach anything
the cluster's network allows. `deployment/production/README.md` says restricting
egress is worth doing and is not settled here, and points at this paper.

## Goals

- Decide whether execution profiles are a BuildMax concept at all, or whether
  the boundary belongs entirely to the cluster.
- If they are: define a small set of named profiles, their precedence between
  operator, team, and task, and what a run records about which one applied.
- Define fail-closed semantics — what a worker does when a selected profile
  needs a backend, a NetworkPolicy, or a sandbox that is not available.
- Identify which operations, if any, are worth an approval gate rather than a
  static policy.

## Non-Goals

- Claiming that every MCP tool, provider call, or host process is sandboxed.
- A general policy language or arbitrary policy code execution.
- Replacing operating-system, Kubernetes, cloud, or network controls; a profile
  should *drive* a NetworkPolicy, not reimplement one.
- Re-opening credential scope or pod containment, which are settled above.
- Perfect isolation on platforms where the required primitives do not exist.

## Options To Evaluate

| Option | Strength | Main concern |
|---|---|---|
| Leave the boundary entirely to cluster manifests | No new concept; operators already own NetworkPolicy and pod security | Gives a team operator no control, and BuildMax cannot record or explain a boundary it does not model |
| One deployment-wide profile in `server.yaml` | Simple to explain, test, and record in a trace | Too rigid for a deployment that runs both trusted internal automation and untrusted work |
| Layered operator, team, and task profiles | Matches the team ownership boundary | Requires deterministic precedence and diagnostics good enough to explain a refusal |
| Per-user runtime settings only | Familiar local configuration | Cannot give an operator an authoritative worker boundary |

The likely direction remains a small layered model: an operator baseline sets
non-bypassable limits, a team may select from narrower approved profiles, and
the worker resolves and records one effective profile before starting. That is
not a decision — it needs evidence that anyone wants per-team differences, since
the deployment-wide option is materially cheaper.

## Questions To Resolve

- Is a per-team boundary a real requirement, or is one hardened deployment-wide
  profile enough for Beta and after?
- Should a profile that cannot be applied fail the run or downgrade it with a
  recorded warning? The trace already reports an unsandboxed run as unsandboxed,
  which makes downgrade defensible in a way it was not before.
- Which egress destinations does a worker legitimately need — model endpoint,
  object storage, the server — and can a default-deny NetworkPolicy be shipped
  with the production reference without breaking a normal run?
- Does an approval gate belong here at all, given that unattended scheduled work
  is a primary use and nothing gates it today?
- How are profile changes versioned and attached to an existing TaskRun record?

## Evidence Needed For A Decision

- An operator statement that per-team boundaries are needed, or that they are
  not.
- A threat model for a malicious prompt, a compromised model response, and
  accidental access to an adjacent workspace, evaluated *against the containment
  that now exists* rather than against the state before it.
- A kind smoke path demonstrating a default-deny egress policy that still lets a
  normal run complete.
- Cross-platform tests proving profile resolution and fail-closed behaviour.

## Likely Destination If Accepted

The outcome belongs in the P0.5 trust-harness plan and the sandbox
specification, with operator-facing configuration in `docs/reference/` and the
egress half landing in `deployment/production/`, which currently documents its
absence.
