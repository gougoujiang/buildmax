# Unified Artifacts

## Status

- roadmap_priority: `P2 follow-on`
- status: `implemented` (§10 phases 1 and 2: the artifact object, storage, API,
  and Portal listing; `UploadArtifact` on every surface with a server, and
  artifacts on issue result cards. Registering a run's output directory and
  phase 3 external sharing are both decided against — §12 questions 6 and 4.
  Phase 4 follow-ons stay open)
- follows: [surface-positioning.md](./surface-positioning.md) and
  [team-governance.md](./team-governance.md)
- roadmap: [../ROADMAP.md](../ROADMAP.md)
- created_at: `2026-08-22`

## 1. Decision

An artifact is a first-class BuildMax object, not a by-product of a task run.
The system provides one artifact capability, and anything that produces a file
a user should keep — an agent, a background run, a direct upload — uses that
capability. A task run that wants to preserve its output calls the artifact
service like any other producer. A run therefore keeps its reproducible output
directory, while the files it produced can be found, referenced, and shared
without knowing its task or run ID.

This is what separates the model from the `artifact`/`artifact_item` tables
that migration `0001_artifact_tables_to_task_run_artifact` removed. Those were
a child structure of a task run: no run, no artifact, and the only way to name
one was the run it hung off. The object described here owns its identity and
lifetime, and records its producer as provenance rather than as a parent.

The artifact's identity is its `ar_...` ID. That ID is what the service
returns, what a tool reports, and what a user cites. A URL is one surface's
rendering of that ID, not the identity itself.

Artifacts are a server capability. They live in a Team namespace, which is the
existing authorization boundary; a user working alone is represented by their
personal Team and need not see the Team concept in the UI. CLI and Desktop
reach artifacts by being logged in to a BuildMax server — the private
deployment case this product is built for. A local session running straight
against a model provider, with no server, has no artifact capability at all,
and no artifact tool appears in its tool list.

The shared Agent runtime exposes an `UploadArtifact` tool only where that
capability is present. It accepts an explicitly named local file, streams it
through the artifact service, records an Artifact, and returns the artifact's
ID and canonical URL.

The object store remains an implementation detail. A BuildMax artifact URL is
not a bucket key and is not an object-store presigned URL.

## 2. Product Goal

An agent, a user, or a background run should be able to make a useful file
available to the team and give the recipient one durable reference. The
recipient should be able to:

- preview or download it when they have team access;
- attach it to an issue, conversation, or result without copying storage keys;
- keep that reference in a document or message and have it still resolve; and
- intentionally create a revocable external sharing link when access outside
  the team is required.

The product language is **artifact**, not bucket, object, result directory, or
worker output path.

## 3. Current Baseline

BuildMax already has most storage primitives, but not the product object:

| Capability | Current behavior | Limit |
|---|---|---|
| Object storage | `internal/infra/objectstore` supports local filesystem and S3-compatible storage, including MinIO. | Keys are infrastructure details. |
| Team files | Portal accepts uploads into a mutable team file tree. | A file has no durable identity, provenance, immutable version, or share model. |
| Task-run artifacts | A worker archives `artifacts/` and records `task_run_artifact` paths. | A file is addressed through its task run and relative path; it cannot be independently referenced. |
| Artifact viewing | Team members can retrieve run output through team-authenticated task-run routes. | It is text/Markdown-oriented and not a general file-preview or download contract. |
| IDs | `ar_` and `f_` prefixes were reserved in `internal/util/id.go`. | They named a resource that was removed; see below. Type prefixes are gone entirely now — see [entity-identity.md](entity-identity.md). |

`task_run_artifact` is intentionally a set of paths, not a durable resource:
it has no public handle or timestamps. The new model must not reinterpret
that table as if it already provided the required contract.

