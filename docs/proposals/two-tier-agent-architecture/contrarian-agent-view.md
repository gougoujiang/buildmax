# Contrarian Agent View: A Red-Team Challenge To Two-Tier Agent Architecture

> **Audience:** participants in the two-tier Agent architecture roundtable · **Status:** proposal — independent Contrarian Agent view, not a project decision

Opened: 2026-08-30

Related: [roundtable index](README.md),
[current Portal execution model](../../design/portal-execution-model.md),
[product vision](../../design/product-vision.md),
[evaluation system](../../design/evaluation-system.md), and
[roadmap](../../ROADMAP.md).

## Contents

- [1. Red-Team Thesis](#1-red-team-thesis)
- [2. Goals And Non-Goals](#2-goals-and-non-goals)
- [3. Two Lifecycles Are Not A First-Principles Result](#3-two-lifecycles-are-not-a-first-principles-result)
- [4. Weakening Agent Ontology May Weaken Governance](#4-weakening-agent-ontology-may-weaken-governance)
- [5. What BuildMax Implements Today](#5-what-buildmax-implements-today)
- [6. Central Coordination Creates Its Own Failure Domain](#6-central-coordination-creates-its-own-failure-domain)
- [7. Trust, Context, And Synthesis Are Unresolved](#7-trust-context-and-synthesis-are-unresolved)
- [8. Alternatives The Current Framing Underweights](#8-alternatives-the-current-framing-underweights)
- [9. A Topology-Neutral Substrate](#9-a-topology-neutral-substrate)
- [10. Conclusions That Survive The Challenge](#10-conclusions-that-survive-the-challenge)
- [11. Decision Questions](#11-decision-questions)
- [12. Evidence And Falsification](#12-evidence-and-falsification)
- [13. Recommendation](#13-recommendation)

## 1. Red-Team Thesis

BuildMax should not treat either the current Tier 1/Tier 2 design or the softer
claim of "two execution lifecycles, not two mandatory Agent identities" as the
final architecture.

Only one conclusion follows directly from the product constraints: work that
must survive disconnection needs durable state, explicit authority, bounded
resources, observable execution, and recovery semantics independent of a
browser connection. That conclusion does not establish that there are exactly
two lifecycles, that foreground work is non-durable, that background work is
non-interactive, or that one Conversation Agent should coordinate all work.

The stronger target is a durable work substrate on which several orchestration
policies can operate:

- direct execution when the user or product already selected the capability;
- deterministic Workflow execution when the sequence is known;
- an optional model coordinator when decomposition is genuinely dynamic;
- a durable Agent actor when one continuing identity should own the work; and
- a governed event or blackboard topology when peers should cooperate without
  one global coordinator.

The product may present one conversational voice, but the runtime should make
execution principals, grants, revisions, evidence, and responsibility more
explicit rather than less. The current discussion risks weakening the wrong
ontology while preserving the wrong number of lifecycle classes.

## 2. Goals And Non-Goals

### 2.1 Goals

This view attempts to:

- identify assumptions that neither user needs nor distributed-systems
  constraints require;
- test the two-tier proposal against enterprise authority, recovery, cost, and
  multi-Agent coordination requirements;
- distinguish a user-facing Assistant identity from an execution principal;
- consider architectures that do not place a central model in every path; and
- name evidence that could disprove the red-team position.

### 2.2 Non-goals

This view does not propose removing workers, weakening durable TaskRun state,
or moving long-running ownership back into a browser request. It does not claim
that BuildMax should implement every orchestration topology now. It also does
not treat common industry terminology as proof that a particular topology is
correct for BuildMax.

## 3. Two Lifecycles Are Not A First-Principles Result

The current Portal design divides foreground and background by connection,
latency, durability, isolation, specialization, and parallelism. Those are
useful dimensions, but combining them into two tiers assumes that they remain
correlated.

They do not necessarily remain correlated:

- A short foreground tool call may have an irreversible external side effect
  and therefore require durable idempotency, audit, and cancellation intent.
- A long background investigation may need to ask the user a question, wait
  for approval, stream progress, and then resume.
- A run may begin interactively, detach when its latency exceeds the turn
  budget, enter a waiting state, and later return to interactive execution.
- A long read-only reasoning operation may require no separate OS isolation,
  while a fast code-changing operation may require the strongest boundary.
- A foreground turn can be lost during Server restart and therefore needs a
  defined durability and recovery contract even if the model call is short.
- Work may be durable because of its effect semantics, not because of elapsed
  time or whether a user is connected.

The current TaskRun state machine supports `PENDING`, `SCHEDULED`, `RUNNING`,
`SUCCEEDED`, `FAILED`, and `CANCELED`. It has no durable state for waiting on
user input, waiting on approval, partial success, a blocked dependency,
checkpointed suspension, or required compensation. This is a sound batch-job
state machine, not yet a general long-lived Agent lifecycle.

The durable model should therefore describe orthogonal policy attributes and
state transitions, for example:

| Dimension | Illustrative values |
|---|---|
| Interaction | immediate, streaming, detached, needs input |
| Durability | ephemeral, recorded, checkpointed, recoverable |
| Placement | Server, worker, local client, external Agent |
| Authority | presenter, planner, executor, approver |
| Orchestration | direct, Workflow, coordinator, peer |
| Effect semantics | read-only, idempotent, compensatable, irreversible |

Foreground and background may remain useful product language. They should not
be the root domain classification until evidence shows that these dimensions
continue to move together.

## 4. Weakening Agent Ontology May Weaken Governance

The proposal to present one BuildMax Assistant and treat Planner, Presenter,
and Worker as runtime roles is attractive as user experience. It becomes
dangerous if that simplification reaches the authority model.

An enterprise Agent is potentially an execution principal, not merely a prompt
or model persona. The system must be able to answer:

- which principal proposed a plan;
- which principal read each data source;
- which grants permitted each tool call;
- which principal produced or published an Artifact;
- who approved an authority or budget change;
- which model, Agent revision, tool set, plugin set, and policy actually ran;
  and
- who is accountable when a result or side effect is wrong.

A Conversation Agent, Planner, Presenter, Verifier, and Worker may use the same
base model and appear as one Assistant while still requiring distinct
credentials, context, grants, and audit identities. A Presenter should not
inherit dispatch authority. A Planner should not automatically inherit the
Worker's data access. A Worker should not be able to convert its own output into
a new grant.

The better distinction is:

> Keep the product persona simple, but make the principal, role, provenance,
> and grant ontology explicit and enforceable.

Execution cards and audit views should disclose which Agent and revision ran,
which policy selected it, which capabilities it received, and whether the
selection was user-pinned, deterministic, or model-recommended. A single voice
must not become a single, ambiguous authority.

## 5. What BuildMax Implements Today

### 5.1 It is a durable job system with an Agentic router

The persistent execution substrate is real: Task and TaskRun carry status,
output, Artifacts, trace, usage, failure, retry lineage, cancellation intent,
worker liveness, Agent revision, plugin pins, and source-message provenance.
The scheduler and workers execute a claimed run independently of a browser.

The model orchestration half is much smaller. The Conversation service gives
Tier 1 four task tools: StartTask, ContinueTask, ListTasks, and GetTask. There
is no persistent decomposition, parent-child work relation, dependency graph,
join policy, completion policy, structured child contract, verifier, or
replanning record.

The current shape is more accurately described as:

```text
model router and instruction rewriter
                |
                v
       durable single-Agent job
                |
                v
        model result announcer
```

That is useful, but it is not yet an LLM orchestrator-workers cluster. Calling
it one risks designing future data models around capabilities that do not yet
exist and overlooking simpler direct or Workflow paths that already fit many
requests.

### 5.2 Selection is traceable but not fixed at authorization time

The worker route resolves Agent instructions and plugin activations when the
worker fetches the run. It then records the Agent revision and plugin pins that
were served. This records what happened, but it does not guarantee that what
happened is what the user or coordinator authorized.

An Agent can be selected under one description, edited before the worker
claims the run, and executed under a different instruction revision. The
revision record makes the race visible after execution; it does not close the
race. The same distinction applies to policy, model, tools, context, and plugin
resolution unless an immutable specification fixes them before dispatch.

An enterprise execution contract should become immutable at admission or an
explicit approval boundary. Worker claim should verify and receive that
contract, not complete its semantic definition.

### 5.3 Provenance is evidence, not intent preservation

`source_message_id` allows an operator to compare the user's request with the
run input produced by Tier 1. That is valuable evidence. It cannot prevent Tier
1 from dropping a constraint, adding an unauthorized objective, continuing the
wrong Task, or selecting the wrong Agent.

The system should not confuse post-hoc inspectability with a correctness or
authorization guarantee. Whether instruction normalization adds more value
than it loses remains an evaluation question.

### 5.4 The worker is not yet the claimed enterprise trust boundary

The roadmap states that worker runs currently inherit the CLI baseline, omit
the intended worker sandbox surface, and use an allow-all tool policy. Process
and resource limits and hook or MCP child-process boundaries remain Beta work.

Server-to-worker placement currently provides lifecycle and workspace
separation. It does not yet prove least privilege, OS containment, egress
restriction, or bounded subprocess behavior. Recursive delegation or a larger
Agent catalog would multiply this unresolved authority surface.

## 6. Central Coordination Creates Its Own Failure Domain

Central orchestration can improve coherence, but it also concentrates latency,
cost, context, and availability.

### 6.1 Conversation serialization becomes cluster serialization

BuildMax serializes user turns and task-result turns through one in-memory
queue per Conversation. The queue is bounded, and the current multi-instance
topology does not share it. If several workers complete together, their model
presentation turns contend with new user messages for the same queue.

This creates questions that the two-tier diagram does not expose:

- Do result presentations have lower priority than real user input?
- Can ten simultaneous completions prevent the user from correcting or
  canceling work?
- Does retrying a refused presentation amplify queue pressure?
- Which Server instance owns the next planning decision after failover?
- Can two coordinators consume the same result and produce divergent plans?

A durable delivery obligation preserves the sentence's eventual attempt, but
it does not solve plan serialization, priority, or multi-instance ownership.

### 6.2 Full-history coordination scales poorly

Conversation turns replay stored history. If every worker completion produces
another full Conversation turn, later summaries carry an increasingly large
context. For a fan-out of many workers, repeated central synthesis can approach
quadratic prompt cost unless context is compacted, indexed, or queried by
reference.

The central model also becomes a semantic single point of failure. A poor
decomposition affects every child. A poisoned result can distort the remaining
plan. A context-window limit can prevent final synthesis even though all
workers succeeded.

### 6.3 A deterministic kernel can also become a bottleneck

Replacing the Conversation Agent with a deterministic orchestration kernel is
not automatically correct. A kernel that owns every possible plan transition
can grow into a general Workflow language, duplicate model reasoning, and
become a large central service whose schema must anticipate uncertain work.

The boundary must remain narrow. Deterministic code should own authorization,
leases, budgets, state transitions, dependency satisfaction, idempotency, and
recorded facts. It should not pretend to judge semantic completeness that only
a domain model, verifier, user, or explicit contract can assess.

## 7. Trust, Context, And Synthesis Are Unresolved

### 7.1 Current result replay violates the documented trust boundary

The current design says Worker output is untrusted data, is not a user
instruction, and is not replayed in LLM history by default. Current code sends a
synthetic `[Task Result]` through a system channel but stores it with
`role=user`. The system channel removes tools only from the immediate
presentation turn. A later normal user turn reloads the stored result as user
input and restores StartTask and ContinueTask.

An adversarial Worker result can therefore persist in the Conversation context
and influence a later control decision. Hiding the row from the Portal
transcript changes presentation, not model trust.

This must be treated as an architecture-level indirect prompt-injection path.
Raw Worker output should remain typed, provenance-marked data. A bounded
Presenter may read it without dispatch tools, but the result must not acquire
user authority merely because another model summarized it.

### 7.2 Separating a Presenter is necessary but insufficient

A future Planner or Verifier will still read Worker output and Artifacts.
Prompt injection survives role separation whenever untrusted text can persuade
a privileged role to act. Required controls include:

- provenance and trust labels that survive storage and retrieval;
- typed result and delegation envelopes;
- explicit separation of data fields from instruction fields;
- capability checks at every state-changing call;
- renewed user authorization for high-risk intent changes;
- bounded, reference-based context assembly; and
- independent verification where one model's output could trigger material
  external effects.

No prompt wording can replace these boundaries.

### 7.3 Current result synthesis lacks adequate evidence

The automatic result turn receives status, an error, or a bounded text excerpt.
It does not receive a structured output contract, complete Artifact manifest,
trace evidence, claim-to-source mapping, sibling results, or a parent goal's
completion policy. A concise summary of truncated free text is presentation,
not trustworthy multi-Agent synthesis.

Large-scale synthesis needs result structures such as:

- findings with confidence and unresolved questions;
- claims linked to evidence and Artifact references;
- machine-checkable completion criteria;
- child-run lineage and dependency status;
- contradictions and verifier decisions; and
- bounded retrieval rather than copying every child transcript into one
  prompt.

## 8. Alternatives The Current Framing Underweights

### 8.1 Durable Agent actor

A Conversation, Issue, Goal, or dedicated Agent Session can be a durable actor.
Inputs, tool outcomes, checkpoints, and authority changes are events. An
executor holding a lease advances the actor one step. With a connected user it
streams interactively; without one it continues on a worker; when it needs a
decision it enters a durable waiting state.

This is not a synchronous Agent owned by a browser. The browser is only a view
and input source. It also need not mean one OS process remains alive. The actor
can move between executors while retaining one logical identity and journal.

BuildMax already has some ingredients: persisted Conversation messages,
serialized turns, Task session identifiers, worker continuation, TaskRun state,
and durable storage. The remaining work is substantial: leases, checkpoints,
provider-state compatibility, effect journals, and recovery semantics. That
cost is a reason to evaluate the model, not a reason to exclude it from the
architecture comparison.

The durable actor is strongest when continuity of one Agent's understanding is
more valuable than decomposition into separate Worker identities. It reduces
the semantic handoff in which Tier 1 rewrites the user's intent for Tier 2.

### 8.2 Issue or Case-first orchestration

For enterprise work, the durable parent may be an Issue, Case, or Goal rather
than a Conversation or Agent. It owns a plan containing deterministic steps,
Agent steps, approval steps, dependencies, and verification. Conversation is a
control and explanation surface over that work.

```text
Issue or Goal
  |
  +-- durable plan
       +-- deterministic step
       +-- Agent step
       +-- approval step
       +-- verification or synthesis step
```

This model makes responsibility, SLA, partial success, reassignment, approval,
and progress visible. A model Planner may propose or amend the plan, but the
plan and state machine remain authoritative. It is often a better enterprise
default than an invisible model decomposition held in Conversation history.

### 8.3 Direct durable execution

When the user, Issue, Workflow, webhook, or API has already selected the Agent
and objective, BuildMax should validate an execution specification and create a
run directly. No model coordinator or automatic model summary is required.

Direct execution is not merely an optimization. It is the control baseline
against which a coordinator must prove that its extra interpretation, latency,
cost, and failure modes improve outcomes.

### 8.4 Blackboard or event-driven Agent collaboration

Agents can cooperate through a governed board or mailbox of typed work,
findings, evidence, dependency requests, and delegation proposals. A
coordinator may exist for one Goal without becoming the global path for every
Conversation.

This avoids a single model holding all child context and allows specialist
Agents to react to relevant events. It also creates hard problems: duplicate
claims, convergence, deadlock, budget competition, conflicting findings,
authorization propagation, and understandable progress. Those costs argue for
a governed substrate, not for excluding the topology.

### 8.5 Human-directed plan

For expensive or high-risk work, the default may be plan-before-execution:

1. a model proposes a structured plan;
2. the user reviews Agents, permissions, budgets, and dependencies;
3. the system fixes an approved execution specification;
4. workers execute it; and
5. material plan changes or risky effects require renewed approval.

This sacrifices some autonomy but may provide a better enterprise trust and
cost boundary than either a universal supervisor Agent or a hidden automatic
router.

## 9. A Topology-Neutral Substrate

BuildMax should make the durable substrate neutral to which orchestration
policy produced a run. A possible conceptual model is:

```text
Work or Goal
  +-- versioned Plan, optional
  +-- immutable ExecutionSpec
  +-- Run or Attempt
  +-- Event or Mailbox
  +-- Artifact and Evidence
  +-- Approval and PolicyDecision
  +-- AgentPrincipal and CapabilityProfile
  +-- projections
       +-- Conversation
       +-- Issue
       +-- Notification
```

An `ExecutionSpec` should fix at least:

- objective and instructions;
- actor and origin;
- Agent identity and revision;
- model and execution profile;
- tool, plugin, credential, and network grants;
- workspace and context references;
- time, token, cost, and resource budgets;
- output and completion contracts;
- effect and retry policy; and
- approval evidence where required.

The substrate should support several policy adapters:

| Policy | Who chooses the next work | Appropriate use |
|---|---|---|
| Direct | User or deterministic product code | Explicit Agent or known operation |
| Workflow | Persisted state machine or DAG | Stable, repeatable process |
| Coordinator | Bounded model Planner plus kernel validation | Dynamic decomposition |
| Durable actor | One journaled logical Agent | Long-lived continuity and interaction |
| Peer or blackboard | Governed event and claim protocol | Distributed specialist collaboration |

This model postpones topology commitment without postponing the contracts that
every topology needs: authority, persistence, evidence, budgets, and failure
semantics.

## 10. Conclusions That Survive The Challenge

The red-team position still supports several current design conclusions.

### 10.1 The browser cannot own work

Long-running work must survive Socket, HTTP request, browser, and individual
Server-process lifetimes.

### 10.2 TaskRun and outcome projection are load-bearing

Structured run state, Artifact identity, trace, usage, cancellation, failure,
and liveness should remain durable. Cards and Issue outcomes should be derived
from those facts and should survive a failed model summary.

### 10.3 Transcript and execution data require separate trust levels

A Worker result is neither a user message nor system authorization. A natural
language summary is a projection, not execution truth.

### 10.4 Conversation should not be the mandatory execution parent

Team or Work should own execution. Conversation, source message, Issue,
Workflow step, webhook, or external Agent should be explicit origins and
delivery targets.

### 10.5 Models propose; deterministic boundaries authorize

Models may interpret goals, propose plans, select candidates, verify semantic
criteria, or explain results. Server-owned policy must validate grants,
budgets, identity, state transitions, and immutable execution facts.

### 10.6 Simple commands should bypass semantic coordination

Explicit status, cancellation, retry, user-pinned Agent selection, Workflow
steps, and webhook dispatch should not require a probabilistic routing turn.

## 11. Decision Questions

### 11.1 Product and ownership

- What is the stable user-owned work object: Conversation, Issue, Task, Goal,
  or Plan?
- Are Task runs continuations of one goal, attempts of one specification, or
  independent instructions sharing a session?
- Is one Assistant only a presentation choice, or does it imply one authority?
- When must the product disclose the actual executing Agent and grants?
- Are task completion, goal completion, and user acceptance distinct states?

### 11.2 Principal and authority

- Is an Agent a prompt bundle, capability profile, execution principal, or a
  combination with separate identities?
- Which credentials and tools belong to Planner, Presenter, Verifier, and
  Worker?
- Who may create child work, with what depth, fan-out, and budget?
- At what boundary are model, Agent revision, plugins, tools, context, and
  policy fixed?
- How does an Agent request more authority without granting it to itself?

### 11.3 Lifecycle and effects

- Must Run support waiting for input, approval, or dependencies?
- What durability and idempotency contract applies to foreground turns?
- Can one Work move between interactive and detached execution?
- Does a user reply resume the same Run or create a new attempt?
- How are partial success, compensation, and irreversible effects represented?
- After a crash, how does the system distinguish no effect from an unreported
  effect?

### 11.4 Orchestration topology

- Which requests require a Planner, and which should dispatch directly?
- Is Plan a durable product object or an internal execution record?
- Which dependency, join, quorum, race, fallback, and verification semantics
  are actually needed?
- Is a coordinator global, per Conversation, per Goal, or a short-lived call?
- Can Workers request delegation, and who validates the request?
- Who decides semantic completion when the kernel only knows structural state?

### 11.5 Context and outcomes

- Does a Worker receive original user content, normalized instructions, or
  both with distinct trust labels?
- What prevents constraint loss during normalization?
- Which output contracts must be typed rather than free text?
- How are large Artifacts, contradictory findings, and evidence retrieved for
  synthesis?
- Which data can enter Conversation model history?
- How does trust provenance survive summaries and derived Artifacts?

### 11.6 Cost, scale, and recovery

- How many model calls and tokens are acceptable per useful outcome?
- What is the priority relationship between user turns and result processing?
- How does plan ownership move across Server instances?
- How are duplicate events and non-idempotent effects handled?
- What limits fan-out, recursive delegation, and replanning loops?
- When does a cheaper deterministic or model router outperform a strong
  coordinator?

## 12. Evidence And Falsification

The decision should compare at least these execution modes on the same tasks:

1. direct durable run with no coordinator and no automatic summary;
2. current Tier 1 router, Worker, and automatic result turn;
3. deterministic routing with a Planner only for ambiguous requests;
4. a durable Plan or DAG containing Agent steps; and
5. a small durable-actor prototype that can detach and wait for input.

Evaluation should measure:

- user-constraint preservation from source request to executed specification;
- incorrect dispatch, continuation, and Agent selection;
- end-to-end latency, model-call count, token cost, and queue delay;
- completion quality and claim-to-evidence accuracy;
- adversarial Worker-output influence on later privileged decisions;
- recovery after Server restart, worker loss, duplicate delivery, and
  multi-instance ownership change;
- repeated or ambiguous external side effects;
- the frequency of genuine dynamic decomposition, fan-out, replanning,
  approval, and waiting-for-input behavior;
- user value from automatic summaries versus durable cards and on-demand
  explanation; and
- whether operators can identify the responsible principal, policy, grants,
  inputs, and evidence for every outcome.

The current evaluation design says the Conversation adapter remains unbuilt.
Until that adapter measures Tier 1 decisions and result return, claims that the
central Conversation Agent improves end-to-end reliability remain hypotheses.

This red-team position would be weakened by evidence that:

- real workloads cluster cleanly into exactly two lifecycle classes;
- foreground recovery and background interaction are negligible;
- a central coordinator materially improves success and constraint fidelity
  after accounting for cost and additional failure stages;
- central context and queue contention remain bounded under realistic fan-out;
- users consistently benefit from automatic model synthesis; and
- distinct execution principals add audit complexity without improving policy
  enforcement or incident diagnosis.

## 13. Recommendation

Do not replace the current two-Agent wording with another architecture decision
that still presumes exactly two lifecycles. Treat foreground/background as
product modes and Server/worker as current placement, not as the foundational
domain taxonomy.

Preserve the durable TaskRun and outcome-projection work. Correct the result
trust-boundary violation, make execution specifications immutable at an
authorization boundary, and model Agent roles as explicit principals and
grants even when the product presents one Assistant.

Then evaluate direct execution, deterministic Workflow, optional
coordinator-workers, and durable actor operation against the same evidence.
Build the shared substrate so a future blackboard or peer topology can use the
same Work, Run, Event, Artifact, policy, and audit contracts without making one
central Conversation Agent the semantic parent of every action.

The red-team conclusion is therefore:

> Durable execution is necessary. Two tiers are not. A single user-facing voice
> is useful. A single ambiguous authority is not. BuildMax should commit first
> to a topology-neutral, principal-aware, event-driven work substrate and make
> each Agent orchestration pattern earn its place with evidence.
