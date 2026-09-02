# Codex Agent View: Two Execution Lifecycles, Not Two Mandatory Agent Tiers

> **Audience:** participants in the two-tier Agent architecture roundtable · **Status:** proposal — Codex Agent position, not a project decision

Related: [roundtable index](README.md),
[current Portal execution model](../../design/portal-execution-model.md), and
[product vision](../../design/product-vision.md).

## Contents

- [1. Thesis](#1-thesis)
- [2. First-Principles Derivation](#2-first-principles-derivation)
- [3. Three Meanings Of Orchestrator And Worker](#3-three-meanings-of-orchestrator-and-worker)
- [4. Assessment Of Current BuildMax](#4-assessment-of-current-buildmax)
- [5. Proposed Architecture](#5-proposed-architecture)
- [6. Runtime Semantics](#6-runtime-semantics)
- [7. Trust And Authority](#7-trust-and-authority)
- [8. Domain And Data Shape](#8-domain-and-data-shape)
- [9. Alternatives](#9-alternatives)
- [10. Evidence And Falsification](#10-evidence-and-falsification)
- [11. Recommendation](#11-recommendation)
- [12. Long-Term Hypothesis: Multi-Agent Becomes Infrastructure](#12-long-term-hypothesis-multi-agent-becomes-infrastructure)

## 1. Thesis

BuildMax should preserve the currently necessary foreground and detached
execution lifecycles but should not hard-code them as two mandatory Agent
identities or as the exhaustive lifecycle ontology.

The durable distinction is between:

- foreground interaction, which has a short and predictable response budget;
  and
- detached execution, which must outlive a connection and be retryable,
  cancelable, isolated, observable, and recoverable.

The correct target is one user-facing BuildMax Assistant backed by four
separate runtime responsibilities:

1. an interaction surface;
2. an optional semantic coordinator;
3. a deterministic durable orchestration kernel; and
4. isolated execution Agents plus outcome projection.

The coordinator may propose what to do. The orchestration kernel decides what
is authorized, persists what was accepted, and owns what has happened. Worker
Agents decide how to complete one bounded objective. Outcome projection reports
structured state independently of whether another model call succeeds.

This keeps the engineering value of the present worker substrate while
removing the assumption that a foreground Agent must be the semantic and
causal parent of every execution. `Two lifecycles` is shorthand for the two
modes the product already requires, not a claim that future Work cannot be
streaming, checkpointed, waiting for input, waiting for approval, or moved
between interactive and detached execution.

## 2. First-Principles Derivation

Users ask for outcomes, not Agent topology. A Portal system has to satisfy a
small set of constraints before any Agent roles are named.

### 2.1 Interaction

An interactive turn needs a coherent user relationship, bounded latency, and a
clear answer about what happened. It should not keep a browser request open for
hours or make the browser connection the owner of work.

### 2.2 Detached execution

Long-running or unattended work must survive disconnection and Server restart.
It needs an explicit lifecycle, idempotent state transitions, cancellation,
retry, resource and permission policy, evidence, and a stable output location.

### 2.3 Authority

The Team boundary, actor, quota, policy, model, tools, plugins, credentials, and
workspace scope must be validated by deterministic code. A model can recommend
a choice but cannot make an unrecorded choice authoritative.

### 2.4 Truth

Execution status comes from durable structured state. Model memory and natural
language summaries are projections, not sources of truth. A completion sentence
may be lost while the result remains intact.

### 2.5 Trust

Worker output is untrusted data. Receiving it must not grant it the authority of
a user instruction, and presenting it must not implicitly grant the presenter
permission to start more work.

These constraints derive two execution lifecycles and a durable control plane.
They do not derive two model personas, two model strengths, or a fixed
supervisor/subordinate relationship.

The long-term domain model should keep interaction mode, durability, authority,
topology, placement, and Agent identity orthogonal. Foreground and background
are useful defaults over those dimensions, not substitutes for them.

## 3. Three Meanings Of Orchestrator And Worker

The phrase `orchestrator/worker` commonly collapses three architectures.

| Meaning | Orchestrator responsibility | Worker responsibility | BuildMax today |
|---|---|---|---|
| Distributed job system | Persist, schedule, retry, cancel, constrain, and observe | Execute one claimed run | Strong match through Task, TaskRun, scheduler, and workers |
| LLM orchestrator-workers | Dynamically decompose, delegate, inspect, and synthesize | Solve independent or dependent subtasks | Partial match; Tier 1 can dispatch tasks but has no durable plan or join |
| Supervisor and specialists | Route by capability, domain, tools, or policy | Provide a bounded specialist capability | Early shape only; Agent summaries exist, stable capability contracts do not |

Anthropic's
[orchestrator-workers workflow](https://www.anthropic.com/engineering/building-effective-agents)
uses a central LLM to determine unpredictable subtasks, delegate them, and
synthesize the results. Amazon Bedrock's
[multi-Agent collaboration](https://docs.aws.amazon.com/bedrock/latest/userguide/agents-multi-agent-collaboration.html)
describes a supervisor that routes among named collaborators with distinct
roles. Those are useful patterns, not proof that every request benefits from a
model supervisor.

Microsoft's
[Agent orchestration guidance](https://learn.microsoft.com/en-us/azure/architecture/ai-ml/guide/ai-agent-design-patterns)
recommends deterministic routing or a simpler dispatcher when the correct
Agent or sequence is already identifiable. Amazon AgentCore's
[asynchronous runtime](https://docs.aws.amazon.com/bedrock-agentcore/latest/devguide/runtime-long-run.html)
also supports foreground and long-running behavior through one runtime and
session model. These examples support separating execution mode from Agent
identity.

## 4. Assessment Of Current BuildMax

### 4.1 What should remain

The current Portal execution design got its most important substrate decisions
right:

- TaskRun is the durable execution fact;
- status, output, Artifacts, trace, usage, and failure survive independently of
  the browser;
- task cards and Issue outputs are projections of stored state;
- terminal result delivery is a durable obligation rather than an in-memory
  callback;
- a worker does not speak directly to the user;
- `source_message_id` makes the user's request and Tier 1's normalized run input
  comparable;
- Agent revision and plugin pins improve provenance; and
- retry and cancellation are state transitions rather than conversational
  claims.

This is the distributed-system half of orchestrator/worker, and it is the
foundation on which any multi-Agent direction should build.

### 4.2 Tier 1 is overloaded conceptually

`internal/service/conversation` currently combines several roles:

- user-facing assistant;
- foreground question answerer;
- foreground-versus-background classifier;
- task continuation resolver;
- Agent selector;
- instruction normalizer;
- dispatcher; and
- completion presenter.

Its actual tool surface is much narrower than the word `orchestrator` suggests:
StartTask, ContinueTask, ListTasks, and GetTask. It has no general Team file,
Issue, Artifact, plan, or execution-policy capability. In the current product
it is primarily a probabilistic router and rewriter around a durable job
system.

Some of those decisions need language understanding. Others do not. Explicit
retry, cancel, task status, user-selected Agent, Issue assignment, Workflow
steps, and webhook dispatch should use deterministic commands. Current direct
task rerun behavior already demonstrates that bypassing the Tier 1 LLM is
compatible with the architecture.

### 4.3 Instruction normalization is a reliability boundary

When Tier 1 rewrites a user request into a run input, end-to-end reliability
becomes the product of several uncertain stages: intent interpretation,
instruction preservation, worker execution, and result presentation.
`source_message_id` makes loss inspectable but does not prevent it.

Normalization earns its model call only when decomposition, clarification,
context selection, or capability selection improves the result more than the
rewrite risks losing constraints. That is an empirical threshold, not a
property of the topology.

### 4.4 Automatic presentation is optional work

A durable result card already tells the truth about a run. Replaying every
finished run through the full Conversation Agent adds cost, latency, context,
and another failure point. It should be justified by observed user value.

Completion may instead produce a deterministic notification. A user can ask
for an explanation, or policy can request a bounded presenter call when a
particular product journey needs one.

### 4.5 Current result replay crosses the stated trust boundary

The accepted design says worker output is not a user instruction and is not in
LLM history by default. Current code nevertheless stores the synthetic
`[Task Result]` input with `role=user`, hides it from the Portal transcript, and
replays all stored messages into later Conversation turns.

Disabling tools for the immediate system-channel presentation turn is not
enough. A later user turn restores orchestration tools while retaining the raw
worker output in model context. An adversarial worker result can therefore
influence a later control decision.

Raw worker output should be read as typed, untrusted result data by a bounded
presenter. It should never be reclassified as a user message.

### 4.6 Conversation is still an execution parent by schema accident

`task.conversation_id` is required, so Issue Agent runs and Workflow steps
create synthetic Conversations that no user holds. Hiding them from a list
fixes the projection, not the ownership model.

Team should own Task. Conversation, source message, Issue, Workflow step, or
webhook should be explicit optional origins and delivery relationships.
Worker storage and authorization should be addressed by Team, Task, and
TaskRun rather than by Conversation.

### 4.7 BuildMax is not yet a durable LLM orchestrator-workers system

Tier 1 can start several independent Tasks, but there is no durable record of:

- the decomposition that created them;
- parent and child relationships;
- required and optional branches;
- dependency and join conditions;
- an overall completion criterion;
- failure and replanning policy; or
- which synthesis step turns sub-results into the requested outcome.

If those facts exist only in Conversation history, restart recovery reconstructs
text, not orchestration intent. That is temporary model-managed coordination,
not durable orchestration.

### 4.8 Existing Workflow advancement is not fully durable orchestration

The linear Workflow records are durable, but the transition from one terminal
TaskRun to the next step is still initiated by an in-process terminal callback
after the run's terminal transaction. A callback failure is logged; unlike
`task_result_delivery`, there is no durable advancement obligation that a sweep
can reclaim after a crash.

Creating the next step's Task and then binding it to the Workflow StepRun also
crosses store operations without one idempotent orchestration transaction. A
future retrying reconciler would need a stable event or node idempotency key to
avoid creating the downstream Task twice.

This distinction matters to the roundtable: BuildMax has durable execution
records and a durable result-delivery mechanism, but it should not yet claim a
general durable multi-step orchestration kernel. The result-delivery pattern —
persist the obligation, claim it idempotently, and retry it from stored facts —
is the pattern to generalize.

## 5. Proposed Architecture

The proposed target separates four responsibilities.

```text
User / Conversation
        |
        v
Optional semantic coordinator
understand, clarify, decompose, select, propose
        |
        | validated Plan / ExecutionSpec proposal
        v
Deterministic durable orchestration kernel
authorize, persist, dispatch, retry, cancel, join, recover
        |
        +--> TaskRun A --> Worker Agent
        +--> TaskRun B --> Worker Agent
        +--> Verify or synthesize run
        |
        v
Outcome projection
status, Artifacts, notification, optional explanation
```

### 5.1 Interaction surface

Conversation owns user and assistant messages, immediate answers, references to
shared work, and the single user-facing voice. It projects structured task and
outcome cards without encoding them as messages.

The product may show one BuildMax Assistant even when several bounded model
roles run behind it. Product identity does not require one shared authority or
one shared transcript.

### 5.2 Semantic coordinator

The coordinator appears only when language ambiguity or dynamic decomposition
requires it. It may:

- ask for clarification;
- propose foreground work or detached work;
- identify existing work to continue;
- propose subgoals and dependencies;
- recommend an Agent capability;
- propose context references and expected outputs; and
- inspect structured results and recommend replanning.

It does not directly own durable state. It produces a typed proposal that the
kernel validates and commits.

### 5.3 Orchestration kernel

The kernel is a deterministic service, not an LLM persona. It owns:

- authorization and Team scope;
- idempotency and concurrency;
- quota and admission;
- validated immutable ExecutionSpecs;
- task and run state machines;
- scheduling, cancellation, retry, and timeouts;
- parent/child and dependency state when dynamic plans need them;
- result and Artifact references;
- human-input and authorization interruptions;
- completion and join conditions;
- audit and trace correlation; and
- recovery after process loss.

Every model-requested state change returns through this boundary. A Worker may
request delegation, but the kernel validates and creates the child execution.

### 5.4 Worker Agent

A Worker owns how to complete one bounded objective inside its granted runtime.
It receives a fixed ExecutionSpec and may plan internally, use tools, checkpoint
its session, and publish structured results and Artifacts.

It does not decide whether it was authorized, expand its own permission set,
rewrite the durable parent plan, or declare the whole composite outcome
complete. It never bypasses the interaction surface to speak as the user's
assistant.

### 5.5 Presenter and verifier

Presentation and verification are separate bounded roles, not automatically the
same Conversation Agent.

A presenter receives structured status, selected result fields, and Artifact
references. It has no orchestration tools. Its generated explanation may be
stored as an assistant message explicitly linked to the run or composite
outcome.

A verifier receives objective acceptance criteria and evidence. It emits a
typed judgement that the kernel can use within a bounded retry or escalation
policy. A model saying `done` is not itself a state transition.

## 6. Runtime Semantics

### 6.1 Direct response and detached work share an entry point

The interaction surface should support three outcomes:

1. a direct Message when no durable work is needed;
2. a deterministic command against known work; or
3. a durable Task when the execution properties require it.

This resembles the distinction in the
[A2A protocol](https://a2a-protocol.org/dev/specification/), where a direct
Message and a stateful Task are different response shapes. A Task owns lifecycle
and Artifacts; Messages are communication and are not a reliable substitute for
critical state.

### 6.2 A plan is durable only when the system can resume it

Dynamic fan-out does not require a general workflow engine on day one. A small
durable shape may be sufficient:

- one orchestration or task-group identifier;
- parent execution identifier;
- dependency edges or a declared join policy;
- node ExecutionSpecs;
- node terminal states; and
- a recorded coordinator decision for each expansion or replan.

The product should add this only after evaluation shows recurring dynamic
decomposition. Until then, independent Tasks and explicit Workflows remain
simpler and more truthful.

### 6.3 Agent-to-Agent communication is typed

Workers report through durable result records or a mailbox/event contract, not
through the user's transcript. The kernel may translate a report into:

- a state transition;
- a newly admitted child execution;
- an `input required` interruption;
- an `authorization required` interruption;
- a verifier run;
- a presenter run; or
- a user notification.

Critical information is re-readable after disconnect. Streaming is an
optimization for latency, not the durability mechanism.

### 6.4 Synthesis is an execution node when it matters

If several workers jointly answer one goal, synthesis should be an explicit
bounded node with declared inputs and provenance. Its failure must not erase
the child results. The user can still inspect partial outcomes and retry only
the synthesis.

### 6.5 Human participation is a state, not a failed run

Long-running Agents may need clarification, approval, a credential, or an
external decision. Treating all of those as failure or making the worker speak
around Tier 1 loses intent and auditability.

The eventual run lifecycle should distinguish terminal states from interrupted
states such as `INPUT_REQUIRED` and `AUTH_REQUIRED`, with explicit resumption
authority and timeout behavior. The exact states require a separate accepted
design before changing the current lifecycle.

## 7. Trust And Authority

### 7.1 Model output is a proposal

Coordinator, Worker, verifier, and presenter outputs all cross a trust boundary.
Each consumer must parse a declared contract, validate identifiers and scope,
and refuse undeclared authority.

### 7.2 Least authority by role

| Role | Typical authority |
|---|---|
| Conversation Agent | Read conversation context; propose or invoke user-authorized foreground operations |
| Coordinator | Read bounded goal and capability summaries; propose a Plan or ExecutionSpec |
| Kernel | Validate and commit durable state under Team policy |
| Worker | Use only the tools, workspace, credentials, and model fixed for its run |
| Verifier | Read declared evidence; emit a judgement; no mutation tools |
| Presenter | Read selected outcome data; emit user-facing text; no orchestration tools |

The same model may implement several roles, but role-specific input and
authority must remain separate.

Weakening a user-visible supervisor/subordinate ontology does not mean weakening
internal Agent identity. Planner, Worker, verifier, presenter, and approver may
need distinct security principals, revision provenance, grants, and audit
records even when the interface presents one BuildMax Assistant. Product
persona and accountable principal are different dimensions.

### 7.3 Result isolation

Raw Worker text should not enter the user's message role or a future
orchestration context by default. Structured fields and Artifact references
should be selected explicitly for each consumer. This reduces prompt injection,
context growth, and accidental authority transfer.

### 7.4 Centralization has a bounded purpose

The deterministic kernel is a central authority, not necessarily a single
process. Its concurrency and delivery semantics must work across supported
Server topology. Process-local turn queues and connection registries cannot be
described as a distributed orchestration guarantee.

## 8. Domain And Data Shape

The stable product objects should express ownership, execution, and origin
separately.

```text
Team
  +-- Issue / Workflow / Conversation
  +-- Task or Work
        +-- optional origin relations
        +-- stable objective and Agent selection
        +-- TaskRun attempts
              +-- immutable ExecutionSpec snapshot
              +-- status and interruption state
              +-- output and Artifact references
              +-- trace, usage, and policy evidence
              +-- optional summary or verification
```

The immediate direction remains:

- `team_id` authoritative for Task ownership;
- `conversation_id` nullable and relational rather than parental;
- source message optional and traceable;
- Issue and Workflow relations explicit;
- storage addressed by Team, Task, and TaskRun;
- Conversation transcript limited to actual communication; and
- task cards and outcome views derived from execution records.

An ExecutionSpec should eventually snapshot the fields that determine what ran:

- normalized instructions and source references;
- Agent revision;
- model profile;
- tool and plugin pins;
- permission, sandbox, network, and credential grants;
- workspace and attachment references;
- time, token, and resource budgets;
- expected output contract; and
- actor, trigger, and parent provenance.

The field set should be evidence-driven. Naming the concept does not justify
inventing unused catalog dimensions.

## 9. Alternatives

### 9.1 Keep the current mandatory Tier 1 to Tier 2 chain

This is operationally understandable and already partly implemented. It keeps
one user voice and gives the model flexibility. It also makes semantic routing,
instruction rewriting, and completion presentation mandatory costs and failure
points. It leaves durable plan state in conversation context and preserves the
current result trust-boundary problem.

This alternative is acceptable only if conversation evaluation shows that the
foreground Agent materially improves outcomes and that automatic summaries are
valuable enough to justify their cost and risk.

### 9.2 One synchronous Agent

This removes delegation semantics but couples long work to an interaction and
loses durable retry, cancellation, isolation, and recovery. It is not a viable
Portal execution architecture.

### 9.3 One logical Agent with foreground and background modes

This is close to the recommendation at the product layer. One BuildMax
Assistant can answer immediately or detach a durable run using the same Agent
Core. Internally it still needs the deterministic kernel and role-specific
authority. A single product persona does not mean one process, one model call,
or one shared context.

### 9.4 Fully decentralized Agent handoff

Agents can discover one another and transfer control without a central planner.
This may help cross-vendor or cross-domain systems, but it complicates Team
authorization, quota, cancellation, audit, and global completion. BuildMax may
support A2A-style interoperability later while still requiring every local
state change to pass through its kernel.

### 9.5 Predefined deterministic Workflows only

This maximizes predictability and governance but cannot cover goals whose
subtasks emerge from inspection. It remains the right choice when the sequence
is known. Dynamic coordinator-driven plans should complement explicit
Workflows, not replace them.

### 9.6 A general orchestration DAG immediately

This would make fan-out/fan-in explicit but risks building a workflow platform
before usage demonstrates the need. Start with the smallest durable relation
that can represent observed dynamic plans, and keep the existing roadmap's
evidence discipline.

## 10. Evidence And Falsification

The recommendation should change if evidence shows that its extra boundaries
cost more than they protect.

Build a conversation evaluation adapter and compare at least:

1. mandatory Tier 1 dispatch plus automatic completion summary;
2. deterministic commands with coordinator use only for ambiguous requests;
3. one logical Assistant with an explicit or suggested background mode; and
4. dynamic multi-worker decomposition for tasks that genuinely require it; and
5. a small durable Agent actor prototype that can move between interactive,
   detached, and waiting-for-input states without a separate semantic handoff.

Measure:

- foreground-versus-background decision accuracy;
- new Task versus continuation accuracy;
- Agent or capability selection accuracy;
- constraint retention between source message and ExecutionSpec;
- successful end-to-end outcome rate;
- useful outcome latency;
- model calls, tokens, and cost per outcome;
- user corrections after dispatch;
- automatic-summary read, follow-up, and dismissal behavior;
- partial-result recovery after a failed synthesis;
- duplicate and lost transitions under restart and concurrency;
- adversarial Worker output influencing later orchestration; and
- how often tasks require dynamic fan-out, joins, replanning, or specialist
  isolation.

The proposed separation is falsified or should be simplified if deterministic
routing plus role isolation does not improve reliability, security, cost, or
recovery in representative Portal journeys.

The mandatory Tier 1 chain is falsified if it loses user constraints, creates
or continues the wrong work, produces unwanted summaries, or expands prompt
injection authority more often than it improves outcomes.

## 11. Recommendation

Retain the physical and lifecycle separation already expressed by Server,
scheduler, TaskRun, and Worker. Stop treating that separation as proof of a
fixed two-Agent hierarchy.

Evolve toward:

- one user-facing BuildMax Assistant;
- deterministic direct operations for known commands;
- an optional semantic coordinator for ambiguity and dynamic decomposition;
- a durable orchestration kernel as the only authority for execution state;
- isolated Worker Agents that own bounded execution, not global control;
- typed reports and mailbox/events rather than transcript-based Agent
  communication;
- explicit verifier and synthesis runs where a composite outcome needs them;
- optional, tool-less presentation rather than mandatory replay through the
  Conversation Agent; and
- durable outcome projection independent of every model call.

In short: keep two execution lifecycles, replace two mandatory Agent tiers with
orthogonal interaction, coordination, orchestration, execution, and
presentation roles.

## 12. Long-Term Hypothesis: Multi-Agent Becomes Infrastructure

Multi-Agent collaboration is unlikely to disappear, but much of today's
visible Agent hierarchy may.

Many current Agent teams compensate for model limitations by assigning the
same model several prompt personas such as researcher, writer, reviewer, and
manager. As models improve their planning, context use, tool use, and internal
parallel reasoning, these roles may collapse into one logical Agent with
several internal steps or rollouts. Users should not have to understand how
many model invocations produced one answer.

The multi-Agent boundaries that should survive are those justified by
properties a stronger model does not erase:

- distinct security principals, owners, approvals, and audit responsibility;
- parallel work in independent workspaces, regions, or compute pools;
- context and prompt-injection isolation;
- independent failure, cancellation, retry, and cost accounting;
- specialist tools, data, policy, or hardware; and
- collaboration across organizational or vendor trust boundaries.

This suggests a compression at the product layer and a strengthening at the
infrastructure layer. The user may see one BuildMax Assistant while the runtime
creates several bounded executions. Internally, the executions that remain
distinct should have explicit identity, grants, provenance, lifecycle, and
contracts rather than theatrical personas.

BuildMax should therefore model multi-Agent as a topology selected per Work,
not as the one fixed ontology of the product. A Work may use direct execution,
one Agent, a static Workflow, a Planner and one Worker, several parallel
Workers, a durable actor, or external Agent collaboration. Additional Agents
are justified only when they materially improve at least one of permission
isolation, organizational accountability, context isolation, specialist
capability, parallel latency, local recovery, or independent verification.

The long-term prediction is:

> Multi-Agent becomes less visible as a product metaphor and more important as
> a governed execution substrate. Prompt personas shrink; principals,
> capabilities, attempts, evidence, and trust boundaries remain.