An `artifact`/`artifact_item` table pair existed before the first public
release. Migration `0001_artifact_tables_to_task_run_artifact` in
`internal/infra/db/migration.go` removed it: it copies `artifact_item` into
`task_run_artifact`, drops both tables, and drops the `last_artifact_id` and
`artifact_seq` columns from `task` and `chat`. That model was a run's child
structure, which is why collapsing it into a path index lost nothing worth
keeping. Reusing the `artifact` table name is safe on any deployment that has
applied migration 0001, and the table this design creates is a different
object for the reason section 1 gives.

## 4. Scope

### 4.1 In Scope

- An Artifact in a Team namespace — a personal workspace being the user's
  personal Team — with a stable `ar_...` ID as its canonical reference.
- One immutable file per Artifact in the first slice.
- Server-mediated upload, metadata lookup, preview, and download.
- `UploadArtifact` as a normal shared Agent tool, registered only on a surface
  that has an authenticated artifact service.
- Provenance for agent uploads, task-run outputs, and direct user uploads.
- Stable authenticated URLs and separate revocable external share links.
- Portal artifact cards, list/detail view, and safe lightweight previews.
- Migration of newly produced worker output files into the unified model while
  retaining the existing task-run artifact listing for compatibility.
- Team authorization, deletion/tombstoning, retention hooks, quotas, and audit
  events at the artifact boundary.

### 4.2 Out Of Scope

- A local artifact store for a CLI or Desktop session with no server login.
  Such a session keeps its normal local output behavior and has no artifact
  tool; see section 7.1.
- A general editable drive or synchronized local folder.
- Replacing the mutable team `home/` file space.
- A public-by-default file host.
- Multi-file archives, folders, version history, deduplication, or client-side
  encryption in the first slice.
- Direct browser-to-S3 uploads before the server upload contract and controls
  have proven adequate.
- A full document editor, arbitrary file conversion service, or malware
  scanner implementation. The API must leave room for these controls.
- Cross-team artifact ownership or copying artifacts between deployments.

## 5. Domain Model

### 5.1 Artifact

The Artifact model has one immutable content object per artifact. `TeamID` is
required because Team is the authorization namespace, including for a user
working alone in their personal Team:

```go
type Artifact struct {
	ID              uint   `json:"-"`
	ArtifactID      string `json:"artifact_id"` // ar_...
	TeamID          string `json:"team_id"`
	Filename        string `json:"filename"`
	MediaType       string `json:"media_type"`
	SizeBytes       int64  `json:"size_bytes"`
	SHA256          string `json:"sha256"`
	StorageKey      string `json:"-"`
	CreatedByType   string `json:"created_by_type"`
	CreatedByID     string `json:"created_by_id"`
	SourceType      string `json:"source_type"`
	SourceID        string `json:"source_id,omitempty"`
	Title           string `json:"title,omitempty"`
	DeletedAt       *int64 `json:"deleted_at,omitempty"`
	ExpiresAt       *int64 `json:"expires_at,omitempty"`
	CreatedAt       int64  `json:"created_at"`
}
```

There is one artifact store. A logged-in CLI or Desktop session creates the
same Artifact, in the same Team namespace, through the same service Portal
uses; the surfaces differ only in how they render the result. Should a
local-only store ever be added, it must issue real `ar_...` IDs from the start,
so that the reference a tool returns means the same thing on every surface.

`StorageKey` is private and generated by the storage adapter. No API, tool
output, trace, or UI exposes it. `SHA256` is calculated while streaming the
upload and proves what was stored; it is not a cross-team deduplication key.

Initial source types are:

| Source type | Source ID | Meaning |
|---|---|---|
| `agent` | agent run or session ID when available | An agent invoked `UploadArtifact`. |
| `task_run` | task run ID | The worker materialized the file as run output. |
| `user_upload` | optional request/audit correlation ID | A member uploaded a file directly. |
| `system` | optional operation ID | BuildMax generated the file without an agent call. |

`created_by_type` distinguishes a user, agent, worker, or system actor; it
does not invent a user ID for automated work. This follows the existing audit
model's actor rule.

