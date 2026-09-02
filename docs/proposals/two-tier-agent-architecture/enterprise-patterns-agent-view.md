# Enterprise Patterns Agent View: Durable Execution Before Multi-Agent Hierarchy

> **Audience:** participants in the two-tier Agent architecture roundtable · **Status:** proposal — under discussion; independent enterprise-patterns view

Opened: 2026-08-30

Related current documents:

- [Roundtable index](README.md)
- [Portal execution model](../../design/portal-execution-model.md)
- [Product vision](../../design/product-vision.md)
- [Evaluation system](../../design/evaluation-system.md)
- [Data model](../../contribute/architecture/data-model.md)

External patterns considered:

- [Anthropic orchestrator-workers workflow](https://www.anthropic.com/engineering/building-effective-agents)
- [Amazon Bedrock multi-Agent collaboration](https://docs.aws.amazon.com/bedrock/latest/userguide/agents-multi-agent-collaboration.html)
- [Amazon Bedrock AgentCore asynchronous runtime](https://docs.aws.amazon.com/bedrock-agentcore/latest/devguide/runtime-long-run.html)
- [Microsoft multi-Agent patterns](https://learn.microsoft.com/en-us/agents/architecture/multi-agent-patterns)

## Contents

- [1. Position](#1-position)
- [2. Goals And Non-Goals](#2-goals-and-non-goals)
- [3. Three Enterprise Patterns](#3-three-enterprise-patterns)
- [4. Assessment Of Current BuildMax](#4-assessment-of-current-buildmax)
- [5. What Enterprise Orchestration Must Own](#5-what-enterprise-orchestration-must-own)
- [6. Recommended Product Mental Model](#6-recommended-product-mental-model)
- [7. Recommended Runtime Forms](#7-recommended-runtime-forms)
- [8. Architecture Principles](#8-architecture-principles)
- [9. Options Considered](#9-options-considered)
- [10. Evidence-Gated Hypotheses](#10-evidence-gated-hypotheses)
- [11. Open Questions](#11-open-questions)
- [12. Recommendation](#12-recommendation)

## 1. Position

BuildMax already resembles an enterprise durable asynchronous Agent runtime
more than it resembles a complete multi-Agent orchestrator. That distinction
should guide its evolution.

The present foreground/background split is justified by execution properties:
latency, disconnection, isolation, cancellation, retry, and recovery. Those
properties require two execution lifecycles. They do not require two permanent
Agent identities or a supervisor/subordinate product hierarchy.

BuildMax should therefore preserve its durable worker substrate while treating
the foreground model as an optional semantic coordinator, not as the
authoritative orchestration kernel. The target should be:

1. one coherent user-facing BuildMax Assistant;
2. deterministic commands for unambiguous control operations;
3. an optional planner or capability router when language understanding adds
   value;
4. a durable control plane that validates and advances execution; and
5. isolated Agent runs whose outputs become structured outcomes before they
   become conversation prose.

This position does not reject multi-Agent systems. It argues that specialization,
dynamic decomposition, and Agent-to-Agent protocols should be added only where
they solve observed quality, context, security, ownership, or parallelism
problems.

## 2. Goals And Non-Goals

### 2.1 Goals

This view aims to:

- classify the current BuildMax design against common enterprise Agent
  patterns;
- separate model-driven semantic decisions from deterministic execution
  authority;
- identify a user mental model that does not expose infrastructure topology;
- describe runtime forms that can coexist instead of forcing every request
  through one pattern;
- state architecture invariants that are safe to adopt now; and
- identify assumptions that require evaluation before they become schema or
  product commitments.

### 2.2 Non-goals

This view does not:

- propose deleting Task, TaskRun, the scheduler, workers, or outcome cards;
- claim that one general-purpose Agent will always outperform specialists;
- prescribe an immediate general-purpose execution DAG;
- define fixed `ResearchAgent`, `CodingAgent`, or similar product types;
- make A2A or another federation protocol a current requirement;
- claim that worker sandbox hardening or Team approvals have shipped; or
- change the current accepted Portal execution model by itself.

## 3. Three Enterprise Patterns

The phrase `orchestrator/worker` is used for several different architectures.
They should be evaluated independently.

| Pattern | Primary concern | Orchestrator responsibility | Worker responsibility |
|---|---|---|---|
| Supervisor and specialists | Domain or capability routing | Keep the user relationship, select a bounded specialist, optionally combine answers | Apply distinct knowledge, tools, permissions, or policy |
| Dynamic orchestrator-workers | Unpredictable decomposition | Create subtasks at runtime, delegate, inspect, join, and replan | Solve one dynamically assigned subproblem |
| Durable asynchronous Agent runtime | Execution lifecycle | Admit, persist, schedule, isolate, cancel, retry, recover, and observe | Execute one accepted run independently of the client connection |

These patterns can be combined, but none implies the others. A durable runtime
can host one logical Agent. A supervisor can synchronously route without a
background worker. A dynamic planner can delegate into a durable runtime, but
the runtime still needs a deterministic owner for its state.

### 3.1 Supervisor and specialists

Enterprise supervisor patterns normally start with relatively stable specialist
roles. A supervisor identifies the domain, invokes the appropriate specialist,
and either routes its answer or synthesizes a final response. The reason to
create a specialist is not its name. It is an operational difference such as:

- a smaller and more relevant tool set;
- a distinct knowledge domain;
- a different permission or trust boundary;
- a separately owned service or policy;
- an independently evaluated prompt and model profile; or
- a context that should not be mixed with the supervisor's conversation.

The pattern is useful when those boundaries are stable. It adds little when the
specialists are only differently worded prompts over the same tools and data.

### 3.2 Dynamic orchestrator-workers

In a dynamic orchestrator-workers workflow, the planner cannot know all
subtasks before seeing the request. It creates a decomposition, delegates one
or more branches, gathers the results, and decides whether to synthesize,
verify, retry, or replan.

The defining property is not that several LLM calls occur. It is that one
compound goal has explicit coordination semantics. A durable implementation
must be able to answer:

- which goal the branches serve;
- why each branch exists;
- which branches are required or optional;
- which dependencies and join conditions apply;
- what constitutes completion;
- what happens when a branch fails; and
- which step produces the final outcome.

If those facts exist only in model context, the system has temporary
model-managed cooperation, not durable orchestration.

### 3.3 Durable asynchronous Agent runtime

A durable Agent runtime separates a request from the lifetime of the browser or
HTTP call. Its core concerns are persistent identity, execution isolation,
state transitions, cancellation, retry, resource policy, provenance, outputs,
and recovery.

This pattern does not require a supervisor Agent. One logical Assistant can
start a detached run and later read its result. It also does not define how a
complex goal is decomposed. Those are separate semantic and product decisions.

## 4. Assessment Of Current BuildMax

### 4.1 Strong match: durable asynchronous runtime

The current [Portal execution model](../../design/portal-execution-model.md)
and [data model](../../contribute/architecture/data-model.md) already provide
many of the correct runtime primitives:

- Task is a durable unit of work and TaskRun is an execution attempt;
- a scheduler and worker carry execution independently of the browser;
- cancellation and retry have explicit state rather than conversational
  meaning;
- output, Artifacts, trace, usage, and failure are durable run evidence;
- Agent revision, plugin pins, trigger source, and source message improve
  provenance;
- a task card is projected from stored execution state; and
- result delivery is a durable obligation whose failure does not erase the
  result.

This is the strongest and most defensible part of the current architecture. It
should remain the substrate for direct Task creation, Issue work, Workflow
steps, webhooks, and any future dynamic planner.

The remaining enterprise gaps include a validated ExecutionSpec, completed
worker hardening, Team approval semantics, richer wait-for-input states, and a
Task ownership model that does not require every execution to have a
Conversation parent.

### 4.2 Partial match: supervisor and specialists

Tier 1 has the basic topology of a supervisor. It is the single user-facing
voice and can select an Agent when starting a background Task. Issue,
Workflow, and direct Task paths can also pin an Agent.

The current specialist model is still thin. An Agent definition is primarily a
name, description, instructions, and plugin selection. Tier 1 selects from
natural-language summaries rather than typed capability contracts. The system
does not yet record stable accepted inputs, produced outputs, permission
profiles, execution budgets, service ownership, or historical quality signals
as catalog semantics.

BuildMax therefore resembles a front-door Assistant selecting an asynchronous
execution profile more than a mature supervisor coordinating independently
governed specialist services.

### 4.3 Weak match: dynamic orchestrator-workers

Tier 1 may start several Tasks, but independent task creation is not sufficient
to form a compound execution. Current durable state does not express a dynamic
plan, parent-child Task relationships, dependency joins, branch completion
policy, replanning, or a synthesis node.

The existing Workflow system is useful evidence in the other direction: it is
a deterministic, authored, linear sequence whose Agent definitions are pinned
for the run. That is closer to a sequential workflow than to dynamic
orchestrator-workers.

Until BuildMax persists the coordination semantics of a compound goal, calling
Tier 1 a dynamic multi-Agent orchestrator overstates what the system can recover
and explain.

### 4.4 The current foreground Agent is a semantic front door

The current Tier 1 tool surface centers on starting, continuing, listing, and
reading Tasks. It does not own general Team data access, an execution graph, or
policy administration. In practice it performs several jobs:

- immediate conversation response;
- foreground-versus-background classification;
- existing-Task resolution;
- instruction normalization;
- optional Agent selection;
- dispatch; and
- completion presentation.

Only some of those jobs inherently require a model. Explicit retry, cancel,
status lookup, user-selected Agent execution, Workflow steps, Issue execution,
and webhook dispatch should remain deterministic paths. A model call should be
used for ambiguity, decomposition, or synthesis only when it improves the
outcome enough to justify its latency, cost, and failure mode.

## 5. What Enterprise Orchestration Must Own

The most important boundary is between an orchestrator service and an
orchestrator Agent.

### 5.1 Semantic coordinator

A model coordinator may:

- interpret an ambiguous objective;
- ask for missing information;
- recommend foreground or detached execution;
- propose an Agent or capability;
- propose a decomposition and dependency structure;
- select relevant context references; and
- recommend synthesis, verification, or replanning.

Its output is a proposal. It is not execution truth.

### 5.2 Deterministic orchestration kernel

The service control plane must own:

- Team and actor authorization;
- resource, model, tool, plugin, and permission policy;
- quota and budget admission;
- validation and immutable capture of an ExecutionSpec;
- idempotent creation and state transitions;
- dispatch, leases, cancellation, retry, and recovery;
- dependency and join evaluation when compound execution exists;
- human approval and wait-for-input state;
- completion criteria and failure policy; and
- provenance, audit, and outcome publication.

The model may suggest what should happen. The kernel decides what is allowed,
records what was accepted, and is the only authority on what has happened.

### 5.3 Worker Agent

A Worker Agent should control how it achieves one bounded objective within its
accepted ExecutionSpec. It should not be able to:

- grant itself additional tools or permissions;
- rewrite the durable parent plan;
- declare a compound goal complete;
- speak directly as the user-facing Assistant;
- create unbounded descendants; or
- convert its own output into a new user instruction.

A worker that discovers additional work can return a typed delegation or
input request. The kernel decides whether that request becomes another run.

### 5.4 Presenter

Raw worker output is untrusted result data. Outcome projection should read
durable state directly. If a model-generated explanation is useful, a bounded
presenter may read the result and relevant Artifacts without receiving task
mutation tools.

The presenter can fail without losing the result. Its prose may be stored as
an assistant reply, but the raw worker result must not be replayed as a user
instruction.

## 6. Recommended Product Mental Model

End users should not need to understand Tier 1 and Tier 2. A better mental
model is:

> One BuildMax Assistant can answer now, ask an appropriate executor to work in
> the background, show the work's durable state, and explain the outcome.

The product may show the selected executor because ownership, expertise,
permission, or audit matters. It should describe that relationship as
`executed by`, not as a subordinate conversational persona.

| User concept | What the user needs to know | Internal detail normally hidden |
|---|---|---|
| Assistant | One coherent conversational relationship | Which model role classified or presented the turn |
| Work or Issue | The outcome being pursued | Scheduler topology and delivery rows |
| Executor | Relevant capability, owner, and policy | Worker process or Kubernetes Job identity |
| Run | Status, attempt, evidence, cost, and result | Internal queue and lease mechanics |
| Outcome | Summary, Artifacts, provenance, and next actions | Intermediate model messages |

Enterprise administrators need a deeper view: the exact Agent revision,
permissions, plugins, model profile, actor, approvals, budget, trace, and
Artifact lineage. That administrative need still does not require an end-user
hierarchy of Agent personas.

## 7. Recommended Runtime Forms

BuildMax should support several runtime forms over one execution substrate.
Complexity should be selected per request rather than imposed globally.

### 7.1 Foreground answer

Use when one bounded turn can answer safely within the interaction budget. No
TaskRun is created merely to preserve a two-tier topology.

### 7.2 Single durable run

Use when work must outlive the interaction or needs isolation, cancellation,
retry, a workspace, or durable evidence. This should remain the default
background form and can use a general-purpose Agent.

### 7.3 Routed specialist run

Use when a stable capability, permission, ownership, or context boundary makes
a specialist materially better. Routing may be deterministic when the user,
Issue, Workflow, or request type already identifies the executor. A semantic
router is justified only for genuine ambiguity.

### 7.4 Authored Workflow

Use when the sequence is known in advance and repeatability matters. A static
Workflow should not be replaced with dynamic planning merely because each step
uses an Agent.

### 7.5 Dynamic compound execution

Use only when subtasks cannot be predicted before the request is understood and
multiple branches measurably improve the outcome. If adopted, introduce a
durable Goal or Plan aggregate with nodes, dependencies, completion policy,
and TaskRun attempts. The planner proposes the graph; the kernel advances it.

### 7.6 On-demand presentation or verification

Use a presenter when the user asks for an explanation or when a product journey
has demonstrated a need for automatic synthesis. Use a verifier when the
outcome has an explicit rubric whose enforcement improves measured quality.
Neither role should be an automatic tax on every successful run.

## 8. Architecture Principles

The following principles can be adopted without first proving a particular
multi-Agent product bet.

1. **Execution lifecycle is not Agent identity.** Foreground and detached work
   may belong to one logical Assistant.
2. **An orchestrator service is not an orchestrator Agent.** Model planning is
   advisory; deterministic state is authoritative.
3. **Use the shortest reliable path.** Escalate from direct answer to one run to
   compound execution only when the task requires it.
4. **Task and TaskRun remain distinct.** A stable work objective can have
   multiple immutable execution attempts.
5. **Workers are infrastructure; Agent definitions are execution policy.** A
   process count does not determine the number of logical Agents.
6. **Team owns shared work.** Conversation, Issue, Workflow, and webhook are
   origins or delivery relations rather than universal execution parents.
7. **Execution facts are structured.** Status, dependencies, results,
   Artifacts, usage, and provenance are not inferred from conversation prose.
8. **Worker output is untrusted data.** Presentation must not grant it user or
   orchestration authority.
9. **Every run captures its boundary.** Actor, Agent revision, permissions,
   model, tools, plugins, workspace, context, budget, and output contract are
   validated and recorded.
10. **High-impact autonomy requires prior controls.** Least privilege,
    approvals, idempotency, cancellation, and audit are prerequisites rather
    than optional evaluation findings.
11. **Specialization needs an operational reason.** Quality, security,
    ownership, context reduction, or independent lifecycle should justify a
    specialist.
12. **Protocols follow trust boundaries.** Internal invocation can remain
    platform-native; Agent-to-Agent federation becomes useful when separately
    owned or opaque runtimes must interoperate.

## 9. Options Considered

| Option | Strength | Primary problem | Position |
|---|---|---|---|
| Preserve mandatory Tier 1 to Tier 2 Agent hierarchy | Simple narrative and close to current implementation | Conflates interaction, planning, authority, and presentation | Do not make this the durable abstraction |
| One synchronous Agent only | Minimum coordination overhead | Cannot satisfy detached execution, isolation, cancellation, and recovery | Reject |
| One logical Assistant with foreground and detached modes | Simple user model while preserving the runtime substrate | Still needs explicit control-plane boundaries | Recommended baseline |
| Fixed supervisor with named specialists | Useful for stable domain, permission, or ownership boundaries | Premature catalog design and routing errors if specialists are not real | Add selectively after evidence |
| Dynamic orchestrator-workers for complex goals | Flexible decomposition and parallelism | Adds graph state, join, replanning, cost, latency, and failure modes | Add only for demonstrated compound work |
| Peer-to-peer or federated Agent mesh | Supports opaque cross-team and cross-platform Agents | Highest governance, identity, protocol, and debugging complexity | Defer until an external trust boundary requires it |

## 10. Evidence-Gated Hypotheses

The following are product or modeling hypotheses, not architecture invariants.

| Hypothesis | Evidence required | Falsifying signal |
|---|---|---|
| Tier 1 model routing chooses foreground versus background better than explicit actions or deterministic rules | Labeled conversation evaluation with user correction rate | Similar or worse quality at higher latency and cost |
| Tier 1 instruction normalization improves worker outcomes | Source-message-to-run-input constraint retention and end-to-end task success | Dropped constraints outweigh clarification or decomposition gains |
| Stable specialists outperform one general Agent with tools | Per-capability quality, cost, latency, and permission comparisons | No durable quality or boundary advantage |
| Natural-language Agent summaries are sufficient for routing | Routing accuracy against user intent and administrator policy | Frequent wrong selection or dependence on hidden knowledge |
| Dynamic multiple-worker execution improves complex tasks | Controlled comparison against one capable worker using the same total budget | Extra calls add cost and failure without a quality gain |
| A general execution DAG is needed | Frequency and shape of real fan-out, fan-in, optional branch, and replanning cases | Linear Workflow and independent Tasks cover the workload |
| Automatic completion summaries create user value | Read rate, follow-up rate, satisfaction, latency, and cost | Users rely on cards or request explanations only occasionally |
| Visible Agent hierarchy helps users | Comprehension, trust, selection success, and correction behavior | Users care only about outcome, owner, and status |
| Task continuation benefits from session reuse | Follow-up success and context fidelity versus explicit-context fresh runs | Session state creates more ambiguity or recovery risk than value |
| Separate planner or verifier Agents improve reliability | Rubric-based evaluation and failure attribution | Equivalent quality from one Agent or deterministic checks |
| A2A-style federation is necessary | Concrete cross-team or cross-platform Agent integrations with opaque internals | All meaningful collaboration remains within one BuildMax control plane |
| Recursive worker delegation is valuable | Real tasks that require discovery beyond one planning layer | It mostly creates loops, budget expansion, and unclear accountability |

Evaluation should compare at least four paths under comparable budgets:

1. direct foreground answer;
2. one general-purpose durable Agent run;
3. supervisor routing to one specialist; and
4. dynamic planning with multiple workers.

The shared measures should include task quality, instruction fidelity, routing
accuracy, wall-clock latency, model calls, tokens, human interventions,
recovery behavior, policy violations, and the usefulness of the delivered
outcome.

## 11. Open Questions

- What is the stable product aggregate for a compound goal: Issue, Task, a new
  Goal or Plan, or a revision of Workflow?
- Which operations require human approval before an Agent can proceed, and how
  does a run durably wait for that decision?
- Should an Agent definition describe a user-visible specialist, an execution
  profile, an organizational owner, or some combination of the three?
- Which context belongs in the worker input, which is fetched by reference, and
  which must never cross the execution boundary?
- When should a Conversation request create or attach to an Issue?
- Which result fields need typed contracts before dynamic joins are safe?
- How should partial success be represented when some branches are optional?
- What failure should trigger deterministic retry, model replanning, human
  escalation, or terminal closure?
- Does a compound execution need one final synthesis artifact, or can multiple
  independently useful outcomes satisfy the goal?
- Which Agent integrations are genuinely outside BuildMax's trust and control
  plane and therefore need a federation protocol?

## 12. Recommendation

Describe BuildMax internally as a durable Agent execution platform with an
interactive front door, not as a permanently hierarchical two-Agent system.

Keep the physical foreground/background split. Make deterministic commands the
default for unambiguous control operations. Use the Conversation Agent for
interaction and semantic ambiguity, and separate presentation from authority.
Strengthen the durable kernel with a validated ExecutionSpec, Team-owned Task
semantics, least-privileged runs, approvals, and typed outcomes before adding a
general multi-Agent graph.

Then let evidence choose among one general Agent, routed specialists, authored
Workflows, and dynamic orchestrator-workers for each class of work. This gives
BuildMax the enterprise properties that matter now without freezing a product
hierarchy that future evidence may not support.
