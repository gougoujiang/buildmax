# Two-Tier Agent Architecture Roundtable

> **Audience:** maintainers, product designers, and Agent contributors · **Status:** proposal — under discussion

Opened: 2026-08-30

Related current documents:

- [Portal execution model](../../design/portal-execution-model.md)
- [Product vision](../../design/product-vision.md)
- [Surface positioning](../../design/surface-positioning.md)
- [Evaluation system](../../design/evaluation-system.md)
- [Current-state assessment](../../current-state.md)

## Contents

- [1. Question](#1-question)
- [2. Why Reopen It](#2-why-reopen-it)
- [3. Discussion Protocol](#3-discussion-protocol)
- [4. Contributions](#4-contributions)
- [5. Decision Boundary](#5-decision-boundary)
- [6. Evidence Needed](#6-evidence-needed)
- [7. Market-Derived Architecture Proposal](#7-market-derived-architecture-proposal)
- [8. Focused Runtime Notes](#8-focused-runtime-notes)

## 1. Question

What is the most principled architecture and runtime shape for BuildMax's
Portal Agent system?

The current product describes a foreground Tier 1 Conversation Agent and a
background Tier 2 execution Agent plane. This roundtable asks whether that is
the right durable abstraction, or whether the stable boundary is instead among
interaction, deterministic orchestration, durable execution, and outcome
presentation.

The question is not whether long-running work needs workers. It does. The open
question is which responsibilities require an Agent, which require a durable
state machine, how they communicate, and which concepts users should see.

## 2. Why Reopen It

The current design corrected important defects: TaskRun is the execution truth,
outcome cards are projections of durable state, terminal result delivery is an
obligation rather than a socket callback, and a worker does not speak directly
to the user.

Those corrections do not by themselves prove that every Portal request should
pass through a foreground orchestrator Agent, or that every finished run should
trigger another foreground model call. Current Tier 1 behavior also predates
evidence from a conversation evaluation adapter, which remains unbuilt.

This discussion therefore treats the accepted design as the current decision
and implementation context, not as the answer the roundtable is required to
preserve.

## 3. Discussion Protocol

Each participating Agent writes an independently named position file in this
directory. A contribution should:

- identify the authoring Agent in the filename and title;
- distinguish observations about current code from proposed direction;
- state its strongest thesis before supporting detail;
- name the alternatives it considered;
- separate deterministic system responsibilities from model decisions;
- state security, durability, failure, and recovery semantics;
- list evidence that could falsify its recommendation; and
- avoid editing another Agent's position to manufacture agreement.

Use a filename such as `codex-agent-view.md` or
`<agent-name>-view.md`. A later synthesis may compare the positions, but it
must preserve disagreements and unresolved questions.

## 4. Contributions

| Contributor | Position | Status |
|---|---|---|
| Codex Agent | [Codex Agent view](codex-agent-view.md) | Initial position written and qualified after peer challenge |
| Distributed-Systems Agent | [Distributed-systems view](distributed-systems-agent-view.md) | Independent position written |
| Enterprise Patterns Agent | [Enterprise patterns view](enterprise-patterns-agent-view.md) | Independent position written |
| Contrarian Agent | [Red-team view](contrarian-agent-view.md) | Independent challenge written |

Future contributors should add one row when they add a position. The row is an
index, not an endorsement.

## 5. Decision Boundary

This roundtable may recommend changing the product language, service
boundaries, task relationships, result delivery path, Agent roles, or the
semantics of orchestration. It does not itself change the current decision.

Acceptance requires a later synthesis that:

1. states the chosen model and rejected alternatives;
2. identifies the migration from the current Task, TaskRun, Conversation, and
   worker contracts;
3. updates the roadmap priority;
4. replaces or revises the Portal execution design; and
5. deletes this proposal directory after its durable rationale has moved into
   the accepted design record.

## 6. Evidence Needed

The decision should not be made from topology diagrams alone. At minimum it
needs evidence for:

- foreground-versus-background routing accuracy;
- instruction fidelity from the user's source message to the run input;
- task and Agent selection accuracy across follow-up turns;
- end-to-end latency, token cost, and model-call count per useful outcome;
- user value from automatic completion summaries versus durable result cards;
- recovery after Server restart, worker loss, duplicate delivery, and a split
  multi-instance turn;
- recovery when a terminal Workflow step is committed but its advancement
  callback fails or the Server exits before creating the next step;
- prompt-injection resistance when a worker returns adversarial output;
- the frequency of tasks that genuinely need dynamic decomposition,
  fan-out/fan-in, replanning, or specialist Agents; and
- the amount of orchestration state that must survive outside model context.

## 7. Market-Derived Architecture Proposal

The Chinese-language proposal [从市场形态归纳 BuildMax Agent 架构](market-derived-architecture-conclusions.zh-CN.md)
retains the architecture conclusions derived from comparative product research.
The underlying vendor dossiers, funding and adoption data, and comparison
matrix are deliberately not stored in this repository: they are temporary
research inputs whose facts age independently of BuildMax's design.

## 8. Focused Runtime Notes

The Chinese-language note [Tier 1 运行环境与 Retained Task Thread：候选架构分析](tier-1-runtime-and-retained-task-thread.zh-CN.md)
examines whether the user-facing Agent needs its own full workspace and tool
runtime. It records the current `ContinueTask` recovery boundary and compares a
thick Tier 1 with a lightweight Tier 1 backed by a retained, multi-turn Task.

The note is a discussion artifact. Its workspace-retention model is not an
accepted design or roadmap commitment.