An artifact does not contain a free-form metadata JSON column. The fields
above are deliberately queryable and bounded. Add a concrete field only when a
product behavior needs it; arbitrary JSON is an easy place for content,
prompts, and credentials to leak into durable metadata.

### 5.2 Links To Work

An Artifact may be linked to a task run, issue, conversation, or result card.
In the first slice, `source_type` and `source_id` capture the producing
operation, and presentation derives the relevant task/issue/conversation from
that source. A separate many-to-many attachment table is deferred until one
artifact must be attached to independent work objects.

`source_id` holds the task **run**, not the task. The run is the operation that
produced the file, and it is what distinguishes one attempt from another; the
task, issue, and conversation are all reachable from it. A reader going the
other way — an issue asking what its work produced — has to gather every run of
every task, not each task's last one, or a retried task hides what its earlier
attempts published.

This avoids making an uploaded file depend on a task run merely because the
first producer was a worker.

### 5.3 Task-Run Output Is Not Artifacts

`task_run_artifact(task_run_id, relative_path)` stays the run's index of what it
left in its own output directory, and nothing there becomes an Artifact
automatically.

This reverses an earlier draft of this section, which had each uploaded file
create an Artifact on terminal run processing. Two things were wrong with it.
The copy is real: the compatibility route reads the run-output key space, so
registering the same files in the artifact key space stores every output twice.
And the harvesting is the one section 11 rejects — an agent's output directory
is exactly the place `.env` files, caches, and intermediate work end up, and
scanning it is no safer for being done by the server.

The two are different objects with different jobs. A run's output directory is
the reproducible record of what that run produced; an artifact is a file
someone is meant to keep. An agent turns the first into the second by choosing,
once, with `UploadArtifact`.

Nothing changes an old task-run path into an `ar_` ID, and old output is not
backfilled. The task-run route keeps its current response for the runs that
have one.

## 6. Access And URLs

### 6.1 Identity And Routes

The canonical reference is the artifact's public handle. It is unique on its
own — 96 bits of crypto-random data — so locating an artifact needs nothing
else. Team is an authorization fact the
record carries, not part of the address.

Two route shapes follow from that split:

```text
GET /api/artifacts/{artifact_id}          # detail
GET /api/artifacts/{artifact_id}/content  # content
GET /api/teams/{team_id}/artifacts        # the team's listing
```

The ID-addressed routes resolve the artifact, read its `TeamID`, and require
the caller to be a member of that team. This is a new authorization path. It
cannot reuse `access.Guard.UserAndPathTeam`, which takes the team from the
request path, so it needs its own guard method and its own entry in the team
authorization matrix test.

A caller who is not a member gets `404`, never `403`. The opaque
`artifact_id` is an identifier and not a credential, and no response may turn
it into an existence oracle — that is what section 13's non-enumeration
criterion means in practice.

The team-scoped route is the listing and team-view surface. It is not a second
address for one artifact.

The Portal may provide a human-facing detail route in addition to these API
routes. It must resolve the same Artifact and enforce the same authorization.

### 6.2 External Share Links

External sharing is a separate capability, not a flag on the canonical URL.
Creating a link generates a high-entropy, non-guessable share token and a URL
such as:

```text
/shared/artifacts/{share_token}
```

A share link identifies one artifact and has at least:

- creator and creation time;
- optional expiry, with a bounded deployment default;
- revoked time; and
- optional download count for audit and future limits.

MVP policy is **authenticated team access only**. Public sharing is added only
when the authorization matrix specifies who may create it (expected: owner or
admin), how a recipient sees the data policy, how it is revoked, and which
audit events are written. Do not implement permanent S3 presigned URLs as a
shortcut: they bypass BuildMax authorization, cannot be centrally revoked, and
couple saved links to object-store configuration.

### 6.3 Content Delivery

All content requests authorize or validate the share token before retrieving
the object. The server may stream content itself or, after authorization,
redirect to a short-lived object-store URL. The latter is an optimization, not
the public contract.

