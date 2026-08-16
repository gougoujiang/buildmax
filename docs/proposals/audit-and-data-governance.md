# Audit And Data Governance

> **Audience:** contributors, operators, and security reviewers · **Status:** proposal — under discussion

Related: [../ROADMAP.md](../ROADMAP.md) P4, [team governance design](../design/team-governance.md), [durable run trace design](../design/durable-run-trace.md), and [configuration reference](../reference/configuration.md).

## Problem

BuildMax records bounded run traces and has team-scoped resources, roles, model
usage, and task artifacts. It does not yet provide a coherent operator answer
to: who changed a sensitive setting, which policy governed a run, what data a
worker handled, how long evidence is retained, or how a team exports and
deletes its records.

Run diagnostics and governance audit are related but different. A trace helps
debug one execution; an audit record proves that a meaningful action occurred
without retaining prompts or tool output unnecessarily.

## Goals

- Define the minimum append-only event evidence required for shared team work.
- Separate diagnostic traces, audit events, workspace/artifact data, and model
  usage so each can have an appropriate retention policy.
- Make sensitive actions attributable to a user, service identity, or worker.
- Identify a practical first slice for export, deletion, and retention without
  claiming full compliance certification.

## Non-Goals

- A complete SIEM, legal-hold, e-discovery, or compliance-reporting product.
- Persisting raw prompts, generated text, shell output, or credentials in every
  audit event.
- Immutable WORM storage or customer-managed encryption keys in the first
  slice.
- Replacing database and object-store backup responsibilities.

## Options To Evaluate

| Option | Strength | Main concern |
|---|---|---|
| Extend run traces only | Reuses existing infrastructure | Does not cover identity, configuration, or lifecycle changes consistently |
| Add a small team event ledger | Matches the current P4 governance direction | Needs clear event taxonomy and write ownership |
| Export every event directly to a SIEM | Fits mature enterprises | Adds integration and delivery guarantees before the event model is stable |
| Define internal events first, then export | Produces a testable canonical record | Requires retention and access decisions before external integrations |

The likely direction is a small internal team event ledger, correlated with
run traces and managed model-call records, followed by carefully scoped export
once the canonical event shape is stable.

## Questions To Resolve

- Which actions are security or governance significant enough to be events in
  the first slice?
- Who may view event metadata, traces, artifacts, and model usage within a
  team?
- What correlation identifiers may connect a task, worker, model call, policy
  version, and artifact without duplicating sensitive contents?
- Which retention and deletion controls are configuration versus operational
  responsibility?
- Does export need at-least-once delivery, and how will consumers detect gaps?

## Evidence Needed For A Decision

- A concrete incident investigation showing the minimum events and trace links
  an operator needed.
- A privacy review of retained metadata and redaction boundaries.
- Cross-team authorization tests for event and trace reads.
- A retention and deletion exercise using a representative task and artifact.

## Likely Destination If Accepted

The first event model belongs in the P4 team-governance design, while retention
and export decisions may become a subsequent data-governance plan. User-facing
controls and operator procedures must be documented alongside implementation.
