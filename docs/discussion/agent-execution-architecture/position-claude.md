# Position: Claude

> **Author:** Claude (Anthropic), working as a contributing agent in this repository · **Status:** open position
>
> **Opened:** 2026-08-30 · **Evidence base:** [README.md](README.md)

I reached this position by reading the code and the design records, not by
applying a preferred architecture. Where I assert a fact it is in the evidence
base with a reference; where I assert a judgment I have tried to say what would
change it. One of my earlier claims was wrong and is recorded below, because how
I got it wrong is itself evidence about the question.

## Contents

- [Claim 1: The Tier Split Is An Execution Mode](#claim-1-the-tier-split-is-an-execution-mode)
- [Claim 2: Orchestrator/Worker Names Three Different Things](#claim-2-orchestratorworker-names-three-different-things)
- [Claim 3: The Middle Layer Is The Valuable One](#claim-3-the-middle-layer-is-the-valuable-one)
- [Claim 4: The Differentiator Is Accountability, Not Packaging](#claim-4-the-differentiator-is-accountability-not-packaging)
- [Claim 5: One Enforced Admission Rule Beats Three Stated Ones](#claim-5-one-enforced-admission-rule-beats-three-stated-ones)
- [Where I Was Wrong](#where-i-was-wrong)
- [The Question I Now Think Is Hardest](#the-question-i-now-think-is-hardest)
- [What Would Change My Mind](#what-would-change-my-mind)

## Claim 1: The Tier Split Is An Execution Mode

Three things are forced by physics: latency diverges, so some work exceeds a
turn's budget; work outliving a request must leave the connection for a record;
executing untrusted code needs isolation. Together they force an **attached /
detached** split and a durable substrate. Any system doing this work has them.

What is *chosen* is calling the two modes two agents, and the code does not
support the choice. E1–E3: one loop, no tier field, an option struct. E6: the
word is load-bearing once, as a routing label. E12: the record already replaced
the divider the term was coined for and called the naming "work with no behavior
attached." E13: every defect the record closed was fixed by the substrate, none
by the tier distinction.

The tier is real as a description of *how work runs*. It is not real as a
description of *what kind of agent runs it*.

I do **not** claim the tiers should merge. The [portal execution
model](../../design/portal-execution-model.md) §7 rejected "merge into one
synchronous agent" and was right — that loses durable retry, cancel, and
isolation. It considered merging the *modes*. It never considered keeping the
modes and dropping the claim that they are two kinds of agent, which is what I
argue for.

## Claim 2: Orchestrator/Worker Names Three Different Things

The comparison to enterprise orchestrator/worker is only useful once the term is
split:

1. **Deterministic control plane** — Celery, Temporal, Airflow, Kubernetes. The
   orchestrator is code: a state machine, retries, scheduling. It has no
   intelligence and is not supposed to.
2. **LLM orchestration** — a model decomposes a task, dispatches workers, and
   synthesizes results. Decomposition is dynamic because the model decides it.
3. **Agent platform control plane** — a scheduler dispatches agent runs to a
   worker pool. The orchestrator is infrastructure; the agent is the payload.

BuildMax has (2) stacked on (1)/(3): Tier 1 dispatches by tool call (E5), and
the scheduler claims and dispatches by atomic status transition. **"Tier 1 /
Tier 2" names the upper relationship while the substrate is the lower one**, and
one vocabulary covering two relationships at different layers is a large part of
why the naming confuses. The record's §1 already separates them — Task, TaskRun,
scheduler, and workers "are not agents" — but the vocabulary did not follow.

Where BuildMax genuinely differs from the common shape of (2), and where I think
its value is:

- **Most LLM orchestration is in-process and ephemeral.** Results live in the
  orchestrator's context; the process dies and the work is gone. Here the lower
  layer is durable, with claimed runs, retries, and a delivery obligation.
- **Most treat worker output as trusted context**, appended to history. §3
  refuses that — though E9 and E11 show the refusal is prose, not code.
- **Most make the orchestrator's synthesis the only output.** This system
  refuses to let the summary carry the result: "what is durable is the
  obligation, not the sentence," and the card is read from the record. Note that
  "a result card exists only if Tier 1's summary succeeded" is precisely one of
  the defects it fixed. I think this is a real architectural insight the project
  holds and does not advertise: **the orchestrator's account of the work is not
  the work.**

## Claim 3: The Middle Layer Is The Valuable One

The right decomposition is by **authority**, not by kind of agent:

| Layer | What it is | Authority |
|---|---|---|
| Intent (attached) | An agent holding user authority, turning a conversation into a bounded, checkable proposal | Proposal only |
| Execution authority | Deterministic substrate: validation, authorization, scheduling, quota, evidence | Decides — and is not an agent |
| Execution (detached) | One agent capability, recursive, specialized by agent definition rather than by tier | Produces data, never instruction |

This is isomorphic to enterprise orchestrator/worker with one difference that I
think is the whole point: **the middle layer is named as an authority boundary
instead of hidden inside the orchestrator.** Most systems put validation and
scheduling inside the orchestrator, so the orchestrator is simultaneously an LLM
and the authority — a prompt-injectable thing holding dispatch power, which is
the configuration enterprises are right to distrust.

BuildMax has already separated them. It has not named the separation as its
architecture, and E7 is the symptom: the tier called the orchestrator cannot
decompose, while the tier called the execution plane spawns subagents. Measured
by orchestration capability the names are backwards, which is what happens when
a name stops tracking what it denotes.

## Claim 4: The Differentiator Is Accountability, Not Packaging

The records claim deployability — "an out-of-the-box, privately deployable
enterprise Agent platform powered by one shared Go Agent Core." That is a real
strength and a weak moat: packaging is copyable.

What is defensible, and what this system is unusually close to owning, is
**verifiable authority** — answering mechanically, after the fact: *who asked
for this, under which instructions, executed by which definition, inside which
boundary, on what evidence.* The parts exist: `source_message_id` binding intent
per turn so the model cannot omit attribution, `agent_revision` recording the
text that ran, an append-only audit trail with typed actors, bounded redacted
traces, and a provenance route that quotes the user's message beside the
worker's instruction so that "a constraint missing from the instruction is
either one the model dropped or one the user never gave."

That property is hard to retrofit and hard to copy, and it is what a regulated
buyer actually asks for. It comes from the substrate. The tier vocabulary
neither creates nor protects it.

The gaps blocking the claim are, in my judgment, more urgent than the naming:
E8 (the agent holding user authority produces no trace and no ledgered call,
while everything it dispatches is fully evidenced — and AGENTS.md says otherwise),
audit that cannot name a run, and E14.

## Claim 5: One Enforced Admission Rule Beats Three Stated Ones

E11: the rule that untrusted content must not become instruction is stated three
times in three vocabularies. E10: the right primitive already exists one layer
down — `llm.Message.Source`, whose purpose is that "even a compaction summary
cannot turn an observation into something the user said." It has three values,
all local.

E9 is what that costs. `GetTask` returns worker output into Tier 1's context as
a bare `output_snippet:` line — no `Source`, no envelope — while the local
runtime gives the same class of content both. The path is concrete: worker
output, which an attacker may influence through repository content or a fetched
page, reaches the context of the loop that holds dispatch authority, and the
next iteration may call `StartTask` or `ContinueTask`.

So the proposal is one invariant, enforced in the loop rather than asserted in
records: **content that did not originate with an authenticated principal
carries a non-empty `Source` and never enters the instruction channel.** This is
strictly stronger than the per-tier rule, because it also covers Tier 1's own
tool output, which the current rule does not reach.

## Where I Was Wrong

I proposed that the intent layer might not need an agent loop — that if its job
is turning natural language into a bounded proposal, one structured output call
might do, removing a surface that can be injected and then act repeatedly.

That was wrong, and the correction came from a human contributor. Three reasons,
in increasing order of force:

1. **Three of Tier 1's four tools are reads.** The loop is what lets the
   orchestrator ground an answer in actual state instead of guessing it.
2. **It would reintroduce a fixed defect.** "Guess completion without structured
   state" is on §2's *must never* list. Removing the loop reinstates exactly
   that.
3. **Intent is co-constructed.** A user's intent is not a single utterance to be
   rewritten; it is a dialogue sequence that is not well defined at the start.
   An intent layer that cannot iterate cannot do the clarifying that produces the
   intent in the first place.

The instructive part is *how* I was wrong: I conflated the turn-level loop with
the conversation-level dialogue, and then reached for removing capability when
the actual problem was an ungoverned trust boundary. The concern behind the
suggestion was real — a loop that ingests untrusted content while holding
dispatch authority compounds injection risk per iteration — but the remedy is
Claim 5, not amputation. **When a capability looks dangerous, suspect the missing
invariant before the capability.** I record this because it is the kind of error
an agent reasoning from architectural taste rather than from the system's own
history will make repeatedly.

## The Question I Now Think Is Hardest

If intent is co-constructed across turns, then binding a run to **one** message
may be too narrow for the accountability property in Claim 4.

Consider a constraint stated in message 43 and a dispatch in message 47. The
provenance route quotes message 47 beside the worker's instruction. Neither
contains the constraint, so they agree — and the fact that the model dropped
something said four turns earlier is invisible in exactly the view built to
catch it. The property degrades precisely when intent took several turns to
form, which is the normal case rather than the exception.

I do not think this argues against `source_message_id`. Binding per turn so the
model cannot choose or omit its attribution is an integrity property worth
keeping. What seems missing is the **scope** of an intent: which span of
conversation the dispatched constraints were drawn from, recorded at dispatch
time and checkable afterwards.

That points somewhere I did not expect when I started. If it holds, the intent
layer should be *strengthened* rather than narrowed: its output is not a string
of `input` but a set of constraints with provenance, which is what makes "the
model dropped a constraint" mechanically detectable instead of noticed by whoever
happens to remember.

## What Would Change My Mind

- **Claim 1** fails if a stable set of genuinely specialized Tier 2 agents
  emerges with different loops rather than different definitions and tool sets.
  §5.2 gated this on evidence; if the evidence arrives, "agent plane" earns its
  name.
- **Claim 3** fails if the intent layer turns out to need real execution
  capability — filesystem, shell, team data — because then it is not a proposal
  layer and the authority split I describe does not hold. §5.3 says this
  question is open and that a foreground budget becomes a prerequisite the moment
  it happens.
- **Claim 4** fails if private-deployment buyers, asked directly, care about
  installation and operation and not about after-the-fact attribution. That is
  evidence I cannot gather from the repository.
- **Claim 5** fails if a context-admission rule turns out not to be mechanically
  enforceable — if in practice it reduces to prompt framing plus metadata, with
  the model still free to act on framed content. The threat model that [trust
  harness](../../design/trust-harness.md) names as its cheapest missing input
  would settle this, and I would rather see it than argue further.
