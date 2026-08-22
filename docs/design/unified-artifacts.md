# Unified Artifacts

## Status

- roadmap_priority: `P2 follow-on`
- status: `planned`
- follows: [surface-positioning.md](./surface-positioning.md) and
  [team-governance.md](./team-governance.md)
- roadmap: [../ROADMAP.md](../ROADMAP.md)
- created_at: `2026-08-22`

## 1. Decision

BuildMax will make an artifact a durable output object in a personal or team
workspace, rather than a file that only exists as a child of a worker task
run. Portal persists remote artifacts in a Team namespace, which is the
existing authorization boundary; a personal Portal workspace is represented by
that user's personal Team and need not expose the Team concept in its UI. CLI
and Desktop may instead use a local personal artifact store.

The shared Agent runtime will expose an `UploadArtifact` tool when an artifact
service is available. It accepts an explicitly named local file, persists it
through the selected local or remote store, records an Artifact, and returns a
BuildMax reference. A remote artifact returns a stable BuildMax URL; a local
artifact returns a local view reference until the user deliberately publishes
it to a remote personal or team workspace.

The object store remains an implementation detail. A BuildMax artifact URL is
not a bucket key and is not an object-store presigned URL.

Task-run artifacts remain a valid producer of unified Artifacts. A run can
therefore keep its reproducible output directory while users can find, save,
reference, and share the files it produced without knowing its task or run ID.

## 2. Product Goal

An agent, a user, or a background run should be able to make a useful file
available to the team and give the recipient one durable URL. The recipient
should be able to:

- preview or download it when they have team access;
- attach it to an issue, conversation, or result without copying storage keys;
- retain the stable internal URL in a document or message; and
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
| IDs | `ar_` and `f_` prefixes are reserved in `internal/util/id.go`. | They are not the IDs of a public artifact resource today. |

`task_run_artifact` is intentionally a set of paths, not a durable resource:
it has no public prefixed ID or timestamps. The new model must not reinterpret
that table as if it already provided the required contract.

## 4. Scope

### 4.1 In Scope

- An Artifact in a personal or team workspace, with a stable `ar_...`
  identifier.
- One immutable file per Artifact in the first slice.
- Server-mediated upload, metadata lookup, preview, and download.
- `UploadArtifact` as a normal shared Agent tool when configured for the
  execution surface.
- Provenance for agent uploads, task-run outputs, and direct user uploads.
- Stable authenticated URLs and separate revocable external share links.
- Portal artifact cards, list/detail view, and safe lightweight previews.
- Migration of newly produced worker output files into the unified model while
  retaining the existing task-run artifact listing for compatibility.
- Team authorization, deletion/tombstoning, retention hooks, quotas, and audit
  events at the artifact boundary.

### 4.2 Out Of Scope

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

The Portal's remote Artifact model has one immutable content object per
artifact. `TeamID` is required here because Team is the Portal authorization
namespace, including for a user working alone in their personal Team:

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

A local artifact store uses the same public concepts and immutable-content
rules, but its metadata need not enter the Portal database. It is owned by the
local user and project/workspace selected by that surface. A local artifact is
viewable in that surface but has no remotely reachable or externally shareable
URL. Publishing it creates a distinct remote Artifact with `source_type` set
to `local_publish` and the appropriate personal or shared Team scope.

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

This avoids making an uploaded file depend on a task run merely because the
first producer was a worker.

### 5.3 Existing Task-Run Rows

Keep `task_run_artifact(task_run_id, relative_path)` as the internal run-output
index during migration. On terminal run processing, each successfully uploaded
file creates or finds its corresponding Artifact and records its task-run
origin. Do not change an old task-run path into an `ar_` ID in place.

The compatibility task-run route can return its current response while the UI
and new API prefer Artifact records. Once every supported storage backend has
migrated and consumers use Artifact IDs, a later design can retire the old
output-only contract.

## 6. Access And URLs

### 6.1 Internal URL

The canonical URL is a stable application URL such as:

```text
/api/teams/{team_id}/artifacts/{artifact_id}/content
```

It requires an authenticated member of the artifact's team. The opaque
`artifact_id` is an identifier, not a credential. A copied internal URL does
not grant access to someone outside the team.

The Portal may provide a human-facing detail route in addition to this API
route. It must resolve the same Artifact and enforce the same authorization.

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
alongside other default tools. It is available by default when the current
surface supplies either a local personal artifact service or a configured,
authorized remote personal/team workspace.

The tool is not silently emulated by merely returning a source-file path. A
local CLI or Desktop session needs an actual local artifact store; otherwise it
retains normal local output behavior and reports that artifact upload is
unavailable if the model attempts it. This preserves the local-first contract
without claiming that a path on one machine is a shareable object.

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
the Artifact ID, filename, size, and a canonical remote URL or local view
reference. A storage failure returns a meaningful tool error and leaves no
successful artifact record.

