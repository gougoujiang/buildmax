# Plugin Distribution And Private Marketplace

> **Audience:** contributors and operators · **Status:** planned — design ready
> for review; implementation has not started

## Status

- roadmap_priority: `post-Beta, P4 follow-on`
- status: `ready_for_review`
- follows: [enterprise-deployment.md](./enterprise-deployment.md),
  [team-governance.md](./team-governance.md), and
  [system-administration.md](./system-administration.md)
- relates_to: [hook-system.md](./hook-system.md),
  [tool-permissions.md](./tool-permissions.md), and
  [trust-harness.md](./trust-harness.md)
- roadmap: [../ROADMAP.md](../ROADMAP.md)
- created_at: `2026-08-22`

## 1. Decision

BuildMax will treat a plugin as a reusable directory containing capabilities
that already work under `.buildmax/`. A plugin adds distribution and lifecycle;
it does not add another runtime or extension API.

The plugin directory contains the supported content directly. It does **not**
contain another `.buildmax/` directory:

```text
code-review/
├── plugin.yaml
├── README.md
├── skills/
├── agents/
├── mcp.json
├── hooks.yaml
└── hooks/
```

Installed plugins live directly under:

```text
<BUILDMAX_HOME>/plugins/<plugin-name>/
```

The default is `~/.buildmax/plugins/<plugin-name>/`. Respecting
`BUILDMAX_HOME` keeps tests, workers, and isolated installations out of a
contributor's real home.

BuildMax will support two distribution modes over this same directory format:

1. a Git repository cloned directly into the plugins directory;
2. an immutable release published to the deployment's private Plugin
   Marketplace and explicitly downloaded by a user.

These modes share validation, discovery, runtime resolution, and diagnostics.
They deliberately keep different update and trust semantics: a repository is a
developer-controlled working tree; a Marketplace release is an
administrator-published artifact identified by version and digest.

## 2. Why Two Distribution Modes

The two modes serve different parts of one lifecycle rather than competing to
be the only installer.

| Concern | Repository clone | Private Marketplace |
|---|---|---|
| Primary user | Plugin author and developer | Company-wide consumer |
| Setup | `git clone` into the plugins directory | Browse and click/run install |
| Iteration | Edit and test in place | Publish a new immutable release |
| Update | Explicit Git operation | Explicit Marketplace update |
| Identity | Plugin name plus repository URL | Catalog ID plus plugin name |
| Reproducibility | Commit plus dirty state | Release version plus SHA-256 digest |
| Governance | Repository access and review | System Administrator publication and audit |
| Credentials | User's Git/SSH credentials | Existing BuildMax login |
| Server dependency at run time | None after clone | None after download |
| Future worker use | Unsuitable as a mutable source | Suitable as a pinned artifact |

### 2.1 Why Repository Clone Is Worth Supporting

Requiring an author to pack, upload, publish, and download after every edit
would make plugin development unnecessarily slow. A developer should be able to
run:

```bash
git clone git@code.example.com:agents/code-review.git \
  ~/.buildmax/plugins/code-review
```

and have BuildMax discover it on the next runtime start. The repository may be
on a branch, at a detached commit, or locally dirty. That mutability is useful
in development and must be visible rather than prohibited.

BuildMax does not automatically pull a repository. Git already owns branches,
credentials, merge conflicts, and dirty working trees; reproducing those rules
inside an updater would be fragile. A later `buildmax plugin clone` command may
provide a convenience wrapper, but direct `git clone` remains a supported path
and must not require a BuildMax-generated lock record.

### 2.2 Why The Marketplace Still Packages Bytes

The Marketplace should not install by cloning the publisher's repository onto
every member's machine. Doing that would:

- require every consumer to hold Git host credentials;
- make a branch or tag mutable from the consumer's perspective;
- couple installation availability to the Git server;
- make the administrator's reviewed bytes difficult to identify later;
- make yank, audit, and deterministic rollback ambiguous.