Content headers use stored, server-validated metadata; never trust an uploaded
filename or caller-supplied MIME type alone. Download uses a safe content
disposition and filename. Inline previews are an allowlist, not a browser
guess: plain text, Markdown rendered through the existing sanitizer, and safe
image/PDF types may be added deliberately. HTML, SVG, executable content, and
unknown types download as attachments in the first slice.

## 7. Agent Tool Contract

### 7.1 Availability

`UploadArtifact` is a shared runtime tool, assembled in `internal/agentapp`
alongside other default tools. It is registered only where the surface has an
authenticated artifact service: a server deployment's own runtime, or a CLI or
Desktop session logged in to a BuildMax server. `internal/interface/auth`
already answers that question for the local surfaces — `IsLoggedIn` and
`CanAuthenticate(serverURL)` — so it is a precondition of tool assembly, not
something discovered at call time.

Where there is no such service, the tool is **absent from the tool list**. It
is not registered in a state where every call fails. A tool that exists only to
answer "unavailable" costs a round trip and teaches the model nothing, while a
tool that is not there is a fact the model can act on immediately.

The tool is never emulated by returning a source-file path. A path on one
machine is not a shareable object, and a session with no server keeps its
normal local output behavior instead.

### 7.2 Invocation

The agent supplies:

| Argument | Required | Rule |
|---|---|---|
| `path` | yes | A regular readable file within the effective workspace or allowed runtime output directory. |
| `title` | no | A short user-facing label; filename remains the content name. |
| `purpose` | no | A bounded human-readable note for the result card, not arbitrary file metadata. |

The tool must reject directories, device files, symlinks that escape the
allowed root, files over the active quota, and paths it cannot safely open. It
streams the file, creates the Artifact only after storage succeeds, and returns
the Artifact ID, filename, size, and canonical URL. A storage failure returns a
meaningful tool error and leaves no successful artifact record.

The tool does not auto-upload every file an agent writes. The model must choose
the final file it intends to present. This makes the output understandable,
avoids accidental publication of `.env` files or caches, and keeps storage
costs bounded.

### 7.3 Prompting And Results

The runtime prompt tells agents to use `UploadArtifact` for a file that should
be delivered, retained, or shared, and to cite the returned `ar_...` reference
in their final answer. An Artifact result card should include title, filename,
producing run or agent, creation time, preview/download action, and the
canonical link.

The LLM-facing name follows `internal/tool/names.go`; when implementation
adds the constant, `docs/guide/tools.md` becomes the user-facing source of
truth for its arguments and availability.

## 8. Authorization, Governance, And Limits

Artifacts are team resources. Team membership is the baseline read boundary,
and the user's personal Team is the private single-user case. An artifact never
derives authorization from a run URL alone. The detailed matrix
is part of implementation, but the proposed first-slice policy is:

| Action | Member | Admin | Owner | Outside team |
|---|---:|---:|---:|---:|
| Read/download team artifact | yes | yes | yes | no |
| Upload through an authorized agent/user flow | yes | yes | yes | no |
| Delete own direct artifact | yes | yes | yes | no |
| Delete any team artifact | no | yes | yes | no |
| Create/revoke external share link | no initially | future | future | no |

Deletion is a tombstone first: it immediately hides metadata and blocks
content access, records an audit event, then schedules physical object removal
under retention policy. It does not rewrite task-run history. A run page can
say its former output has been removed without leaking a storage key.

The service enforces per-file size and team storage quota before accepting a
file, and counts the final stored bytes. Quota configuration, retention
duration, permitted MIME categories, and virus-scanning integration remain
operator policy decisions; the Artifact service exposes the required decision
points but does not invent configuration fields before they exist.

At minimum, audit these metadata-only actions: artifact created, artifact
deleted, share created, and share revoked. Audit records must not contain file
contents, signed URLs, share tokens, or a user-provided description that has
not been bounded and reviewed.

## 9. Storage And Failure Semantics