The tool does not auto-upload every file an agent writes. The model must choose
the final file it intends to present. This makes the output understandable,
avoids accidental publication of `.env` files or caches, and keeps storage
costs bounded.

### 7.3 Prompting And Results

The runtime prompt tells agents to use `UploadArtifact` for a file that should
be delivered, retained, or shared, and to cite the returned reference in their
final answer. It must call out when the reference is local-only. An Artifact
result card should include title, filename, producing run or agent, creation
time, preview/download action, and canonical link or local view action.

The LLM-facing name follows `internal/tool/names.go`; when implementation
adds the constant, `docs/guide/tools.md` becomes the user-facing source of
truth for its arguments and availability.

## 8. Authorization, Governance, And Limits

Remote Portal artifacts are team resources. Team membership is the baseline
read boundary; the user's personal Team is the private single-user case. A
local artifact is private to its local artifact store until published. An
artifact never derives authorization from a run URL alone. The detailed matrix
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

Use a generated key partitioned by team and Artifact ID; a representative
shape is `teams/{team_id}/artifacts/{artifact_id}/content`. It makes object
ownership clear while allowing bucket layout to evolve behind the adapter.

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

The storage interface remains portable across local filesystem and S3/MinIO.
Server-side behavior, not storage-provider URL behavior, defines the product
contract.

## 10. Delivery Phases

### Phase 1 — Foundations

- Add core Artifact model, store, object-storage contract, and stable remote
  Team-scoped read/download APIs; expose a personal Team as a personal
  workspace in product UI.
- Add migrations, prefixed ID use, authorization tests, quota checks, and
  redacted audit events.
- Provide Portal listing/detail with download and narrow safe previews.
- Keep public sharing disabled.

### Phase 2 — Agent And Worker Producers

- Add `UploadArtifact` to the shared Agent runtime and document it in the tool
  guide.
- Enable it for Portal task runs and any other authenticated surface with the
  remote service configured; add a local personal store for direct CLI/Desktop
  use if it meets the surface's local-result needs.
- Register future worker output files as unified Artifacts and show them in
  conversation/issue result cards.
- Retain task-run artifact route compatibility and migrate Portal consumers.

### Phase 3 — Intentional External Sharing

- Define approved roles, deployment policy, expiry defaults, and user-facing
  warning copy.
- Add revocable high-entropy share tokens, access tests, and audit events.
- Decide whether authenticated external recipients, password protection,
  download limits, and malware scanning are necessary before public links.

### Phase 4 — Follow-ons

- Artifact attachments to multiple work objects.
- Batch/folder outputs, manifests, and optional lineage/version relationships.
- Direct-to-object-store upload optimization with server-issued short-lived
  upload credentials, only if the server-streaming path becomes a bottleneck.
- Content scanning and richer previews where deployment requirements demand
  them.

## 11. Alternatives Rejected

### Keep Artifacts Worker-Only

This preserves the simplest current implementation but makes local agents,
direct user uploads, and future non-worker execution second-class. It also
forces users to understand task-run hierarchy to preserve a useful output.

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

1. **Scope for local surfaces:** should CLI/Desktop connect to a selected
   Portal team for upload, or should a later local artifact store synchronize
   on demand? Prototype the connected-team flow before adding a second sync
   engine.
2. **Team storage quota:** which limits belong in the existing quota model,
   and should storage use be a hard admission check or alert-only initially?
3. **Sensitive-content controls:** which private deployments require malware
   scanning, DLP, or MIME restrictions before agent upload is enabled? Gather
   operator requirements rather than hardcoding a cloud-oriented policy.
4. **External sharing:** is public anonymous access necessary, or is
   cross-organization authenticated sharing sufficient? No external link ships
   without a decision on expiry, revocation, ownership, and audit retention.
5. **Preview support:** which types can be rendered safely and usefully on all
   Portal clients? Start with an allowlist and measure demand.
6. **Run compatibility:** should old task-run output be backfilled, or should
   unified records begin only for new runs? Choose based on deployment upgrade
   cost and whether existing outputs need stable URLs.

## 13. Acceptance Criteria

The first usable increment is complete when:

- a user or authorized agent can upload one explicit file to a personal or
  shared Artifact workspace;
- a remote response includes a stable `ar_...` URL that another authorized
  team member can open, preview when allowed, or download; a local response is
  explicitly labelled local-only;
- a non-member cannot use that URL or enumerate the artifact;
- object keys, provider credentials, and share tokens do not appear in API
  responses, tool output, traces, or audit events;
- a worker-created result appears as a unified Artifact without losing its
  task-run provenance;
- upload failure never reports a successful Artifact and leaves recoverable
  storage/database state; and
- deletion and any later share-link revocation take effect immediately at the
  BuildMax authorization boundary.

