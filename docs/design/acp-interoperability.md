# ACP Interoperability Boundary

> **简体中文：** [阅读中文镜像](../zh-CN/design/ACP互操作边界.md)

> **Audience:** contributors · **Status:** current direction — no implementation or roadmap commitment

Related: [product vision](product-vision.md),
[surface positioning](surface-positioning.md),
[Agent execution and Task threads](agent-execution-and-task-threads.md),
[tool permissions](tool-permissions.md), and
[sandbox boundaries](sandbox-boundaries.md).

## Contents

- [1. Decision](#1-decision)
- [2. User Outcome And Evidence](#2-user-outcome-and-evidence)
- [3. Why ACP Fits The Edge](#3-why-acp-fits-the-edge)
- [4. Product And Architecture Boundary](#4-product-and-architecture-boundary)
- [5. Session And Event Semantics](#5-session-and-event-semantics)
- [6. Authority And Trust](#6-authority-and-trust)
- [7. Delivery Order](#7-delivery-order)
- [8. Alternatives Considered](#8-alternatives-considered)
- [9. Consequences](#9-consequences)
- [10. Decision Triggers](#10-decision-triggers)

## 1. Decision

BuildMax may support the
[Agent Client Protocol](https://agentclientprotocol.com/) at a product edge,
but ACP does not become an internal runtime, persistence, or worker protocol.
The shared Go Agent Core remains the one native Agent implementation used by
CLI, TUI, Desktop, Portal conversations, and TaskRun workers.

If ACP work is prioritized, the first direction is for BuildMax to implement
the **Agent side** of ACP. An ACP client such as an editor or another Agent
control surface can then start BuildMax, open a session, send prompts, observe
updates, answer permission requests, and cancel work. A local stdio command
such as `buildmax acp` is the expected shape, not a committed command name or a
network service.

BuildMax acting as an **ACP client** for Codex, Claude Code, Gemini, OpenCode,
or another external Agent is a separate, deferred product decision. This
record neither accepts an interchangeable execution layer nor introduces an
executor abstraction in `internal/core/agent`.

ACP support is not part of the current private-deployment Beta gate. The active
[roadmap](../ROADMAP.md) continues to take priority.

## 2. User Outcome And Evidence

The intended outcome is narrow: a person who already works in an ACP-capable
client can use the BuildMax Agent Core without moving that work into a second
chat interface, while BuildMax retains its own workspace, permission, trace,
and session behavior.

There is ecosystem evidence that this boundary is useful. ACP defines a
capability-negotiated protocol for initialization, authentication, sessions,
prompts, streaming updates, permission requests, and cancellation. Existing
Agent products use it to connect clients and coding Agents without a bespoke
wire adapter for every pair. That demonstrates interoperability, not demand
for BuildMax to implement it now.

There is no current evidence that BuildMax users need another Agent harness to
execute Space work. No external executor, Portal control plane, or worker
compatibility layer is justified by the editor-integration outcome above.

The constraints are:

- local CLI/TUI remains a single Go binary without Node;
- important Agent behavior continues to land in the shared runtime first;
- ACP does not weaken workspace-root, tool-permission, hook, trace, or sandbox
  behavior;
- Local Session, Conversation, Task, and TaskRun keep their existing ownership
  and persistence contracts; and
- protocol support is capability-negotiated and tested against pinned versions,
  rather than assuming every ACP implementation behaves alike.

## 3. Why ACP Fits The Edge

ACP is a bidirectional JSON-RPC protocol between a client and an Agent. Its
standard surface covers the interaction boundary BuildMax would otherwise
have to invent for each editor:

- version and capability negotiation;
- session creation, resume, listing, and close;
- prompt submission and cancellation;
- streamed messages, plans, tool calls, terminal presentation, and state;
- permission and structured elicitation requests; and
- optional filesystem, terminal, MCP, mode, and configuration capabilities.

Those concepts are projections and controls around an Agent run. They are not
the authoritative implementations of BuildMax's domain behavior. ACP does not
define Space authorization, TaskRun state transitions, scheduler leases,
workspace checkpoints, Artifact publication, quota, audit, worker credentials,
or deployment recovery. Those remain BuildMax contracts.

The boundary is therefore:

```text
ACP client
    |
    | ACP JSON-RPC
    v
ACP edge adapter
    |
    v
internal/agentapp --> internal/core/agent
```

ACP wire types stop at the adapter. They do not enter `internal/core`, and the
Agent Core does not branch on whether a turn came from ACP, TUI, Desktop, or
another surface. The implementation package and dependency choice are decided
only when work is scheduled; this direction does not create a speculative
public interface.

## 4. Product And Architecture Boundary

The first useful surface is local and process-scoped. It exposes the same
workspace Agent a user can already run from CLI/TUI, with an ACP client
providing the interaction UI. It does not require a BuildMax Server, create a
Space resource, or turn an editor session into a Task.

Native BuildMax surfaces remain first-class. ACP is an additional adapter, not
a replacement UI architecture and not the transport between existing
components. In particular:

- CLI/TUI and Desktop continue to assemble the native runtime directly;
- Portal continues to use BuildMax Server APIs;
- workers continue to use the run-scoped worker API;
- Task and TaskRun remain the durable Space execution plane; and
- managed inference continues to use its versioned BuildMax wire contract.

No remote ACP listener is implied. A network-reachable service would introduce
authentication, tenancy, routing, session ownership, and resource-lifetime
questions that a local stdio adapter does not need.

## 5. Session And Event Semantics

An ACP session opened against local BuildMax is a Local Session projection.
It does not create a new server entity or a second session store. If the ACP
contract and the local bundle can share an identifier safely, the existing
Local Session identity should do both jobs; otherwise an adapter-local mapping
is preferred over a new domain concept.

The semantic mapping is:

| ACP operation | BuildMax authority |
|---|---|
| initialize | adapter reports only capabilities the assembled runtime can honor |
| session/new | create or bind one Local Session and its workspace root |
| session/resume | restore through Local Session storage, when supported |
| session/prompt | run one normal shared Agent turn |
| session/update | project Agent, tool, plan, usage, and state events |
| session/request_permission | ask for consent within BuildMax's effective policy |
| session/cancel | cancel the active RunLoop context |
| session/close | release the ACP interaction; do not imply deletion of durable history |

ACP updates are presentation events. The local session bundle and bounded
BuildMax JSONL trace remain the durable records. Unsupported replay or resume
behavior is omitted through capability negotiation, not simulated from an
incomplete history.

ACP protocol revisions are external compatibility boundaries. An implementation
must negotiate only the versions it understands and test pinned client/version
combinations. It must not follow an unpinned `latest` schema or silently accept
unknown behavior as equivalent.

## 6. Authority And Trust

ACP conveys requests; it grants no authority by itself.

- The ACP `cwd` is canonicalized and checked through the same workspace-root
  rules as a native local run. Additional directories are not advertised until
  BuildMax has an explicit multi-root authority model.
- Client approval can satisfy a required user-consent step, but it cannot
  override a BuildMax deny rule, sandbox policy, or unavailable approval path.
- Client-supplied MCP servers, terminal access, and filesystem callbacks are
  capabilities, not trusted configuration. The first slice may omit them;
  accepting them later requires the same policy and process-boundary treatment
  as their native equivalents.
- BuildMax does not send local or Space secrets merely because an ACP field can
  carry environment or authentication data.
- The durable trace distinguishes BuildMax-observed execution from content an
  ACP peer only reported. A projection gap is recorded rather than presented
  as complete evidence.

These rules become stricter if BuildMax later acts as an ACP client. ACP Agents
normally run as child processes and may own their tools, context, and model
authentication. ACP is not a sandbox. An external Agent and every child it
starts would have to run inside the execution boundary BuildMax claims; merely
parsing its tool updates is not enforcement.

## 7. Delivery Order

This direction deliberately separates architectural fit from roadmap priority.
If user evidence earns implementation work, deliver the smallest proof in this
order:

1. Implement the Agent side of the required ACP version over local stdio.
2. Cover initialization, one new session, one prompt, streamed updates,
   permission handling, cancellation, and clean shutdown.
3. Verify it against one named ACP client and a deterministic fake client in
   repository tests.
4. Add resume only when Local Session restoration can satisfy the advertised
   contract exactly.
5. Document and package the surface only after the shipped binary passes the
   same release-archive checks as the CLI.

Do not combine the first slice with a remote listener, Portal UI, worker
execution, arbitrary external Agent commands, multiple protocol generations,
or a public Go SDK.

External ACP Agents are reconsidered separately. A first adapter would require
evidence from multiple users asking for the same Agent, whole-process
confinement, explicit capability differences, and Task/TaskRun continuity that
does not depend on an ephemeral worker retaining hidden session state.

## 8. Alternatives Considered

### Do Not Support ACP

This keeps the product smaller and remains acceptable while users prefer native
surfaces. It gives up a standard integration point and requires bespoke work if
an editor integration later matters. The chosen direction keeps the boundary
available without assigning it roadmap priority.

### Make ACP The Internal Runtime Protocol

Rejected. ACP does not own BuildMax domain state or deployment behavior, and
forcing native surfaces through it would add translation and failure modes
without providing the capabilities they require.

### Act As An ACP Client First

Rejected as the default order. It expands the trust and support boundary to
another Agent's tools, credentials, context, persistence, and child processes.
Exposing the native BuildMax Agent gains interoperability without surrendering
those controls.

### Build Per-Editor Integrations

Rejected unless a client cannot meet a demonstrated need through ACP. Separate
plugins would multiply transport, streaming, permission, and lifecycle code
while solving the same user outcome.

## 9. Consequences

The positive consequences are:

- BuildMax can enter ACP-capable clients without creating another Agent Core;
- one standards-based adapter can replace several editor-specific integrations;
- native permissions, traces, hooks, tools, and model portability remain in
  force; and
- external Agent execution stays out of scope until it proves a separate user
  outcome.

The costs are:

- ACP version compatibility and conformance become maintained boundaries;
- a client may present capabilities differently from BuildMax's native
  surfaces;
- permission and cancellation races need explicit tests; and
- local session replay may not match every optional ACP capability.

The direction does not promise parity with another ACP Agent or client. BuildMax
advertises only what it implements and reports unsupported behavior plainly.

## 10. Decision Triggers

ACP implementation enters the roadmap only when a concrete user journey is
blocked without it and a named client provides a repeatable acceptance target.
The minimum evidence is:

- users want the BuildMax Agent inside that client rather than another native
  BuildMax surface;
- ACP covers the required interaction without a client-specific escape hatch;
- the permission and workspace boundary can be stated and tested; and
- maintaining the tested protocol versions costs less than maintaining the
  equivalent bespoke integration.

Supporting external ACP Agents requires a second decision with stronger
evidence: which Agent, which execution surface, how its process tree is
confined, how its opaque session survives worker loss, which events are
authoritative, and which BuildMax capabilities are unavailable. Until that
decision is recorded, "ACP support" means BuildMax on the Agent side only.