The Marketplace therefore stores an archive made from the same plugin root. A
published release is immutable and identified by `(plugin, version, digest)`.
The user performs an explicit download/install action; publication alone never
changes local capability.

### 2.3 One Promotion Path

The intended flow is:

```text
Git repository
    ↓ clone and edit in ~/.buildmax/plugins
local validation and testing
    ↓ pack and publish a named version
private Marketplace release
    ↓ explicit user install
managed local copy in ~/.buildmax/plugins
```

There is no automatic synchronization between a repository installation and a
Marketplace installation. Converting one source type to the other requires an
explicit replace operation so BuildMax never overwrites a working tree.

## 3. Plugin Directory Contract

### 3.1 Supported Content

The plugin root may contribute only content already supported in a workspace
`.buildmax/` directory:

| Path | Meaning |
|---|---|
| `skills/<name>/SKILL.md` | Skill instructions and their adjacent resources |
| `agents/<name>.md` | Subagent definitions |
| `mcp.json` | MCP server definitions |
| `hooks.yaml` | Hook definitions |
| `hooks/` | Scripts or resources referenced by `hooks.yaml` |

`README.md` and `LICENSE` may accompany the runtime content. Unknown runtime
files are reported by validation rather than silently treated as a new plugin
feature.

A plugin may contain only a subset. A skill-only plugin is valid; so is a
plugin that provides MCP configuration and no skill.

### 3.2 Minimal `plugin.yaml`

The first version keeps the manifest deliberately small:

```yaml
name: code-review
description: Company code review skills and agents.
```

- `name` is required, lowercase, and hyphen-separated. It is the stable local
  identity and the default catalog slug.
- `description` is optional display text.

Version, digest, publication actor, source URL, compatibility, permissions, and
dependencies do not belong in this first manifest:

- a Git installation is identified by commit and dirty state, not a package
  version;
- a Marketplace release receives its version and digest when published;
- capabilities are derived from the actual payload instead of trusted from a
  declaration;
- compatibility and dependencies should be added only after a concrete plugin
  needs them.

The parser accepts unknown fields so the manifest can grow additively. A format
version should be introduced only when the first incompatible change actually
exists.

### 3.3 Plugin-Relative Files

MCP and hook configuration may need to refer to scripts bundled with the
plugin. BuildMax supplies one source-scoped expansion value:

```text
${BUILDMAX_PLUGIN_ROOT}
```

It resolves to the directory containing `plugin.yaml`. Each plugin's MCP and
hook configuration is expanded with its own root before configurations are
merged. It is a BuildMax-provided value, not a process environment variable a
plugin can override.

Skills already receive their skill directory when invoked and need no new path
rule. `WORKSPACE_ROOT` keeps its current meaning: the workspace the agent is
operating on, not the plugin directory.

## 4. Discovery And Local State

### 4.1 Directory Discovery Is The Source Of Truth

At startup, BuildMax scans each immediate subdirectory of:

```text
<BUILDMAX_HOME>/plugins/
```

A directory with a valid `plugin.yaml` is a plugin. This is intentionally enough
for a manually cloned repository to work. Discovery does not depend on a
registry generated by BuildMax.

Directories beginning with `.` are reserved for BuildMax's staging, cache, and
state and are not plugins.

### 4.2 Manager State Is Supplemental

BuildMax keeps supplemental state at:

```text
<BUILDMAX_HOME>/plugins/.state.json
```

It records facts known to the installer, such as:

- source type: `repository`, `marketplace`, or `local`;
- repository URL and last observed commit, when available;
- Marketplace server, catalog ID, release version, and digest;
- whether the plugin is disabled;
- install and update timestamps.

The state file does not list plugin content and is not required for discovery.
A repository cloned manually appears as an unmanaged repository plugin until
BuildMax inspects its `.git` metadata. An ordinary directory appears as a local
plugin. BuildMax never writes source metadata into the plugin directory because
doing so would dirty a Git checkout.

