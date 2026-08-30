# Execution Modes, Authority, And The Two-Tier Model

> **Audience:** contributors, architects, and anyone assessing BuildMax for private deployment · **Status:** proposal — under discussion
>
> **Opened:** 2026-08-30

Related: [Portal execution model](../design/portal-execution-model.md); [product
vision](../design/product-vision.md); [surface
positioning](../design/surface-positioning.md); [trust
harness](../design/trust-harness.md); [team
governance](../design/team-governance.md); [local background
jobs](../design/local-background-jobs.md); [issue agent
access](../design/issue-agent-access.md); [durable run
trace](../design/durable-run-trace.md); and
[tier-2-visibility.md](tier-2-visibility.md), which is one consequence of this
paper's boundary applied to a single feature.

## Contents

- [Problem](#problem)
- [Decision Sought](#decision-sought)
- [What Is Forced And What Is Chosen](#what-is-forced-and-what-is-chosen)
- [Evidence: The Two Tiers Are One Loop](#evidence-the-two-tiers-are-one-loop)
- [Evidence: Every Fix Came From The Substrate](#evidence-every-fix-came-from-the-substrate)
- [Tier Names Something It Was Not Coined To Name](#tier-names-something-it-was-not-coined-to-name)
- [What The Enterprise Differentiator Actually Is](#what-the-enterprise-differentiator-actually-is)
- [The Authority Gaps](#the-authority-gaps)
- [Proposed Direction](#proposed-direction)
- [What This Does Not Change](#what-this-does-not-change)
- [Open Questions](#open-questions)
- [Likely Destination If Accepted](#likely-destination-if-accepted)

## Problem

BuildMax describes its Portal execution as two tiers of agent: Tier 1, a
foreground orchestrator, and Tier 2, a background execution agent plane. The
vocabulary appears in the product vision, the execution-model record, the
architecture documents, code comments, and one string sent to the model itself.

Read as an agent architecture, it does not survive contact with the code. Read as
an execution-mode distinction it is exactly right, and the design record has
already half-said so. The difference matters because the tier framing is
currently absorbing attention that the system's actual differentiator is not
getting.

## Decision Sought

1. **Is "Tier" an agent-architecture boundary or an execution mode?** This paper
   argues execution mode, and proposes demoting the vocabulary accordingly.
2. **What is BuildMax's defensible differentiator for private deployment, and
   what invariant does it require?** This paper argues verifiable authority, and
   that it requires one enforced context-admission rule the repository states
   three times in three vocabularies and enforces centrally nowhere.

## What Is Forced And What Is Chosen

Three things are forced by physics and are not design choices:

1. **Latency diverges.** A synchronous turn has a bounded budget; some work
   exceeds it.
2. **Work that outlives the request must leave the connection.** Its state has to
   live in a record.
3. **Executing untrusted code needs isolation.** Not in a process serving every
   tenant.

Together these force an **attached / detached** execution split and a durable
substrate. Any system doing this work has them.

What is chosen — and only chosen — is describing the two modes as two *agents*.

## Evidence: The Two Tiers Are One Loop

- **Both call the same function.** `agent.RunLoop` has three non-test call sites:
  [`internal/service/conversation/runtime.go:223`](../../internal/service/conversation/runtime.go)
  (Tier 1),
  [`internal/agentapp/app.go:1076`](../../internal/agentapp/app.go) (Tier 2 and
  every local surface), and
  [`internal/tool/subagent_runner.go:199`](../../internal/tool/subagent_runner.go).
- **There is no tier in the loop.** `RunLoopOpts` carries no tier field. Its only
  agent-kind flags are `IsSubagent` and `AgentType`, which mark subagents.
- **The difference is an option struct.** Tier 1 passes eight fields. It leaves
  unset `Compactor`, `Checkpointer`, `Memory`, `Hooks`, `EventSink`, `Grants`,
  `Approval`, `Pricing`, `SessionID`, `Workspace`, and `Invariants`. Both tiers
  pass `agent.AllowAllPolicy()`.
- **Tier 1 is a real agent, not a router.** Ten iterations, four parallel tools,
  and four orchestration tools — `StartTask`, `ListTasks`, `GetTask`,
  `ContinueTask`. Dispatch to Tier 2 is a tool call the model makes, not a
  decision server code takes.
- **The word is load-bearing in code exactly once**, as `Tier1TargetID` in
  [`internal/bootstrap/llmgateway.go`](../../internal/bootstrap/llmgateway.go) —
  a model-routing label, not an execution concept. Everywhere else it is a
  comment.

Delete the word and what remains is one agent capability, assembled two ways.

## Evidence: Every Fix Came From The Substrate

The execution-model record was accepted in a documentation-only commit; the
defects it describes had already been fixed. Each fix is worth naming, because
none of them used the tier distinction:

| Defect | What fixed it |
|---|---|
| A finished run told the creator's first socket — nothing if they had none, one tab if they had three | Team-wide invalidation broadcast plus the server turn queue |
| `[Task Result]` stored as `role=user`, so the Portal drew machine output as the user's own words | Excluding the system channel from the transcript |
| A task existed only if Tier 1's summary succeeded | Cards read from the tasks route and reloaded on invalidation |
| A one-shot report lost on a failed model call or a restart | The `task_result_delivery` row: a claimed, swept, bounded obligation |
| Work not traceable to the request that caused it | `task_run.source_message_id`, bound per turn so the model cannot omit it |
| Two runs of one task executing different agent text, unrecorded | `task_run.agent_revision` |

Every one is "derive from a record" or "make the obligation durable." The tier
split fixed none of them, and the record's own §1 point 4 says so: the load-
bearing idea is the derived projection.

## Tier Names Something It Was Not Coined To Name

The vocabulary predates the record. Its origin document split the tiers by
**role** — Tier 1 understands, clarifies, and routes; Tier 2 executes. The
execution-model record replaced that divider with six execution properties and
added the sentence that retires the original meaning: "Dispatching to Tier 2 does
not mean Tier 1 could not do the work."

So the term already survived a change in what it denotes. The record then went
further, in [§5.5](../design/portal-execution-model.md): defining the tiers as
lifecycle-separated and calling Task, TaskRun, scheduler, and worker
infrastructure "is naming work with no behavior attached."

This paper agrees and takes the next step. A word that names no behavior, whose
meaning has already been replaced once, and which appears in the prompt the model
reads, is a fossil worth retiring rather than a boundary worth defending.

## What The Enterprise Differentiator Actually Is

The records claim **deployability**: "an out-of-the-box, privately deployable
enterprise Agent platform powered by one shared Go Agent Core," one runtime
across three operating profiles, with Portal adding "organizing, reusing,
governing, and observing that capability across teams."

Deployability is a real strength and a weak moat. Packaging is copyable; a single
Go binary is an engineering achievement, not a defensible position.

What is defensible, and what BuildMax is unusually close to owning, is
**verifiable authority**: the ability to answer, mechanically and after the fact,
*who asked for this, under which instructions, executed by which definition,
inside which boundary, and on what evidence.* The pieces already exist —
`source_message_id` binding intent per turn, `agent_revision` recording the text
that ran, an append-only audit trail with typed actors, bounded redacted traces,
and a provenance route that deliberately quotes the user's message beside the
worker's instruction so "a constraint missing from the instruction is either one
the model dropped or one the user never gave."

No competitor gets this for free, and the product vision's own Decision Test
already asks it of every capability: that "authority, durability, failure
behavior, and evidence can be explained plainly."

The tier vocabulary neither creates nor protects that property. The substrate
does.

## The Authority Gaps

If verifiable authority is the differentiator, these are the gaps that matter,
and they are more consequential than anything in the tier question:

1. **Tier 1 produces no trace.** `EventSink` is unset at
   [`conversation/runtime.go:223`](../../internal/service/conversation/runtime.go),
   and the package imports neither `agentapp` nor the trace recorder. Its model
   calls are also unledgered: `bootstrap/server.go` assigns
   `ConversationLLMClient` the raw routed client, bypassing the
   `llmgateway.Service` that holds the ledger and quota. So the agent that holds
   user authority, reads the request, and decides what background work to
   dispatch is the one agent in the system with neither trace nor metered call
   record — while everything it dispatches is fully evidenced. [AGENTS.md](../../AGENTS.md)
   states that every run records a trace by default; for Tier 1 that is not true.
2. **Audit cannot reach a run.** Team governance records that no audit action
   names a run, so an investigation starting from an audit event cannot
   mechanically arrive at the execution that caused it.
3. **Context admission is stated three times and enforced nowhere.** The rule
   that untrusted content must not become instruction appears as a per-tier trust
   table in the execution-model record, as "the same trust class as `WebFetch`
   output … they arrive as tool results" in issue agent access, and as "the event
   envelope tells the model to analyze, not obey" in local background jobs. Three
   records, three vocabularies, one rule, no central enforcement.
4. **The right primitive already exists and is barely used.**
   `llm.Message.Source` in [`internal/core/llm/llm.go`](../../internal/core/llm/llm.go)
   records non-user provenance for a user-role message — "Empty means genuinely
   user-authored" — precisely so that "even a compaction summary cannot turn an
   observation into something the user said." It has three values, all local
   background jobs. The server path marks the same distinction with a *channel*
   instead.
5. **Neither tier has a permission boundary.** Both pass `AllowAllPolicy`, and
   the sandbox is off on every surface.

## Proposed Direction

1. **Demote "Tier" from architecture to execution mode.** Name what is real:
   *attached* and *detached* execution of one agent capability, over one durable
   substrate. Specialization is already carried by agent definitions and their
   revisions, not by a tier. Remove the word from the model's prompt first, since
   an assistant that repeats it teaches users a concept the product should not
   have.
2. **Promote `Source` into a general context-admission invariant.** One rule,
   enforced in the loop rather than asserted in three records: content that did
   not originate with an authenticated principal carries a non-empty `Source`,
   and never enters the instruction channel. This is strictly stronger than the
   present per-tier rule, because it also covers Tier 1's own tool output, which
   today's rule does not reach.
3. **Close Tier 1's evidence gap.** A foreground turn should trace and meter like
   any other run. This is the single highest-value item in this paper.
4. **Let audit name a run.**

## What This Does Not Change

Everything §4 of the execution-model record shipped stays exactly as it is:
derived projection, durable delivery obligation, `source_message_id`,
`agent_revision`, machinery out of the conversation list. The substrate is the
part that earned its keep.

This is emphatically **not** the "merge into one synchronous agent" alternative
the record rejected in §7. That merge would lose durable retry, cancel, and
isolation, and the rejection was correct. Keeping two execution modes over one
substrate loses none of them — the record's §7 considered merging the *modes*,
and never considered keeping the modes while dropping the claim that they are two
kinds of agent.

## Open Questions

- **Does any specialized Tier 2 agent actually exist yet?** §5.2 gated the
  catalog on evidence and warned against predefining agent types. If specialized
  agents never materialize, the "agent plane" framing loses its last
  justification; if they do, they are agent *definitions*, which is a different
  axis from execution mode.
- **What should Tier 1's capability set be?** The record leaves this open, and
  §5.3 notes a foreground budget becomes a prerequisite the moment Tier 1 reads
  team files, issues, or results. Under one context-admission invariant this
  question gets easier, not harder.
- **Is a machine-checkable context-admission rule enforceable in a loop at all**,
  or does it reduce to prompt framing plus provenance metadata? The existing
  envelope is advisory. Deciding this needs the threat model the trust harness
  already names as its cheapest missing input.
- **Is deployability or verifiable authority the position to lead with?** That is
  a product decision this paper informs but cannot make.

## Likely Destination If Accepted

The tier demotion belongs in [portal execution
model](../design/portal-execution-model.md) §5.5, which already reserved the
naming question, and in [product vision](../design/product-vision.md), which
states the tier boundary as product language. The context-admission invariant
deserves its own design record, since it spans `internal/core/llm`,
`internal/core/agent`, `internal/agentapp`, and the conversation service, and it
would then replace the three partial statements rather than join them. The Tier 1
evidence gap is a defect and should become an Issue rather than wait for any of
this.
