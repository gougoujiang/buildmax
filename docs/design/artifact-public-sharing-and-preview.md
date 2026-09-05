# Artifact Public Sharing And Preview

## Contents

- [Status](#status)
- [1. Decision](#1-decision)
- [2. Product Goal](#2-product-goal)
- [3. What This Reopens](#3-what-this-reopens)
- [4. Scope](#4-scope)
- [5. Domain Model](#5-domain-model)
- [6. Public URLs And Routes](#6-public-urls-and-routes)
- [7. Content Delivery And Preview](#7-content-delivery-and-preview)
- [8. Agent Tool Contract](#8-agent-tool-contract)
- [9. Configuration](#9-configuration)
- [10. Authorization, Governance, And Limits](#10-authorization-governance-and-limits)
- [11. Failure Semantics](#11-failure-semantics)
- [12. Delivery Phases](#12-delivery-phases)
- [13. Alternatives Rejected](#13-alternatives-rejected)
- [14. Open Questions](#14-open-questions)
- [15. Acceptance Criteria](#15-acceptance-criteria)

## Status

- roadmap_priority: `P2 follow-on` (reopens phase 3 of
  [unified-artifacts.md](./unified-artifacts.md))
- status: `implemented` (phases 1 and 2). Preview: Markdown and sandboxed-HTML
  rendering on the authenticated detail page and the public page, and the
  backend serving HTML under the sandbox CSP with a `?dl=1` override. Sharing:
  the `artifact_share` row and store, the sessionless `/shared/artifacts/{token}` routes,
  the authenticated `/api/artifacts/{id}/shares` management routes,
  `public_base_url` config, `UploadArtifact(share=true)` through both publisher
  adapters, the Portal create/revoke panel and public page, and share
  created/revoked audit. Two deviations from the text below: the share TTL bound
  ships as `storage.artifact_share_ttl_hours` in server.yaml with no env
  override (§9 named one); and share-**expired** audit is not emitted, because
  no share retention sweep exists yet to notice an expiry — expiry is enforced
  lazily at resolve time (§14 item 6). Phase 3 follow-ons stay open.
- supersedes: unified-artifacts.md §6.2 "MVP policy is authenticated team
  access only", §10 phase 3 "not planned", §12 question 4 "not planned", and
  §6.3's "HTML, SVG … download as attachments in the first slice" for the
  preview surface. Those records said a deployment that needed sharing would
  reopen the question rather than a design anticipating it; this is that
  reopening.
- follows: [unified-artifacts.md](./unified-artifacts.md),
  [worker-run-token.md](./worker-run-token.md),
  [surface-positioning.md](./surface-positioning.md)
- created_at: `2026-09-05`

## 1. Decision

Two capabilities are added on top of the unified artifact object, and they are
distinct:

1. **Preview.** Any artifact a team member can already read gains an in-Portal
   rendered view. Markdown renders as formatted text; HTML renders as a live
   page inside a sandboxed frame; images and text keep today's inline view;
   everything else keeps its download. This needs no new authorization — it is a
   better rendering of content the caller is already allowed to fetch.

2. **Public sharing.** An artifact can be given an explicit, revocable public
   link that a person with no BuildMax login can open. Opening it lands on a
   Portal page that previews the content and offers a download. This is opt-in
   per artifact, never a property of the canonical ID, and never on by default.

A public link is a **stored, revocable share token**, not a stateless signed
URL and not an object-store presigned URL. The token is high-entropy random
data; only its hash is stored, alongside creator, expiry, revoked time, and a
retrieval count. This is the model unified-artifacts.md §6.2 already specified;
what changes is that it moves from "not planned" to built. The reason it is
stored rather than a self-verifying HMAC/JWT is revocation: a stateless token
cannot be withdrawn before it expires without a server-side denylist, which is
the same defect §13 rejects presigned URLs for. Every other property a signed
URL would offer — public, expiring, no login, verified without a session — the
stored token also offers, plus central revocation and a usage record.

The externally reachable link is rendered **by the server**, from a new public
base URL it is configured with. A worker never learns or constructs the public
address; it does not have one and its own `worker.server_url` is deliberately a
cluster-internal listener. The server, which holds both the share record and
the public base URL, is the only place that can name the link, so it is the
place that does.

## 2. Product Goal

An agent writes a file for a person — most often a Markdown document, sometimes
an HTML prototype — and wants to hand that person one link that just works:

- opening it shows the content, rendered, without a download-and-open detour;
- for an HTML prototype, opening it shows the working page, not its source;
- the link can be sent to someone outside the team when that is the intent; and
- the owner can later revoke it, and see whether it was used.

The recipient needs no account and no knowledge of teams, artifacts, or storage
keys. The link is durable until it expires or is revoked.

## 3. What This Reopens

unified-artifacts.md closed external sharing deliberately, listing what a
reopening would first have to settle (§10 phase 3): approved roles, expiry
defaults, revocable tokens, anonymous access, malware scanning, audit
retention. This record answers them for the slice the product now needs:

- **Approved roles** — §10 here.
- **Expiry defaults** — §9 (`BUILDMAX_ARTIFACT_SHARE_TTL`, bounded default).
- **Revocable tokens** — §5, the `artifact_share` row and its `revoked_at`.
- **Anonymous access** — §6, the sessionless `/shared/artifacts/{token}` routes.
- **Malware scanning** — still out of scope and still an operator decision;
  §4.2. Sharing does not change what upload accepts.
- **Audit retention** — §10, share created / revoked / retrieved events on the
  existing metadata-only audit trail.

It does not open public-by-default hosting, cross-organization identity, or any
change to what a canonical artifact ID grants. Those stay out of scope.

## 4. Scope

### 4.1 In Scope

- An `artifact_share` record: one artifact, a hashed public token, creator,
  optional expiry, revoked time, retrieval count.
- Sessionless public API routes that resolve a share token to metadata and to
  content, refusing a revoked or expired token.
- A Portal public preview page reachable without login, rendering the shared
  artifact and offering a download.
- Markdown and HTML preview rendering, shared by the authenticated detail page
  and the public page.
- An `UploadArtifact` option for an agent to publish a file and receive a
  public link in one step, plus a distinct action to create or revoke a link
  for an existing artifact.
- A server-side public base URL configuration, and the plumbing that lets the
  server render the link into tool output and API responses.
- Authorization for who may create and revoke a share, and audit events for it.

### 4.2 Out Of Scope

- Public-by-default access: the canonical ID and its `/api/artifacts/{id}`
  routes keep requiring team membership. A share is a separate, additive grant.
- Cross-organization authenticated sharing (a named external user with an
  account). This slice is anonymous-link only.
- Malware scanning, DLP, or MIME restriction on upload — unchanged operator
  policy; a share exposes exactly the bytes upload already accepted.
- Password-protected links, per-recipient links, view analytics beyond a
  retrieval count, and watermarking.
- Editing a shared artifact: content stays immutable, so a corrected file is a
  new artifact and a new link.
- Server-rendered HTML preview (server turning Markdown into HTML). Rendering is
  client-side in the Portal; the server only serves bytes with safe headers.

## 5. Domain Model

### 5.1 `artifact_share`

A share is its own record, not a column on `artifact`. An artifact may have
more than one live link (a short-lived one and a long-lived one), each
independently revocable, and the artifact row stays immutable.

```go
type ArtifactShare struct {
	ID              uint       `json:"-"`
	ShareID         string     `json:"share_id"`     // opaque public handle for management
	ArtifactID      string     `json:"artifact_id"`  // the shared artifact's public handle
	TeamID          string     `json:"team_id"`      // denormalized for authz and listing
	TokenSHA256     string     `json:"-"`            // sha256 of the token; the token itself is never stored
	CreatedByType   string     `json:"created_by_type"`
	CreatedByID     string     `json:"created_by_id"`
	ExpiresAt       *time.Time `json:"expires_at,omitempty"`
	RevokedAt       *time.Time `json:"revoked_at,omitempty"`
	RetrievalCount  int64      `json:"retrieval_count"`
	LastRetrievedAt *time.Time `json:"last_retrieved_at,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
}
```

The token follows the codebase's existing opaque-credential pattern
(`login_code`, `user_refresh_token`, `workspace_webhook_key`): a prefix plus
`crypto/rand` bytes, hex-encoded, handed to the caller once and stored only as
its SHA-256. A prefix (e.g. `share_`) makes a leaked token greppable and lets
secret scanning recognise it. Resolving a link is a single indexed lookup on
`token_sha256`; there is no per-request signature verification because there is
no signature.

The `xxxShareRow` struct in `internal/infra/db` is the schema source of truth,
following [data-model.md](../contribute/architecture/data-model.md). It carries
a unique index on `token_sha256` and an index on `(team_id, created_at)` for
the management listing. `artifact_id` is stored as the internal `uint64`
foreign key (like `artifact.team_id`) and exposed as the public handle through
the read projection.

### 5.2 Relationship To The Artifact

A share points at a live artifact. Deleting (tombstoning) the artifact makes
every share resolve to the same 404 the canonical routes give — the share
lookup joins the artifact and applies its `DeletedAt`. Retention purging the
bytes is likewise reflected: a share to a purged artifact is gone, not a
dangling link to missing content. Revoking a share does not touch the artifact.

## 6. Public URLs And Routes

### 6.1 The Link Handed Out

A public link is human-facing, so it does not wear an `/api` prefix. The whole
public surface lives under one dedicated top-level namespace, `/shared/`, with
the resource type as its next segment — `/shared/artifacts/…` — so a future
shared issue or conversation is a sibling (`/shared/issues/…`) rather than a
special case, and the namespace never collides with the app's own `/artifacts`
resource routes should those ever become path-based:

```text
<public_base>/shared/artifacts/{token}          # the link the agent cites — opens the rendered preview
<public_base>/shared/artifacts/{token}/raw      # the bytes (?dl=1 forces a download)
```

`public_base` is the deployment's externally reachable origin (§9). Opening
`/shared/artifacts/{token}` lands on a rendered preview page, not a byte dump; the page's
download button and any embedded `<iframe>` point at `/shared/artifacts/{token}/raw`. The
token sits in the path, never the query string, so it does not leak through
`Referer` headers or query-string access logs the way `?token=` would.

The distinction between the two paths is what keeps the namespace routable when
the Portal and the API share one origin behind a reverse proxy — the reason the
app's own routes carry `/api` in the first place. The proxy sends the two
machine leaves to the backend and lets the SPA own the bare token:

```text
location ~ ^/shared/artifacts/[^/]+/(raw|meta)$  ->  API backend   # bytes and metadata
location /shared/                                 ->  Portal SPA    # the preview page
location /api/                           ->  API backend   # the authenticated app, unchanged
```

In a split-origin deployment (Portal on nginx, API on the Go server, as Compose
runs them today) the same paths resolve against their own origins and no extra
rule is needed; `public_base` is the origin that fronts them.

### 6.2 Public Routes — Sessionless, Token-Only

Three anonymous routes, authorized only by the token, no `access.Guard`:

```text
GET /shared/artifacts/{token}        # the preview page (Portal SPA; renders per §7)
GET /shared/artifacts/{token}/meta   # JSON metadata the page needs: filename, media type, size, title
GET /shared/artifacts/{token}/raw    # the bytes — inline-previewable per §7, or an attachment with ?dl=1
```

`/meta` and `/raw` resolve the token the way `worker.requireRunToken` resolves a
run token outside the guard: look up the hash, reject a missing, revoked, or
expired share with `404` (never `403`, and never distinguishing revoked from
never-existed — a token is not an existence oracle, mirroring the canonical
routes' non-enumeration rule). A valid token yields the artifact, whose content
is streamed by the same code path the authenticated content handler uses. The
page at `/shared/artifacts/{token}` is static SPA HTML and carries no secret itself; it
reads the token from its own URL and calls `/meta` and `/raw`.

### 6.3 Share Management — Authenticated

Creating, listing, and revoking a link is an app action performed by a
logged-in member or by an agent's run, not by an anonymous visitor, so it is
genuinely an API surface and keeps the `/api` prefix:

```text
POST   /api/artifacts/{artifact_id}/shares              # create a link (returns the token once)
GET    /api/artifacts/{artifact_id}/shares              # list this artifact's live links
DELETE /api/artifacts/{artifact_id}/shares/{share_id}   # revoke
```

These resolve the artifact, read its team, and require the caller's role per
§10 — the same `MemberOfResourceTeam` shape the canonical ID routes use, with a
role check layered on create/revoke.

## 7. Content Delivery And Preview

### 7.1 Rendering Matrix

Preview is an allowlist keyed on the stored, server-validated media type, never
a browser guess. It is shared by the authenticated `#/artifact/{id}` detail
page and the public `/shared/artifacts/{token}` page — one renderer, two entry points.
The public page reads its token from the path (a clean, non-hash URL served by
the SPA fallback), not from a `#` fragment.

| Media type | Preview | Mechanism |
|---|---|---|
| `text/markdown` | rendered Markdown | `react-markdown` + `remark-gfm`, already used in Portal; **no `rehype-raw`**, so embedded HTML is escaped, not executed |
| `text/plain` | text | existing `<pre>` |
| `image/*` (allowlisted) | image | existing `<img>` |
| `text/html` | live page | sandboxed `<iframe>` (§7.2) |
| everything else | download only | attachment |

Markdown is rendered in the Portal's own origin, which is safe precisely
because `react-markdown` does not execute embedded HTML or `javascript:` URLs
by default (its URL transform drops them). We do not add `rehype-raw` or
`dangerouslySetInnerHTML`. Syntax highlighting, if wanted, is a later additive
plugin and does not change the safety argument.

### 7.2 HTML Preview Safety

Serving agent- or user-authored HTML that runs scripts is the one genuinely
dangerous surface here, and unified-artifacts.md §6.3 forced HTML to download
for that reason. This record lifts that restriction for the preview surface
under a specific safety model, rather than in general.

The mechanism is an opaque-origin sandbox enforced two ways:

1. **Response header.** HTML content is served with
   `Content-Security-Policy: sandbox allow-scripts allow-popups allow-forms;`
   (no `allow-same-origin`). The CSP `sandbox` directive puts even a *directly
   navigated* HTML response into a unique opaque origin, so a script in a shared
   prototype cannot read cookies, `localStorage`, or make same-origin requests
   against BuildMax — whether it is framed or opened in its own tab. `nosniff`
   stays set.
2. **Frame attribute.** The Portal embeds the content in
   `<iframe sandbox="allow-scripts allow-popups allow-forms">` — again without
   `allow-same-origin`, which is what keeps the frame from reaching the Portal's
   own token storage. `allow-scripts` and `allow-same-origin` together would let
   the document remove its own sandbox, so they are never combined.

Defense in depth for operators who want it: because `public_base` can be a
distinct origin from both the API and the Portal, a deployment may serve shared
content from a dedicated content origin that hosts nothing else, the way large
providers isolate user content on a separate domain. The design does not
require a second origin, but nothing in it prevents one, and the config leaves
room to point content at one.

SVG keeps downloading. It is an active document like HTML but is far more often
embedded than viewed as a page, and an `<img>`-embedded SVG cannot get the
frame sandbox; treating it as previewable safely is a separate question left to
§14.

### 7.3 Download

Download uses the existing safe content disposition and both filename forms
(ASCII plus RFC 5987), from stored metadata. `?dl=1` on `/shared/artifacts/{token}/raw`
forces an attachment disposition even for a previewable type, so the public
page's download button and a direct link both get a save rather than a render.

## 8. Agent Tool Contract

### 8.1 Publishing With A Link

`UploadArtifact` gains one optional argument so the common agent flow — write a
document, hand the user a link — is a single call:

| Argument | Required | Rule |
|---|---|---|
| `path` | yes | unchanged |
| `title` | no | unchanged |
| `purpose` | no | unchanged |
| `share` | no | when true, the server also creates a public link and returns it; default false |

Default false keeps unified-artifacts.md §4.2's "not public by default": an
agent publishes privately unless it, or its configuration, opts in. When
`share` is true the tool result adds the public Portal link and its expiry:

```text
Published newton.md as artifact tuiowdqwjwwmyxcnvo3a (5123 bytes).
Public link (expires 2026-10-05): https://buildmax.example.com/shared/artifacts/share_9f...c1
Download: https://buildmax.example.com/shared/artifacts/share_9f...c1/raw?dl=1
Cite the public link in your final answer so the person can open it.
```

The publisher port (`tool.ArtifactPublisher`) grows to carry the `share`
intent and return the link; both adapters (`internal/interface/client` for a
logged-in local session, `internal/infra/workerclient` for a worker) pass it to
the server, and the server — the only holder of `public_base` — fills in the
URL. Neither client constructs the public address.

### 8.2 Sharing An Existing Artifact

A separate capability creates or revokes a link for an artifact that already
exists, for the case where the decision to share comes after the file does.
Whether this is a distinct runtime tool (`ShareArtifact`) or only a Portal
action is §14's open question; the API in §6.2 supports both and the Portal
action is in scope regardless.

## 9. Configuration

One new server-side value, declared in `internal/config/env_spec.go` (the
bootstrap env source of truth) and `ServerConfig`:

- **`BUILDMAX_PUBLIC_BASE_URL`** — the externally reachable base at which people
  open BuildMax (the Portal origin, or the shared origin in a single-origin
  reverse-proxy deployment). The server renders share links against it. When
  unset, share creation is refused with a clear error rather than emitting an
  unusable internal URL — the current bug this whole record exists to avoid is a
  link nobody can open, so the server does not emit one it cannot make public.

This is deliberately not derived from `worker.server_url` (a cluster-internal
listener, validated to *not* be the public port) nor from `BUILDMAX_SERVER_URL`
(the address a process uses to *reach* the server, not one it advertises).
`BUILDMAX_CORS_ORIGIN` already names the Portal browser origin for CORS; the
new value may equal it in a split-origin deployment but is a separate concern
(link rendering vs. request-origin allowance) and is stated separately.

A share TTL bound:

- **`storage.artifact_share_ttl_hours`** (server.yaml) — default and maximum
  lifetime of a share link, with a bounded default (30 days, matching the
  refresh-token horizon). A create request may ask for less, never more. It
  ships as a config-file value only, without the env override the earlier draft
  named: the public base URL is the value a deployment injects at runtime, and
  the TTL is a policy the operator writes once.

The per-file cap, team storage quota, and retention sweep are unchanged; a
share holds no bytes of its own.

## 10. Authorization, Governance, And Limits

Sharing is a team-authority action. The first-slice matrix extends
unified-artifacts.md §8 (which had share creation as "no initially"):

| Action | Member | Admin | Owner | Outside team | Anonymous with token |
|---|---:|---:|---:|---:|---:|
| Create share for an artifact | yes¹ | yes | yes | no | — |
| Revoke a share | own² | yes | yes | no | — |
| List an artifact's shares | yes | yes | yes | no | — |
| Open shared content | — | — | — | — | yes, until expiry/revoke |

¹ A member may share an artifact they can read. If a deployment needs sharing
restricted to admins/owners, that is a bounded tightening left to §14; the
permissive default matches the product goal of an agent handing its operator a
link. ² A member may revoke a share they created; an admin or owner may revoke
any.

An agent or worker creating a share through `UploadArtifact(share=true)` acts
as its run's identity. The run token already carries `UserID`/`TeamID`
(worker-run-token.md), so a worker-created share records the initiating user as
creator and the team as owner — the share is attributable, not anonymous at
creation. The agent creates a link; it cannot revoke or enumerate others.

Audit events, metadata-only, on the existing trail: **share created**, **share
revoked**, and **share expired** (recorded like artifact expiry, per share,
because it is a state change no one asked for at the moment it happens). A
retrieval increments the count and stamps `last_retrieved_at`; individual
retrievals are **not** audited per hit — that is analytics, and the trail is
for authority changes. Audit records never contain the token; §8's rule that
signed URLs and share tokens stay out of the trail is unchanged and now has a
concrete token to exclude.

Deletion and retention win over sharing: a tombstoned or purged artifact's
links resolve to 404 with no separate revocation step (§5.2).

## 11. Failure Semantics

- **Create with no `public_base` configured** → refuse with a meaningful error;
  emit no link. The tool reports it plainly so the model does not cite a URL
  that will not open.
- **Token resolves to a revoked/expired/deleted artifact** → 404, indistinct
  from a never-existed token.
- **Share creation fails after artifact upload succeeded** (in the combined
  `UploadArtifact(share=true)` path) → the artifact is still created and its ID
  returned; the tool reports the artifact plus a share-creation error, rather
  than failing the whole upload or claiming a link that does not exist. Upload
  and share are separate commits; a share failure never rolls back durable
  content.
- **Retrieval-count write fails** → the content is still served; the counter is
  best-effort telemetry, not a gate on delivery, and a failed increment is
  logged, not surfaced to the downloader.

## 12. Delivery Phases

### Phase 1 — Preview (no new authorization)

- Markdown rendering and the HTML sandbox frame in the shared Portal renderer,
  used by the existing authenticated `#/artifact/{id}` detail page.
- The backend change that serves `text/html` with the sandbox CSP under a new
  previewable category, distinct from the plain inline allowlist, plus the
  `?dl=1` override.
- Delivers requirement 2 (preview) for team members with no sharing machinery.

### Phase 2 — Public share links

- `artifact_share` row, store, and service; the sessionless `/shared/artifacts/{token}`
  routes and the authenticated `/api/artifacts/{id}/shares` management routes.
- `BUILDMAX_PUBLIC_BASE_URL` and `BUILDMAX_ARTIFACT_SHARE_TTL` config and the
  server-side link rendering.
- The Portal public `/shared/artifacts/{token}` page, reusing the Phase 1 renderer.
- `UploadArtifact(share=true)` through both publisher adapters, and the Portal
  create/revoke action.
- Audit events and the authorization matrix.
- Delivers requirement 1 (public download) and public preview.

### Phase 3 — Follow-ons (not committed)

- A distinct `ShareArtifact` runtime tool if agents need to share after the
  fact (§14).
- Admin-only share restriction as a team policy, if a deployment asks.
- Safe SVG preview, syntax highlighting, and a dedicated content origin as a
  first-class configuration.

## 13. Alternatives Rejected

### Stateless signed (HMAC/JWT) download token

Mint an HS256 token over `artifact_id` + expiry with the existing `JWTSecret`,
verify it in a sessionless branch with no database row — the lightest possible
build, reusing `authtoken`'s pattern exactly. Rejected as the primary mechanism
because a stateless token cannot be revoked before it expires without a
server-side denylist, which reintroduces the storage a stored token already is,
and cannot carry a retrieval count or a per-link creator without more claims
than a URL should hold. It is the right tool when revocation is genuinely not
needed; sharing a file with someone outside the team is exactly where "I sent
that to the wrong person, kill the link" must work. The stored token costs one
indexed lookup and buys revocation, listing, and audit.

### Object-store presigned URL

Rejected for the same reasons unified-artifacts.md §13 gives: it bypasses
BuildMax authorization, cannot be centrally revoked, couples a saved link to
object-store configuration, and varies by provider. The server streaming bytes
behind its own token keeps the contract portable across local FS and S3/MinIO.

### Public-by-default artifact IDs

Make `/api/artifacts/{id}` readable by anyone holding the ID. Rejected: the
canonical ID is an identifier, not a credential (§6 non-enumeration), and
unified-artifacts.md §4.2 rules out a public-by-default host. Sharing must be an
explicit, revocable, additive act.

### Same-origin HTML preview

Render HTML in the Portal or API origin (an iframe with `allow-same-origin`, or
direct `dangerouslySetInnerHTML`). Rejected: a shared prototype can contain
arbitrary script, and same-origin execution gives it the Portal's token storage
and the API's cookie-less-but-still-real surface. The opaque-origin sandbox
(§7.2) is the whole reason HTML can be previewed at all.

### Worker constructs the public link

Let the worker render the share URL, as it renders today's internal one.
Rejected: the worker has no public base URL and, by network-boundary design,
should not — its only server address is a cluster-internal listener. The server
holds the public base URL and the share record; it names the link.

## 14. Open Questions

1. **Share-after-the-fact tool.** Is a distinct `ShareArtifact` runtime tool
   warranted, or is the Portal create action plus `UploadArtifact(share=true)`
   enough? Adding a tool costs model surface; defer until an agent flow needs to
   share a file it did not just upload.
2. **Who may create a share.** The matrix defaults to any member. Some
   deployments will want admin/owner-only. Left as a bounded team policy rather
   than hardcoded, pending a request.
3. **Safe SVG preview.** SVG is an active document; whether it can be previewed
   under the same sandbox model or must keep downloading needs its own look.
4. **Retrieval accounting granularity.** A single counter is proposed. Whether
   distinct preview vs. download counts, or a capped per-link download limit,
   are worth the columns is unproven; start with the count.
5. **Link privacy in shared caches.** Shared HTML content is cacheable by
   construction; the exact `Cache-Control` for shared vs. private content, and
   whether a revoked link must defeat an already-cached copy, needs stating in
   the content handler. Shipped as `private, no-store` for now, the same as the
   authenticated route, so revocation takes effect immediately at the cost of
   no shared caching.
6. **Share expiry sweep and its audit.** Expiry is enforced lazily: a resolve
   past `expires_at` returns 404, but nothing records the expiry, so the audit
   trail carries share created and revoked but not expired. An `ArtifactRetainer`-
   style sweep that tombstones lapsed shares and audits each — the way artifact
   expiry is recorded — is the follow-on that closes it.

## 15. Acceptance Criteria

The increment is complete when:

- a team member can open any readable artifact's Portal detail page and see
  Markdown rendered and an HTML artifact running in a sandboxed frame, with
  non-previewable types still offering download;
- an agent can call `UploadArtifact(share=true)` and receive a public Portal
  link plus a raw download link in its tool result, built from the configured
  public base URL;
- a person with no BuildMax login can open the public link, see the same
  rendered preview, and download the file;
- a shared HTML prototype's scripts run in an opaque origin and cannot reach the
  Portal's stored session or the BuildMax API as an authenticated caller;
- creating a share with no public base URL configured is refused with a clear
  error and emits no link;
- an owner, admin, or the creating member can revoke a link, after which the
  link resolves to the same 404 a never-existed token gives;
- deleting or purging the artifact makes every link to it resolve to 404 with
  no separate revocation;
- the token appears in no API response after creation, no tool output beyond
  the one-time link, no trace, and no audit event; and
- share created and revoked are on the audit trail; individual retrievals are
  not; expired is deferred to the share retention sweep (§14 item 6).