State changes use a process lock and atomic replacement. If `.state.json` is
missing or damaged, valid plugin directories still load, but BuildMax reports
that Marketplace provenance or disabled state could not be recovered.

### 4.3 Source Replacement

The same plugin name cannot be active from two directories or sources. In
particular, Marketplace installation refuses to overwrite an existing Git
working tree. The user must explicitly remove, rename, or replace it.

Marketplace update is staged under a reserved dot directory, validated, and
atomically exchanged with the active directory. The previous managed copy may
be retained in a cache for rollback. Repository updates remain ordinary Git
operations performed by the user.

### 4.4 Runtime Snapshot

`agentapp` resolves plugins when it starts and keeps that snapshot for the life
of the assembled runtime. A clone, pull, install, update, disable, or removal
does not change an in-flight run. CLI sees changes on its next invocation;
Desktop rebuilds its runtime after a managed plugin action.

A repository edited while a run is in progress can still change a file the run
has not read yet. The trace therefore records repository dirty state at run
start, and the UI describes repository plugins as development sources rather
than immutable inputs. Authors who need reproducibility use a clean commit or a
Marketplace release.

## 5. Runtime Resolution

Plugins add a third configuration source without changing existing tool
contracts. Precedence is:

```text
workspace .buildmax  >  user-global configuration  >  plugins
```

The concrete behavior follows each subsystem:

| Content | Resolution |
|---|---|
| Skills | Workspace first, then global, then plugins sorted by name; first name wins |
| Subagents | Workspace first, then global, then plugins sorted by name; first name wins |
| MCP servers | Plugins merge first, then global, then workspace; later server IDs replace earlier ones |
| Hooks | Additive order: global settings, plugins sorted by name, workspace |

Two plugins contributing the same skill, subagent, or MCP server ID are an
error, with both sources named. Alphabetic order exists for deterministic
loading and hook dispatch; it must not silently resolve an identity collision.

A global or workspace definition may intentionally override a plugin
definition. `buildmax plugin status` and Desktop diagnostics show that shadowing
instead of making the plugin appear fully active.

## 6. Repository Distribution

Repository distribution is supported by convention rather than by a new server
protocol:

1. The author keeps `plugin.yaml` and plugin content at the repository root.
2. A developer clones or checks out that repository under the plugins
   directory.
3. `buildmax plugin validate <path>` parses every contributed capability and
   reports conflicts and derived effects.
4. BuildMax loads the repository directly on the next runtime start.

Status reports, when Git metadata is available:

- remote URL;
- branch or detached state;
- full commit hash;
- whether tracked or untracked changes exist.

BuildMax does not require a remote, does not fetch during discovery, and does
not send Git credentials anywhere. Private repository authentication remains
the user's existing Git/SSH setup.

Repository distribution is developer-friendly but is not called
administrator-managed in the product. It has repository provenance, not
Marketplace publication provenance.

## 7. Private Marketplace Distribution

### 7.1 Scope And Roles

The Marketplace belongs to the deployment, not a team:

| Action | Authority |
|---|---|
| Browse, inspect, and download | Any active authenticated user |
| Create or edit a catalog entry | System Administrator |
| Publish or yank a release | System Administrator |
| Archive a plugin | System Administrator |
| Install or update locally | User controlling that `BUILDMAX_HOME` |

This lets a System Administrator manage company capabilities without granting
access to team prompts, files, artifacts, or traces. Per-team catalog visibility
is deferred until there is evidence that a deployment-wide catalog is too broad.

### 7.2 Pack And Publish

Publishing takes a plugin directory and a version supplied by the publisher:

```bash
buildmax plugin publish ./code-review --version 1.2.0
```

The client validates and packs the directory. The server independently:

