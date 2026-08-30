# Discussions

> **Audience:** contributors and the AI agents that work in this repository · **Status:** current

A discussion is a topic where more than one participant records a **position**,
and the positions are meant to disagree. It exists because some architectural
questions are not settled by finding the answer in the code — the code is the
evidence, not the verdict — and because this repository is worked on by several
AI agents whose reasoning should be comparable rather than averaged.

## How This Differs From The Other Directories

| Artifact | Purpose |
|---|---|
| [../design/](../design/README.md) | Accepted rationale. A decision has been made |
| [../proposals/](../proposals/README.md) | One paper argues for one direction, seeking acceptance |
| This directory | Several papers argue against each other, seeking a better question |

A proposal advocates. A discussion compares. When a discussion converges, it
becomes a proposal or a design record and the discussion is deleted; git history
keeps it.

## The Shape Of A Topic

Each topic is a directory holding:

- `README.md` — the question, an **evidence base** of independently verifiable
  facts, the open questions, and an index of positions.
- `position-<participant>.md` — one participant's position. The filename names
  the author, because who reasoned to a conclusion is part of reading it.

The evidence base is the load-bearing part. It exists so that participants
disagree about **judgment** rather than about facts, and so that a newcomer does
not have to re-derive the same call graph a fourth time. Every entry in it
carries a file reference and is checkable in one command.

## Participation

Anyone may add a position, including a human contributor.

1. **Read the topic's evidence base first, and verify at least the facts your
   position depends on.** Do not trust it because it is written down — that is
   the failure mode this directory is built to avoid.
2. **Correct the evidence base if it is wrong.** A wrong fact is a defect in the
   topic, not a point scored against another position. Fix it in place and say
   so in your position.
3. **Add `position-<yourname>.md`.** State your claims so they can be falsified.
   Say what evidence would change your mind. If you disagree with an existing
   position, name it and say precisely where the reasoning parts.
4. **Record where you were wrong.** A position that has changed is more useful
   than one that never moved, and the reasoning that produced an error is
   evidence about the question.
5. **Do not edit another participant's position.** Reply in your own.

Agreement is not the goal and consensus is not required. A topic that ends with
two well-argued incompatible positions and a sharper question has succeeded.

## Open Topics

| Topic | Question |
|---|---|
| [Agent execution architecture](agent-execution-architecture/README.md) | Is BuildMax's two-tier model an agent architecture or an execution mode, and what is the most correct shape for a privately deployed agent platform? |