The storage key is private, generated by the adapter, and unrelated to any
URL. A representative shape for a server deployment is
`teams/{team_id}/artifacts/{artifact_id}/content`, which makes object ownership
clear while letting bucket layout evolve behind the adapter. Its resemblance to
an API route is a naming coincidence and not a contract: nothing outside the
adapter may parse, construct, or depend on a key.

Existing task-run outputs are keyed by the creating user rather than the team.
`objectstore.RunOutputFileKey` produces
`<prefix>/<created_by>/artifacts/<conversation>/<task>/<run>/<path>`. New runs
write through the artifact service into its key space instead of that one.
Objects written before this design keep their old keys, stay reachable only
through the compatibility route, and are not copied.

The interface `internal/infra/objectstore` called `ArtifactStorage` was
run-output storage — its methods take `RunRef` and `RunObjectRef`. It is now
`RunOutputStorage`, matching the `cfg.RunOutputs` and `db/run_output.go` naming
already in use, and `ArtifactStorage` names the native capability. Two
interfaces sharing that name would have been the easiest way for a later change
to quietly re-invert the dependency direction this design exists to fix.

Upload is write-once:

1. authorize and validate the source stream;
2. reserve an Artifact ID and generated storage key;
3. stream to a temporary or final object while measuring size and SHA-256;
4. commit the immutable Artifact metadata only after the object is durable;
5. remove any uncommitted object on failure, with a reconciler for crashes
   between storage and database work.

No content update endpoint exists. A changed file creates a new artifact and
may later be linked to its predecessor if product requirements demand version
lineage.

The storage interface remains portable across the server's local-filesystem
backend and S3/MinIO. That is a choice about where a deployment's object store
lives; it says nothing about whether a CLI or Desktop session is local, which
section 7.1 settles on its own terms. Server-side behavior, not
storage-provider URL behavior, defines the product contract.

## 10. Delivery Phases

### Phase 1 — Foundations

- Add core Artifact model, store, object-storage contract, and stable remote
  Team-scoped read/download APIs; expose a personal Team as a personal
  workspace in product UI.
- Add migrations, identifier generation, authorization tests, quota checks,
  and redacted audit events.
- Provide Portal listing/detail with download and narrow safe previews.
- Keep public sharing disabled.

### Phase 2 — Agent And Worker Producers

- Add `UploadArtifact` to the shared Agent runtime and document it in the tool
  guide.
- Enable it for Portal task runs and for logged-in CLI and Desktop sessions,
  and leave it unregistered on a session with no server.
- Show deliberately published artifacts in conversation and issue result cards.
  A run's output directory is not registered — see §12 question 6.
- Retain task-run artifact route compatibility and migrate Portal consumers.

### Phase 3 — Intentional External Sharing — not planned

Sharing an artifact outside the deployment is not built and is not queued. An
agent publishing a file with `UploadArtifact` and an authorized team member
fetching it covers the workflow this design exists for. Everything the phase
would have to settle first — approved roles, expiry defaults, revocable tokens,
anonymous access, malware scanning, audit retention — is cost paid ahead of
anyone asking for the capability. A deployment that needs it reopens this, not
a design that anticipates it.

### Phase 4 — Follow-ons

- Artifact attachments to multiple work objects.
- Batch/folder outputs, manifests, and optional lineage/version relationships.
- Direct-to-object-store upload optimization with server-issued short-lived
  upload credentials, only if the server-streaming path becomes a bottleneck.
- Content scanning and richer previews where deployment requirements demand
  them.

## 11. Alternatives Rejected

### Keep Artifacts Worker-Only

This preserves the simplest current implementation but makes logged-in local
sessions, direct user uploads, and future non-worker execution second-class. It
also forces users to understand task-run hierarchy to preserve a useful output.

### Give Server-Less Local Sessions Their Own Artifact Store