1. authenticates the System Administrator;
2. streams the bounded archive while calculating SHA-256;
3. validates paths, `plugin.yaml`, and every supported payload parser;
4. derives a sanitized capability report;
5. stores immutable bytes;
6. creates the release row and audit event.

The version is Marketplace release metadata rather than a field plugin authors
must bump while iterating in Git. Semantic versioning is useful for selecting a
release, but publication does not infer it from a Git tag.

Publishing the same `(plugin, version)` twice returns `409`, even for identical
bytes. A correction requires a new version.

### 7.3 Install And Update

A member explicitly installs a release:

```bash
buildmax plugin install code-review
buildmax plugin install code-review --version 1.2.0
```

Installation:

1. uses the existing BuildMax login for the selected server;
2. shows version, publisher, digest, and derived capabilities;
3. downloads to a reserved staging directory;
4. verifies the SHA-256 digest before extraction;
5. validates the extracted directory again;
6. atomically places it at
   `<BUILDMAX_HOME>/plugins/code-review/`;
7. records Marketplace provenance in `.state.json`.

Publishing does not push to clients. Updates are explicit and show the old and
new capability reports before replacement. There is no automatic update in the
first version.

### 7.4 Yank And Archive

- Yanking a release removes it from default install and update selection.
  Existing downloaded copies continue to work. Exact-version recovery requires
  an explicit acknowledgement of the yanked state.
- Archiving a plugin hides it from the default catalog and prevents new
  releases. It does not remotely delete local copies.
- Published bytes are never replaced in place. Product APIs expose no hard
  delete for a release that may explain a past installation or run.

### 7.5 Storage And Model

The server needs only two deployment-scoped concepts:

- `Plugin`: stable catalog identity, name, description, and archive state;
- `PluginRelease`: plugin, version, digest, object key, size, publisher,
  publication time, sanitized inspection, and optional yank state.

Package bytes sit behind a narrow plugin package storage interface with local
filesystem and S3-compatible implementations. They are not task artifacts and
do not inherit team artifact authorization or retention.

Implementation uses singular database tables and new prefixed public IDs, and
updates the data-model and ID references in the same change. The exact row and
API request shapes belong in the implementation slice rather than this first
design.

### 7.6 API Shape

The intended surface is small:

```text
GET  /api/plugins
GET  /api/plugins/{plugin_name}
GET  /api/plugins/{plugin_name}/releases/{version}/download

POST /api/admin/plugins
POST /api/admin/plugins/{plugin_name}/releases
POST /api/admin/plugins/{plugin_name}/releases/{version}/yank
POST /api/admin/plugins/{plugin_name}/archive
```

Routes are provisional until implemented; `internal/server/handlers/routes.go`
becomes the source of truth when they ship. Upload and download stream bytes and
never buffer a whole package in memory.

The audit trail records catalog creation, metadata change, release publication,
yank, and archive. It records actor, target, version, and digest prefix, never
package contents or configuration values. Downloads are ordinary authenticated
reads rather than governance events in the first slice.

## 8. Trust And Capability Inspection

Both source modes can provide active behavior:

- skills and subagents are instructions that can cause tool use;
- stdio MCP starts a process, potentially with credentials;
- HTTP MCP and hooks can reach network destinations;
- command hooks execute local programs.

The same validator inspects both modes and reports:

- skill, subagent, and MCP names;
- MCP transport, executable name, and HTTP host;
- hook event, type, executable name, and HTTP host;
- referenced environment variable names;
- local file paths referenced through `BUILDMAX_PLUGIN_ROOT`;
- collisions and shadowed definitions.

It does not store command arguments, header values, environment values, prompts,
or file contents in the catalog inspection record.

Marketplace publication means “published by this administrator,” not “safe.”
Repository plugins are labeled with their repository and working-tree state.
Existing tool permissions, hook gates, sensitive-path checks, sandbox decisions,
and operator policy continue to apply; a plugin cannot grant itself permission.

