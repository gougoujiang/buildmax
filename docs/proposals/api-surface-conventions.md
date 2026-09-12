# API Surface Conventions

> **简体中文：** [阅读中文镜像](../zh-CN/proposals/api-surface-conventions.md)
>
> **Audience:** contributors · **Status:** proposal — under discussion
>
> Opened: 2026-09-12

Related: [Worker API network boundary](../design/worker-api-network-boundary.md),
[Unified Artifacts](../design/unified-artifacts.md),
[Space secrets](../design/space-secrets.md),
[Entity identity](../design/entity-identity.md),
[current state](../current-state.md), and [ROADMAP.md](../ROADMAP.md).

## Contents

- [1. Problem And Current Context](#1-problem-and-current-context)
- [2. Goals And Non-Goals](#2-goals-and-non-goals)
- [3. Naming Conventions](#3-naming-conventions)
- [4. Decision: No URL Versioning Yet](#4-decision-no-url-versioning-yet)
- [5. Decision: Split OpenAPI Along The Listener Boundary](#5-decision-split-openapi-along-the-listener-boundary)
- [6. Options Considered](#6-options-considered)
- [7. Open Questions](#7-open-questions)
- [8. Likely Destination If Accepted](#8-likely-destination-if-accepted)

## 1. Problem And Current Context

The server exposes roughly 150 route registrations across two listeners. Their
composition is authoritative in each handler subpackage's `Register` method,
wired together in `internal/server/handlers/routes.go`, and mirrored exactly by
`internal/server/static/openapi.json`.

The surface grew feature by feature. Most of it is already consistent —
kebab-case path segments, plural collections, `{xxx_id}` path parameters — but
a few shapes were chosen locally and now disagree with each other. Because
BuildMax is Alpha with no frozen API contract and every client
(Portal, Desktop, CLI, workers) ships from this repository and deploys with the
server, this is the cheapest moment to fix the surface coherently rather than
carry the divergence forward.

Two structural questions came up while reviewing the surface and are settled
here because they shape every future route: whether to introduce URL versioning,
and whether to keep one OpenAPI document or split it.

The current inconsistencies worth a decision:

- **Top-level versus space-scoped ownership is mixed.** Some resources live
  under a Space and some at the `/api` root, sometimes in pairs:
  `/api/webhook-keys` alongside `/api/spaces/{space_id}/plugin-activations`;
  `/api/usage` alongside `/api/spaces/{space_id}/usage`; `/api/invitations`
  (invitations I received) alongside `/api/spaces/{space_id}/invitations`
  (invitations a Space issued). The top-level forms are "current subject's
  view" aggregates, but no written rule says so, so `webhook-keys` reads as
  ownerless.
- **Sub-resources are addressed two ways.** `task-runs` and `workflow-runs` are
  created and listed under a parent (`.../tasks/{task_id}/runs`) but read as a
  single entity by their own id (`.../task-runs/{task_run_id}`). This is
  reasonable — a run has a durable id and needs no parent to locate it — but it
  is not written down, so the next contributor picks one arbitrarily.
- **Auth routes have no common prefix.** `/api/otp/request`, `/api/login`,
  `/api/logout`, `/api/password`, and `/api/token/refresh` sit directly under
  `/api`; only refresh is grouped under `/api/token/`.
- **State transitions use two styles.** Most use an RPC-style POST action
  suffix (`.../cancel`, `.../retry`, `.../disable`, `.../enable`, `.../yank`,
  `.../archive`, `.../accept`, `.../restore`), while Space secrets use a state
  sub-resource (`PUT .../secrets/{secret_id}/state`). This is the surface's
  largest single style split.
- **Artifacts use two entry shapes on purpose.** Create and list are
  space-scoped (`/api/spaces/{space_id}/artifacts`); a single artifact is
  addressed by id without a Space in the path (`/api/artifacts/{artifact_id}`),
  because the record carries its own authorization. See
  [Unified Artifacts](../design/unified-artifacts.md). This is a designed
  exception, not drift, and should be preserved as the reference pattern for
  "take the Space from the record, not the path."

## 2. Goals And Non-Goals

Goals:

- State a small set of rules that decide, without further debate, where a new
  route lives and how it is named.
- Settle URL versioning and the OpenAPI document shape.
- Give a target for reconciling the divergent routes above, so the surface
  converges rather than accumulating a third style.

Non-goals:

- A public, externally supported API contract. There is no out-of-band
  consumer today; committing to one is a separate, evidence-driven decision.
- Rewriting handler internals, authorization, or storage. This is about the
  addressable surface and its documentation only.
- Changing the two-listener network boundary, which is already decided in
  [Worker API network boundary](../design/worker-api-network-boundary.md).

## 3. Naming Conventions

Proposed rules, most already followed:

1. **Path segments are kebab-case; collections are plural.** `task-runs`,
   `webhook-keys`, `audit-events`. Already consistent; make it a rule.
2. **Path parameters are `{resource_id}` for public identifiers** and a
   descriptive name (`{plugin_name}`, `{version}`, `{revision}`) where the
   segment is a natural key rather than a `NewPublicID`. See
   [Entity identity](../design/entity-identity.md).
3. **Ownership rule for top-level versus space-scoped.** A resource is
   space-scoped (`/api/spaces/{space_id}/...`) when it is managed within one
   Space. A top-level route (`/api/...`) is reserved for the acting subject's
   cross-Space view — "everything I can see" aggregates such as `/api/usage`
   and the invitations I received. `webhook-keys` must resolve to one of these
   two readings explicitly rather than staying ambiguous.
4. **Collection versus single-entity addressing.** Collection operations
   (create, list) hang off the parent path; reading or mutating one entity that
   owns a durable id uses the flat `.../{entity}-runs/{id}` form. Write this
   down so both `task-runs` and `workflow-runs` are covered by one rule.
5. **State transitions.** Choose one house style for the whole surface. The two
   candidates are an explicit state sub-resource (`PUT .../state`, as Space
   secrets already use) or an RPC-style action suffix (`POST .../cancel`). The
   recommendation is the state sub-resource where a transition is a simple
   enable/disable/activate toggle, and an action suffix only where the verb
   carries semantics a state field cannot (`retry`, `accept`). Whichever wins,
   secrets and tasks must stop disagreeing.
6. **Authorization by record where a resource is globally identified.**
   Artifacts are the reference pattern: a route addressed by artifact id takes
   the Space from the record, not the path.

`internal/tool/names.go` remains authoritative for LLM-facing tool names; these
conventions govern HTTP routes only.

## 4. Decision: No URL Versioning Yet

URL versioning (`/api/v1/...`) is a compatibility tool for a consumer you cannot
redeploy together with the server. That consumer does not exist: Portal,
Desktop, CLI, and workers all ship from this repository and deploy with the
server, and the N-1 rollback promise has been withdrawn, so there is no version
skew to bridge. Adding `/v1/` now would ship a version that never gets a
successor, because the surface can simply change in lockstep with its clients —
a concept with exactly one value, which Occam's razor rejects.

Introduce versioning only when there is evidence of an out-of-band consumer that
pins a version — a public API, a third-party integration, or a published SDK.
That is an evidence-driven decision, not a calendar one, and its likely shape is
a versioned public subset rather than a global `/v1/` prefix over the entire
internal surface.

The `info.version` field in `openapi.json` is spec metadata, not a URL version.
If nothing consumes it for compatibility decisions it is a value that will
drift; tie it to the application version or drop it.

## 5. Decision: Split OpenAPI Along The Listener Boundary

Today one `openapi.json` documents both listeners, including `/api/worker/*`.
This erases, in documentation, a boundary the code and the network deliberately
maintain: the public listener cannot dispatch a worker route, the two use
different authentication (user JWT versus run token), and they run on separate
sockets. See
[Worker API network boundary](../design/worker-api-network-boundary.md).

Split the specification along that existing boundary into a public document and
a worker document, each corresponding to one `Register*` method. This is not a
new concept — it follows an invariant already enforced in code — and it lets the
"spec matches routes exactly" check run per listener instead of reconciling one
large file against two route sets by hand.

Do not split finer. Admin, shared-artifact, and auth routes share the public
listener and one authentication scheme (or an explicit unauthenticated one);
separating them into more documents would multiply artifacts without a boundary
to justify it. Group those sub-audiences with OpenAPI `tags` inside the public
document.

## 6. Options Considered

- **Versioning: adopt `/api/v1/` now.** Rejected: no consumer needs it, and it
  freezes today's shape as "v1" during Alpha, contrary to the product
  principles.
- **Versioning: header- or content-negotiation-based.** Same objection — it
  solves a skew problem that does not exist yet — with more machinery than a
  URL prefix.
- **OpenAPI: keep one document.** Rejected: it documents the worker control
  plane to public-surface readers and makes the match-the-routes check reconcile
  one file against two disjoint route sets.
- **OpenAPI: one document per handler subpackage.** Rejected: subpackages are an
  internal decomposition, not an external boundary; the listener split is the
  boundary that matters to a reader and to network policy.

## 7. Open Questions

- Which state-transition style wins for the reconciliation in §3.5, and is it
  worth changing the already-shipped routes on the losing side during Alpha, or
  only holding new routes to the rule?
- Does `webhook-keys` belong under a Space or at the subject level? The answer
  determines whether it moves.
- Should the two OpenAPI documents live as separate committed files, or as one
  source generated into two views? Either satisfies the boundary; the choice is
  about how the match-the-routes check is wired.
- Is `openapi.json`'s `info.version` consumed by anything, or can it be dropped?

## 8. Likely Destination If Accepted

The naming rules and the two decisions become a short section in the server
architecture documentation
([`docs/contribute/architecture/server.md`](../contribute/architecture/server.md)),
which is where a contributor adding a route already looks. Each accepted route
change and the OpenAPI split become backlog tasks. This proposal is then
deleted; Git history keeps its rationale.