A local store would let `UploadArtifact` exist everywhere, which sounds like
the local-first answer. It is not: the tool would return two different kinds of
thing depending on the surface, and the model would have to explain which one
the user got. It also doubles the metadata model and the storage adapter, and
buys a second sync problem the moment anyone wants to publish upward.

Artifacts exist to be found, referenced, and shared by other people. A session
with no server has no other people in it. Such a session keeps writing files
where it already writes them, which is the behavior that fits it. Section 4.2
records this as out of scope rather than deferred, and section 5.1 records the
one condition a later local store would have to meet.

### Reuse Mutable Team Files As Artifacts

Team files are workspace state. They can be overwritten or deleted and do not
identify a producing operation. Calling them artifacts would make a saved link
change meaning as the workspace changes.

### Return Object-Store URLs Directly

This exposes deployment details, varies across providers, weakens auditing and
authorization, and makes revocation depend on object-store behavior. Short
lived signed URLs may still be used behind an authorized BuildMax endpoint.

### Give Every Agent Full Automatic Upload

Automatic directory harvesting is convenient only until it uploads secrets,
intermediate outputs, or large caches. An explicit tool invocation gives the
model and the user one legible publishing event.

## 12. Open Questions And Evidence Needed

1. **Team selection for local surfaces:** ~~settled for the first slice~~.
   `POST /api/artifacts` resolves the caller's personal Team when the request
   names none and honours an explicit `?team_id=` that the caller is a member
   of. A local client therefore publishes without ever being told about teams.
   What remains open is only whether a client should be able to *choose* and
   remember a team, which is a settings question rather than an API one.
2. **Team storage quota:** which limits belong in the existing quota model,
   and should storage use be a hard admission check or alert-only initially?
3. **Sensitive-content controls:** which private deployments require malware
   scanning, DLP, or MIME restrictions before agent upload is enabled? Gather
   operator requirements rather than hardcoding a cloud-oriented policy.
4. **External sharing:** ~~decided: not planned~~. Neither public links nor
   authenticated cross-organization sharing is built. See phase 3.
5. **Preview support:** which types can be rendered safely and usefully on all
   Portal clients? Start with an allowlist and measure demand.
6. **Run output as artifacts:** ~~decided: no~~. A run's output directory is
   not registered as artifacts, and no backfill of old output is planned. It
   would have meant a second copy of every output file — the compatibility
   route still reads the run-output key space — for the same directory
   harvesting section 11 rejects for agents, applied to a directory an agent
   wrote. An agent that wants a file kept publishes it deliberately with
   `UploadArtifact`; a run's output directory stays the reproducible record of
   what the run left behind. See section 5.3.

7. **A run's output can be silently short.** Not an artifact question — run
   output is deliberately not artifacts (question 6) — but it is the other half
   of "what a run left behind", so it is recorded here until someone fixes it.
   `walkAndUploadFiles` in `internal/agentapp/taskrun/runtime.go` logs a failed
   upload at `Warn` and skips the file, appending to the returned relative paths
   only on success. The record stays consistent with storage, which is why this
   is not a corruption bug; what is missing is that the run still reports
   `SUCCEEDED` and nothing tells the reader that fewer files arrived than the
   agent produced. A dependency failure is therefore indistinguishable from a
   run that simply produced less. Deciding between failing the run, recording a
   partial-output marker, or surfacing the count is open.

## 13. Acceptance Criteria

The first usable increment is complete when:

- a user or authorized agent can upload one explicit file to a personal or
  shared Artifact workspace;
- the response carries a stable `ar_...` ID that another authorized team member
  can resolve, preview when allowed, or download;
- a CLI or Desktop session with no server login has no artifact tool in its
  tool list at all;
- a non-member can neither use that reference nor learn whether it exists;
- object keys, provider credentials, and share tokens do not appear in API
  responses, tool output, traces, or audit events;
- a worker-created result appears as a unified Artifact without losing its
  task-run provenance;
- upload failure never reports a successful Artifact and leaves recoverable
  storage/database state; and
- deletion takes effect immediately at the BuildMax authorization boundary.