The package contract forbids embedded credentials. Validation rejects obvious
private-key material and warns about suspicious literal MCP environment values,
but cannot prove that arbitrary instructions or text contain no secret.

Direct clone does not weaken an existing enforcement boundary: a local user who
can clone into `~/.buildmax/plugins` can already write their own `.buildmax`
configuration. Deployments that need execution enforcement use operator policy,
not Marketplace provenance as a substitute.

## 9. Product Surfaces

### 9.1 CLI

The first useful command set is intentionally small:

```text
buildmax plugin validate [path]
buildmax plugin list
buildmax plugin status [name]
buildmax plugin publish <path> --version <version>
buildmax plugin install <name> [--version <version>]
buildmax plugin update <name>
buildmax plugin disable <name>
buildmax plugin enable <name>
buildmax plugin uninstall <name>
```

Direct `git clone` is part of the repository workflow; a redundant BuildMax
clone command is optional. `uninstall` must not silently delete a dirty Git
working tree. For a repository source it prints the path and requires the user
to manage or explicitly confirm removal.

### 9.2 Desktop

Desktop uses the same Go services to show:

- repository, local, or Marketplace source;
- commit and dirty state, or release version and digest;
- contributed capabilities and required environment names;
- collision, shadowing, validation, and update status;
- explicit install, update, disable, and uninstall actions.

### 9.3 Portal

Portal's System Administration area manages catalog entries and releases.
Normal users may browse the catalog, but Portal must not claim a plugin is
installed on a local machine it cannot inspect. The primary install surfaces
are CLI and Desktop; raw archive download from Portal is optional.

## 10. Provenance And Future Worker Use

Every run records the active plugin inventory at start.

For a repository plugin:

```text
name, source type, remote URL when present, commit, dirty flag
```

For a Marketplace plugin:

```text
name, catalog ID, version, digest, source server
```

This bounded metadata contains no package content or secrets. Skill invocation,
subagent start, MCP calls, and hook events should carry plugin origin when the
resolved definition came from one.

Future Portal and worker distribution should consume only immutable Marketplace
releases pinned before dispatch. A worker must not clone a mutable repository,
receive a developer's Git credential, or resolve “latest” while starting a run.
Team activation, secret selection, and worker delivery require a follow-on
design because they change authorization and the non-interactive trust boundary.

## 11. Implementation Ownership

The dependency direction remains unchanged:

- `internal/config` discovers plugin roots, resolves source-relative paths, and
  merges plugin configuration layers;
- `internal/agentapp` assembles the resolved skills, subagents, MCP servers, and
  hooks for CLI and Desktop;
- `internal/core/model` owns Marketplace domain records and store interfaces;
- `internal/service/plugin` owns publication and catalog lifecycle;
- `internal/infra/db` persists catalog metadata;
- `internal/infra/objectstore` stores immutable package bytes;
- `internal/server/handlers` owns authenticated catalog and administration
  routes;
- `internal/interface/cli` and Desktop expose local workflows.

`internal/core/agent` remains unaware of plugins. It receives the same resolved
tools, instructions, and hooks it receives today.

## 12. Delivery Phases

### Phase A — Directory Format And Repository Workflow

- root-level plugin format and minimal `plugin.yaml`;
- discovery under `<BUILDMAX_HOME>/plugins`;
- validation, merge rules, plugin-relative paths, and diagnostics;
- direct cloned-repository support;
- plugin provenance in traces.

Acceptance: cloning a valid repository into a clean isolated plugins directory
makes its capability available to CLI and Desktop without copying files or
writing a generated registry.

### Phase B — Private Marketplace

- catalog records and package object storage;
- administrator publish, yank, and archive flows;
- authenticated browse and download;
- explicit local install/update with digest verification;
- audit events and source-aware local state.

Acceptance: an administrator publishes one version from a tested repository;
another company account installs it by name; both sides report the same digest.

### Phase C — Product UI

- Portal catalog administration and browsing;
- Desktop Marketplace install and update;
- clear capability, source, dirty-state, and provenance presentation.

Acceptance: a member can discover and use a Marketplace plugin without editing
configuration or handling an archive manually.

### Phase D — Team And Worker Distribution, Deferred

Write a follow-on design before implementing centrally activated plugins for
Portal or workers. It must decide team ownership, who may enable active hooks or
stdio MCP, server-held secret scope, package materialization, version pinning,
and sandbox/egress reporting.

## 13. Alternatives Rejected

### Marketplace Only

This gives the cleanest governance story but makes every development edit pass
through release machinery. It would push authors back to copying directories by
hand, outside the product. Supporting a directly cloned repository keeps the
authoring loop obvious and promotes the same bytes into the Marketplace later.

### Git Only

This is pleasant for developers and poor for broad company consumption. It
requires Git credentials and knowledge, exposes mutable refs, and provides no
central publication, audit, or stable digest. It remains a source mode, not the
managed company catalog.

### Marketplace Backed By Git Clone At Install Time

Storing only repository URLs and refs in the catalog looks cheaper than storing
archives, but moves mutability, Git authentication, and availability into every
installation. The reviewed release must be the bytes users download, so the
Marketplace stores packages.

### Copy Into Global Configuration

Copying skills into `<BUILDMAX_HOME>/skills` and merging JSON/YAML destroys
ownership information. Update and uninstall could not distinguish user-authored
content from plugin content. Keeping one directory per plugin makes its source
and lifecycle inspectable.

### General Plugin Runtime

Dynamic binaries or a new SDK create another extension contract beside skills,
subagents, MCP, and hooks and expand the execution boundary. Packaging current
behavior delivers reuse without that cost.

## 14. Validation

Implementation is not complete until tests prove:

- a plugin root needs no nested `.buildmax` directory;
- a manually cloned valid repository is discovered without `.state.json`;
- discovery never performs a network request or Git fetch;
- repository commit and dirty state appear in status and trace metadata;
- Marketplace installation refuses to overwrite a Git working tree;
- archive traversal, symlink, duplicate-path, file-count, and expanded-size
  attacks are rejected on publish and install;
- a modified or truncated download fails digest verification and is never
  installed;
- a published release cannot be overwritten;
- publication alone never changes an installed plugin;
- workspace and global definitions resolve ahead of plugin definitions;
- two plugins with a colliding runtime name fail with both sources named;
- global, plugin, and workspace hooks run in documented order;
- `${BUILDMAX_PLUGIN_ROOT}` is scoped to the originating plugin;
- an in-flight runtime keeps its resolved snapshot across managed updates;
- a non-admin cannot mutate the catalog and an admin gains no team-content
  access through catalog routes;
- audit and trace records contain provenance but no configuration values or
  secrets;
- CLI and Desktop load the same plugin inventory through `agentapp`;
- `git diff --check`, documentation link checks, focused Go tests, and relevant
  CLI/Desktop end-to-end suites pass.

## 15. Open Questions

1. Should a future convenience clone command invoke the system Git binary or
   only validate a repository the user cloned themselves?
2. Should the Marketplace retain one previous local version automatically, or
   should rollback redownload an exact release?
3. Should Portal expose raw archive downloads, or direct users to CLI/Desktop
   so installation state stays truthful?
4. What evidence would justify compatibility and dependency fields in
   `plugin.yaml`?
5. Do regulated deployments need offline signatures in addition to TLS,
   administrator publication, and server-calculated digests?

## Related Documents

- [Skills and subagents](../guide/skills-and-subagents.md)
- [MCP servers](../guide/mcp.md)
- [Hooks](../guide/hooks.md)
- [Configuration reference](../reference/configuration.md)
- [System administration](./system-administration.md)
- [Team governance](./team-governance.md)
- [Tool permissions](./tool-permissions.md)
- [Trust harness](./trust-harness.md)
