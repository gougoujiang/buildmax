# Team Secrets And Run Delivery

## Contents

- [Status](#status)
- [1. Problem](#1-problem)
- [2. Decision](#2-decision)
- [3. What This Does Not Protect](#3-what-this-does-not-protect)
- [4. Scope And Ownership](#4-scope-and-ownership)
- [5. Resource Model](#5-resource-model)
- [6. Declaration And Binding](#6-declaration-and-binding)
- [7. Run Lifecycle](#7-run-lifecycle)
- [8. Delivery Modes](#8-delivery-modes)
- [9. Storage Backends](#9-storage-backends)
- [10. Authorization](#10-authorization)
- [11. Audit And Provenance](#11-audit-and-provenance)
- [12. Redaction](#12-redaction)
- [13. Existing Credential Debt](#13-existing-credential-debt)
- [14. Failure Semantics](#14-failure-semantics)
- [15. API Shape](#15-api-shape)
- [16. Package Boundaries](#16-package-boundaries)
- [17. Alternatives Rejected](#17-alternatives-rejected)
- [18. Phases](#18-phases)
- [19. Acceptance Evidence](#19-acceptance-evidence)
- [20. Open Questions](#20-open-questions)

## Status

- roadmap_priority: [`R0`](../ROADMAP.md) for the credential debt in §13, and
  [`R3`](../ROADMAP.md) for the team-facing surface. It answers Phase D3 of
  [plugin-team-distribution.md](plugin-team-distribution.md), which deferred
  secret delivery to a follow-on record.
- status: `designed, not started` — no table, route, or runtime path exists.
  `internal/infra/db` has no secret row, and the only credential a worker
  legitimately holds today is its run token.
- supersedes: the `run-scoped-secret-broker` proposal, whose settled decisions
  are here and whose remaining uncertainty is §20.

## 1. Problem

A worker executes model-chosen commands on behalf of a Team, and useful work
needs credentials: an Agent driving `git` and `gh` needs GitHub authority, one
calling an internal service needs its credential, one deploying needs cloud
authority.

There is no Team-scoped Secret resource. A credential reaches a run only if the
deployment put it in the worker's ambient environment, which gives every Team
and every Agent the same values, has no rotation, no per-Agent choice, and no
audit. [plugin-team-distribution.md](plugin-team-distribution.md) §5 stops at
reporting the environment variable names a release reads; it does not decide
who supplies them.

Two properties of the ambient path make it unusable rather than merely coarse:

- `internal/infra/sandbox/env_scrub.go` strips secret-shaped names from every
  sandboxed child, including `GITHUB_TOKEN` and anything ending `_TOKEN`,
  `_KEY`, `_SECRET`, or `_PASSWORD`. Because the sandbox defaults off on the
  CLI baseline and on for workers, the same configuration works locally and
  fails silently in a pod.
- The worker still receives credentials that are not its Team's business at
  all — the deployment's object-store key, and in direct mode a provider API
  key. §13 records what has to leave.

## 2. Decision

BuildMax gains a Server-side Secret Broker that:

1. represents a Secret as a Team-owned, versioned resource, or as a reference
   to an external secret manager;
2. binds its use to an immutable Agent revision that declares which Secrets it
   needs and how each is delivered;
3. snapshots that authorization onto one TaskRun when the worker claims it;
4. materializes only the values computed for that run; and
5. delivers each value into the run as an environment variable, a rendered
   credential file, or both — whichever the Agent declared.

Values are write-only in every user-facing API. There is no reveal operation.

### 2.1 Delivery Is Run-Level, Not Per-Consumer

The delivery question has one answer and it is worth stating as a decision
rather than a detail: a credential is delivered to the **run**, not to a named
process inside it.

An earlier design delivered values only into named consumers — a stdio MCP
server's explicit environment, a typed HTTP authorization slot, a file handed
to one process — and refused delivery to model-chosen commands. It is rejected
because of what a BuildMax Agent is. The Agent composes shell commands and
invokes tools it selects at run time, including tools it writes during the run.
Any mechanism whose unit of work is "one adapter per tool" is sized against an
unbounded set, so it authenticates `gh` and not the Python script the model
wrote a minute later. That is not a smaller version of the feature; it is a
different feature that does not solve the problem.

The two supported modes are therefore both run-level. §17 records what this
costs and what was given up with it.

## 3. What This Does Not Protect

Both delivery modes put the value inside the run, where the Agent's own
commands can read it. `printenv` reveals an environment grant; `cat` reveals a
file grant. Nothing here prevents that, and no surface may describe it as if it
did.

What remains, and what each part is worth:

| Control | What it buys |
|---|---|
| Team ownership | One Team's values are unreachable from another Team's runs |
| Agent-revision binding | A run receives only what its Agent declared, not the Team's whole set |
| TaskRun snapshot | The authorization is fixed when the run is claimed and cannot be widened from inside the run |
| Short-lived credentials | A disclosed value expires; exchange at run start is the main exfiltration control |
| Narrow provider scope | A repository-scoped token cannot act outside that repository, whoever holds it |
| Encrypted storage | A database or backup disclosure does not yield values |
| No reveal API | A value cannot be recovered through the management surface, only used by an authorized run |
| Audit and revocation | Every grant is attributable and can be withdrawn |
| Exact-value redaction | Reduces accidental appearance in logs, traces, and tool results; not a confidentiality boundary |

The boundary this design draws is around **which run may exercise which
authority, for how long, on whose behalf**. It is not a boundary around the
bytes of the value once a run is authorized to use it.

Two consequences that Portal copy and user documentation must carry, because a
Team that misreads them will grant a credential it should not have granted:

- an Agent can read every Secret granted to its run; and
- a member who can trigger a shared Agent can obtain the values that Agent
  holds, without ever gaining a binding or read permission of their own.

A Team that cannot accept this should narrow the credential's provider-side
scope, shorten its lifetime, or use a different Agent. Delivery mode is not the
lever.

## 4. Scope And Ownership

**Team is the only ownership scope.** A Secret belongs to exactly one Team,
which is the same boundary that owns Agents, plugin activations, and audit. An
Agent definition may reference only Secrets in its own Team; a binding whose
Agent and Secret disagree is refused at write time, not at run time.

There is deliberately no deployment-global, Team-independent Secret that Agents
across Teams could name. The pressure to add one is real — an operator with one
shared internal credential would rather write it once — and it is refused for
now because a global value has no owner to attribute it to, no Team to revoke
it from, and no answer to "which Teams' runs can read this". A Team that needs
the same credential as another Team creates its own Secret with the same value;
that duplication is the visible cost of an ownership model that stays
answerable.

This does not describe BuildMax's own credentials. The database password, JWT
signing key, KEK, object-store administration credential, and managed provider
keys are operator-owned deployment configuration, never Team Secrets, and never
delivered to a run as a grant. §5.6 keeps the classes apart.

There is also no Server-side Project or deployment-environment entity
introduced to scope Secrets. Team is the Server ownership boundary; the local
`Project` of `internal/core/localproject` is a client concept and irrelevant
here.

## 5. Resource Model

The `xxxRow` structs in `internal/infra/db` remain the schema source of truth
once written; the shapes below are what they must express. Public identifiers
use `NewPublicID` per [entity-identity.md](entity-identity.md); timestamps
follow [timestamp-representation.md](timestamp-representation.md).

### 5.1 `secret`

One Team-owned named group of entries.

| Field | Meaning |
|---|---|
| `id`, `public_id` | Numeric relational key and opaque public handle |
| `team_id` | Ownership and authorization boundary; see §4 |
| `name` | Team-unique, non-secret display name |
| `description` | Optional bounded explanation; must not carry a value or locator |
| `provider` | `embedded`, or an operator-configured external provider name |
| `state` | `active`, `disabled`, or `destroyed` |
| `current_version_id` | The version a following declaration resolves for the next run |
| `created_by`, timestamps | Administrative attribution |

A Secret is a **group, not a single value**: one name holding several named
entries — `access_key_id` and `secret_access_key`, or `username` and
`password`. §5.2 gives the reason, and it is correctness rather than
convenience.

`(team_id, name)` is unique. Renaming changes display metadata, not identity.
Disabling refuses new run grants and new materializations. Destruction erases
recoverable version material once no active reference remains; it does not
rewrite audit history.

An entry name is an identifier: `[A-Za-z_][A-Za-z0-9_]*`, validated on every
write. The constraint exists so a group can be injected wholesale as environment
variables (§5.3) without a class of entries that silently cannot be, and it
costs nothing elsewhere — template parameters match by name too.

Every entry is write-only. The non-secret parameters a rendered file also needs
— a cluster's server URL and CA certificate, an AWS region — do not belong
here: §5.4 and §5.5 supply them as literals, where they stay readable. Placement
expresses the classification, so no entry carries a secret-or-config flag and
creating a Secret is nothing but typing names and values.

There is deliberately no `kind` field. An earlier draft carried one to select a
default file renderer. It made storage responsible for interpreting the value,
duplicated a decision §5.5 already owns, and forced "one Secret is one whole
rendered file" — under which a kubeconfig's non-secret server URL and CA
certificate would have been sealed inside an encrypted blob nobody could read or
diff. The rendering is chosen at declaration time instead, and whatever
structure a credential has is carried by its entry names.

### 5.2 `secret_version`

One immutable snapshot of the whole entry set.

| Field | Meaning |
|---|---|
| `id`, `secret_id`, `version` | Identity and monotonically increasing logical version |
| `ciphertext`, `nonce` | AEAD-encrypted entry map, or encrypted provider descriptor |
| `entry_names` | The keys present in this version, stored in the clear |
| `wrapped_dek`, `key_id` | Envelope-encryption metadata |
| `provider_version` | Exact external version when one is pinned rather than followed |
| `created_by`, `created_at` | Rotation attribution |
| `destroyed_at` | Cryptographic erasure or confirmed external deletion |

**The version covers the entire set, and that is why the group exists.**
Rotating an AWS credential produces a new access key ID and a new secret access
key together. Were those two Secrets with independent versions, a run claimed
between the two writes would receive the old ID with the new key — and would do
so non-deterministically, depending on timing. One version of one set removes
the window. The same holds for a username and password pair, and for a cluster
token reissued alongside its CA.

A value update appends a version and atomically moves `current_version_id`. It
never updates ciphertext in place. A rewrap for KEK rotation may update only
`wrapped_dek` and `key_id`; the logical version does not advance, because no
value changed.

`entry_names` is plaintext because a name is not a value, and because
declaration validation (§5.5) and Portal listing must work without decrypting
anything.

The stored material must not be assumed to be a static string map. An exchanged
short-lived credential and a provider lease have to fit the same grant path
without a migration; see §9.3.

### 5.3 `agent_secret_env`

One Agent revision's declaration that a Secret arrives as environment
variables.

| Field | Meaning |
|---|---|
| `agent_revision_id` | Immutable Agent definition that declares the requirement |
| `secret_id` | Team-owned Secret group, in the Agent's own Team |
| `entry_name` | Which entry of the group, or empty for every entry |
| `env_name` | Variable name, when one entry is named |
| `prefix` | Optional prefix applied to each entry name, when the whole group is taken |
| `version_mode` | Follow `current`, or pin an exact logical version |
| `required` | Whether the run fails when the grant cannot be produced |
| `created_by`, `created_at`, `revoked_at` | Administrative lifecycle |

Two forms, and the choice is the Agent's:

- **One entry, one variable.** `entry_name` and `env_name` are both given, and
  the entry arrives under the declared name. This is the recommended default:
  the run receives what the Agent said it needed and the declaration reads as
  documentation of what the Agent uses.
- **The whole group.** `entry_name` is empty and every entry arrives under its
  own name, optionally with `prefix`. This is Kubernetes' `envFrom` shape, and
  it is supported because a Team that has already grouped a credential
  correctly should not have to restate every member of it. §5.1's identifier
  constraint on entry names is what makes it well-defined.

The convenience is real and so is its cost: the run receives entries the Agent
never named, which widens what §3 already concedes. It is a Team's call to make,
not a mode to reach for by default, and Portal should show a whole-group
declaration as exactly that rather than expanding it into a list that implies
each was chosen.

A write is valid only when the Agent and Secret belong to the same Team, a named
`entry_name` appears in the resolved version's `entry_names`, and the caller
holds the required Team permission. The resolved variable names of a revision
must not collide — across two whole-group declarations, or a whole-group and a
named one — and the collision is refused at write time against the current
version, with `prefix` available to resolve it.

There is deliberately no `plugin_activation_id`, `plugin_name`,
`consumer_name`, or `approved_digest` on this record or on §5.5's. Those fields
belonged to per-consumer delivery, where "exactly which code receives this" was
an enforceable question. Run-level delivery makes it unenforceable — every
process in the run sees an environment grant — so pinning a release digest would
be ceremony that protects nothing. They return only with a per-consumer mode, if
one ever does.

### 5.4 `secret_file_template`

One Team-scoped reusable rendering.

| Field | Meaning |
|---|---|
| `id`, `public_id`, `team_id` | Identity and ownership |
| `name` | Team-unique display name |
| `target_path` | Path relative to the run's `HOME` |
| `mode` | Permission bits, constrained to non-executable |
| `parameters` | Declared parameter names, and which are required |
| `literals` | Non-secret parameter values shared by every Agent using this template |
| `template` | Text template substituting the resolved parameters |
| `created_by`, timestamps | Attribution |

A template declares parameters by name and says nothing about where their values
come from; §5.5 wires them. `literals` is where a cluster's server URL and CA
certificate belong — Team-scoped, readable, written once, and shared by every
Agent that renders against that cluster.

BuildMax ships built-in templates of exactly this shape, so the shipped list is
a default rather than a ceiling and a Team meeting a credential family BuildMax
has not heard of writes its own:

| Template | Parameters |
|---|---|
| `aws_credentials` | `access_key_id`, `secret_access_key`, `session_token`, `region` |
| `kubeconfig` | `server`, `certificate_authority_data`, `token` |
| `git_credentials` | `host`, `username`, `password` |
| `netrc` | `machine`, `login`, `password` |
| `npmrc` | `registry`, `auth_token` |
| `docker_config` | `registry`, `username`, `password` |

§8.3's write constraints are load-bearing and belong to this record, because
`target_path` and `mode` live here: a template that can write an executable or a
shell-startup file injects code into the run instead of delivering a credential.

### 5.5 `agent_file_render` And `agent_file_render_param`

One Agent revision's declaration that a file is rendered, and how each parameter
is satisfied.

| Field | Meaning |
|---|---|
| `agent_revision_id` | Immutable Agent definition that declares the rendering |
| `template_id` | The Team template or built-in to render |
| `required` | Whether the run fails when the file cannot be produced |
| `created_by`, `created_at`, `revoked_at` | Administrative lifecycle |

Each parameter is one row:

| Field | Meaning |
|---|---|
| `render_id`, `param_name` | Which rendering, which declared parameter |
| `secret_id`, `entry_name` | The group entry supplying it, when the source is a Secret |
| `literal` | The value supplying it, when the source is not a Secret |
| `version_mode` | Follow `current`, or pin an exact logical version |

A parameter resolves by name from three sources, in order: this declaration's
row, the template's `literals`, then nothing. A declaration literal overrides a
template literal, which is how one Agent uses a shared cluster template against
a different region without copying the template.

A write is valid only when every required parameter of the template is
satisfied, every referenced Secret belongs to the Agent's Team, every referenced
`entry_name` appears in the resolved version's `entry_names`, and the caller
holds the required Team permission. This replaces the earlier rule that a file
binding was valid when "the Secret's `kind` has a renderer" — the question is
now about the parameters a template actually declares, which is checkable.

### 5.6 `task_run_secret`

One non-secret snapshot of a run grant.

| Field | Meaning |
|---|---|
| `task_run_id` | The run |
| `source` | The `agent_secret_env` or `agent_file_render_param` row that produced it |
| `secret_id`, `secret_version_id`, `entry_name` | Exactly which entry of which version resolved |
| `provider_version` | Exact external version resolved, if available |
| `delivery` | `env` or `file` |
| `env_name`, `file_target` | Resolved delivery target |
| `status` | `pending`, `materialized`, `revoked`, `expired`, or `failed` |
| `expires_at` | When an exchanged or leased credential stops working |
| `materialized_at`, `revoked_at` | Runtime lifecycle evidence |

It holds no ciphertext, plaintext, lease token, provider error body, or hash of
a value. A hash would enable offline guessing for a low-entropy Secret and is
not needed to explain the run.

### 5.7 Credential Classes Kept Apart

One storage and delivery mechanism must not absorb credentials whose lifecycle
is already different.

| Class | Examples | Owner | Handling |
|---|---|---|---|
| Deployment bootstrap | database password, JWT signing key, Secret KEK | operator | deployment injection; never a Team Secret |
| Server-managed upstream | model API key, object-store administration credential | operator | encrypted store or external reference; never delivered to a Team's run |
| Team execution | GitHub token, Slack token, internal service credential | Team | this design |
| Ephemeral run authority | run token, presigned URL, STS credential, Vault lease | Server or external issuer | minted or exchanged at run time; short TTL; not a reusable Team Secret |
| User authentication | password verifier, refresh token, webhook key | account subsystem | existing hash, rotation, and revocation models |

## 6. Declaration And Binding

### 6.1 Why The Declaration Belongs To An Agent Revision

Agent revisions are append-only and already answer which instructions and
selections a run used, so declaring on one gives the authorization a stable
definition without putting mutable Secret names into task input. It is also the
narrowest scope available: two Agents in a Team may legitimately use different
accounts for the same service, and a run with no Agent receives no grants at
all.

An Agent edit that changes declarations creates a new revision. Replacing the
values behind a Secret does not — that is rotation, not an Agent behavior
change. The TaskRun snapshot joins the two histories.

### 6.2 What An Agent Declares

An Agent declares, independently:

- zero or more `agent_secret_env` rows (§5.3) — a named entry under a chosen
  variable name, or a whole group under its own entry names; and
- zero or more `agent_file_render` rows (§5.5) — a template, with each of its
  parameters wired to a Secret entry or a literal.

The two are separate records rather than modes of one, because their arity
differs. A variable holds one value. A file often needs several values plus
non-secret configuration, and that shape is what §5.4's parameters express.

Nothing stops the same entry feeding both. A token is useful as `GH_TOKEN` and
as the `password` parameter of a `git_credentials` rendering at the same time,
and the two reach different tools. Both resolve from the same version in a given
run.

This is a property of how the Agent works — one driving `kubectl` needs a
rendered `~/.kube/config`, one calling an internal HTTP API needs a variable —
so it belongs beside the Agent's other run-shaping configuration rather than in
Team-wide Secret settings.

### 6.3 Missing And Optional Grants

A `required` declaration whose grant cannot be produced fails the run before the
Agent starts, naming the Secret's display name, the entry or parameter, and the
reason — never a value, locator, or provider response. A declaration that is not
required is skipped, and the run records that it was skipped.

A background run whose declared authenticated capability cannot start fails
rather than continuing without it, matching plugin materialization. Silent
degradation produces an output that does not reflect the Agent definition.

## 7. Run Lifecycle

```text
Team Owner                 Server                         Worker
    |                         |                              |
    |-- write/rotate Secret -->| encrypt or record reference |
    |-- declare Agent grants ->| validate same-Team ownership|
    |                         |                              |
    |                    dispatch TaskRun                    |
    |                         |-- resolve Agent revision     |
    |                         |-- resolve declared bindings  |
    |                         |-- snapshot task_run_secret   |
    |                         |                              |
    |                         |<---- claim with run token ---|
    |                         |-- require live matching run  |
    |                         |-- resolve exact versions     |
    |                         |-- exchange short-lived creds |
    |                         |-- return computed bundle --->|
    |                         |                              |-- set declared env grants
    |                         |                              |-- render declared files
    |                         |                              |
    |                         |<---- report terminal state --|
    |                         |-- revoke leases, close grant |
```

The snapshot happens where the Agent revision and plugin pins are already
resolved: while the worker claims the run. Resolution never uses values the
worker supplied, and a worker cannot browse Team state.

The worker-facing operation returns the bundle computed for one TaskRun. It
accepts no Secret ID, name, provider path, or version selector. Its route is
registered in `internal/server/handlers/routes.go` and described in
`internal/server/static/openapi.json` when built.

Materialization requires all of: a valid run token whose run matches the path;
a non-terminal run in the expected execution state; the stored grant set; an
enabled Secret and non-destroyed version; a binding still valid for the pinned
Agent revision; and a configured, healthy backend.

The response uses TLS, sets `Cache-Control: no-store`, and bypasses body
logging. Retrying a failed fetch is allowed — "read exactly once" is not useful
if a lost response permanently fails an otherwise valid run — and every
successful materialization is recorded.

## 8. Delivery Modes

Two modes. An Agent revision declares each independently; §6.2 says how.

### 8.1 Prerequisite: A Run-Scoped `HOME`

Neither mode is well-defined until the run owns its `HOME`. A worker receives a
run-scoped `BUILDMAX_HOME` (`taskrun.RuntimePaths.RuntimeTaskRunHomeDir`) but no
run-scoped operating-system `HOME`; it inherits whatever the container image
sets, which is shared across runs.

That changes first, for two reasons: a rendered credential file needs a private
location tools look in by default, and anything a tool writes to `~/.config`
today survives into unrelated runs. The run's `HOME` becomes the existing
per-run home directory, created empty and removed with the run.

### 8.2 Environment Variables

The runtime places each `env` grant into the environment of the commands the run
executes: a named entry under the variable name the Agent chose, or every entry
of a group under its own name with an optional prefix (§5.3).

This is the universal mode. It needs no cooperation from the tool, covers
non-HTTP protocols, and works for a program written during the run. It is the
fallback whenever a credential family has no file convention, and the first mode
to implement.

Two rules bound it:

- a grant is placed under the name the declaration resolved to and no other,
  never additionally exported under a guessed alias; and
- the run's environment is otherwise the deny-by-default baseline of §13.1, not
  the worker's inherited environment. Delivering declared grants is not a reason
  to stop withholding the deployment's own credentials.

### 8.3 Run-Scoped Credential Files

The runtime renders a template (§5.4) into a file under the run's `HOME`, in
the layout the credential family already defines, and tools find it themselves.
Each parameter is resolved from a Secret entry or a literal before the Agent
starts.

The unit is the credential family, not the tool — this is what keeps the work
bounded. One rendered `~/.aws/credentials` serves the AWS CLI, every language
SDK, and Terraform at once. One rendered `~/.git-credentials` with its
`~/.gitconfig` serves `git` and everything that shells out to `git`. The mode
exists for two distinct reasons:

- some families have no usable environment form at all — `~/.kube/config` and
  `~/.docker/config.json` carry structure a single variable cannot; and
- where an environment form does exist, the file form additionally reaches tools
  that only read configuration.

Write constraints, all mandatory:

| Constraint | Reason |
|---|---|
| Target resolves inside the run's `HOME` | A file under the workspace is one `git add -A` from being committed, and is a candidate for artifact upload |
| No `..` traversal, no symlink following | The same, by another route |
| Mode `0600`, never executable | A credential file is data |
| Refuse shell-startup and command-bearing targets — `.bashrc`, `.profile`, `.zshrc`, `git` config keys naming a command | Rendering into one of these injects code into the run rather than delivering a credential |
| Removed when the run ends | The run directory is removed anyway; this must not depend on that |

### 8.4 Using Both

An Agent may declare an entry as a variable and wire the same entry into a
template parameter. Both resolve from one version in a given run, and the
snapshot records each target separately. Where a family's file form references a
variable rather than embedding the value, the template uses that form.

## 9. Storage Backends

### 9.1 Embedded Encrypted Store

The default for an out-of-the-box private deployment. Only ciphertext and
wrapped data-encryption keys are stored in MySQL.

Each `secret_version` receives a fresh random DEK and nonce, and the value is
encrypted with AES-256-GCM or an equivalently reviewed AEAD. Associated data
binds the ciphertext to at least the deployment, Team public ID, Secret public
ID, and logical version, so moving a ciphertext row between owners fails
authentication.

The DEK is wrapped by a KEK, which never belongs in the database or
`server.yaml`. Its sources are a root-key file mounted read-only into the Server
process for the portable baseline, a cloud KMS using workload identity, or a
Vault transit key where a deployment already runs Vault.

The Server fails startup when encrypted data exists and its KEK is missing or
unusable. It must not generate a replacement key or treat values as empty.
Losing the KEK means losing the values, and backup documentation says so.

### 9.2 External Secret Reference

An external backend stores an encrypted descriptor naming a provider, a
provider-managed Secret, and a version selector. BuildMax does not copy the
value into its database. At resolution the Server authenticates with its
workload identity, retrieves the value or a dynamic credential, records the
exact provider version when available, and applies the same grant and delivery
rules as embedded mode.

Provider configuration is deployment-scoped and operator-managed. A Team cannot
submit an arbitrary Vault address or cloud endpoint that would send the Server's
provider identity elsewhere: the operator defines named providers, TLS roots,
regions, allowed path prefixes, and authentication methods, and a Team record
selects only among them.

Vault is the first external integration, because private deployment is a product
promise and Vault serves on-premises static and dynamic Secrets. AWS and Google
follow the same interface. Provider parity is not a first-slice requirement.

### 9.3 Short-Lived Credentials

Where a target supports it, the Server exchanges a stored long-lived credential
for a short-lived one at run start — a GitHub App installation token, an STS
`AssumeRole` result, a Vault lease — and delivers that instead.

This is not a convenience. Under §2.1's run-level delivery, the value is
reachable by the Agent, so lifetime and provider-side scope carry most of what
is left of the exfiltration control. A one-hour repository-scoped token that
leaks is a bounded incident; a stored organization-wide token that leaks is not.
An exchanged credential is preferred over a stored one whenever both are
possible, and §5.5's `expires_at` exists so a run and its operator can see when
it stops working.

Workload identity — a dedicated OIDC issuer letting a live TaskRun federate
directly with Vault, AWS, or Google — is the same idea without the stored
credential, and is the last phase rather than the first.

## 10. Authorization

The roles are `owner`, `admin`, and `member`, per
[team-governance.md](team-governance.md).

| Action | Owner | Admin | Member |
|---|---:|---:|---:|
| List Secret metadata and binding health | yes | yes | no |
| Create, replace, disable, or destroy a value | yes | no | no |
| Create or revoke an Agent Secret declaration, or a file template | yes | no | no |
| Save an Agent revision declaring a Secret the Team has not bound | yes | yes | no |
| Trigger an already authorized Agent run | yes | yes | yes |
| Read Secret audit events | yes | no | no |
| Reveal a value | no | no | no |

Members may indirectly use a credential by running an Agent whose owner
authorized it. That is necessary for shared automation, and under §3 it also
means the member can read the value. Both facts belong in the Portal surface;
neither may be implied away.

Value and binding authority stays with the owner until BuildMax has finer Team
grants. If operator evidence shows owners cannot be the operational Secret
managers, add an explicit `secret_manager` grant rather than quietly widening
`admin`.

This design does not depend on the unbuilt approval system. A future high-risk
or environment approval can gate materialization without changing the stored
model.

## 11. Audit And Provenance

Audit actions: `secret.created`, `secret.rotated`, `secret.disabled`,
`secret.destroyed`, `secret.declaration_created`, `secret.declaration_revoked`,
`secret.materialized`, `secret.revoked`, and `secret.access_denied`.

An event names the actor, Team, Secret public ID, binding or TaskRun, action,
and a bounded non-sensitive detail such as the delivery mode and target name. It
never carries plaintext or ciphertext, a hash of plaintext, a provider token,
lease ID, response body, or full path, an HTTP header or environment value, or
prompt, tool output, command arguments, or file contents.

The existing audit table can represent a materialization with the Secret as
target and the TaskRun as bounded detail. If volume or query needs make that
awkward, a dedicated append-only access ledger is the alternative. Two
partially overlapping trails would be worse than either, so exactly one is
authoritative for "which runs materialized this Secret?"

Trace provenance records handles, binding IDs, delivery modes, target names,
and materialization outcomes. It records no provider locator and no value.

## 12. Redaction

Every materialized static value is registered with a per-run exact-value
redactor before the Agent starts, covering application logs, worker status
errors, tool results before they enter model context, streamed output, and
durable traces.

Exact-value redaction ignores empty and very short values, so ordinary output is
not replaced everywhere, and uses a bounded representation so a large
certificate does not make every trace operation unbounded. The existing
shape-based redaction stays for credentials that did not come through the
Broker.

Under run-level delivery this is mitigation against **accident**, not against
the Agent. A run holding a value can encode, split, transform, or transmit it,
and a model told to print a credential will succeed. The purpose is to keep a
value from drifting into a durable artifact nobody intended it to reach — a
trace shipped to an operator, a tool result cached in a conversation. Every
description of it says so.

Artifact upload stays out. Rewriting arbitrary artifacts can corrupt outputs and
declaring every binary safe would be false, so this design claims no artifact
DLP. Exact-match scanning of bounded text previews, with a warning or
quarantine, is available later as its own measured decision.

## 13. Existing Credential Debt

Adding Team Secrets without removing broader deployment credentials from workers
would produce a narrow new door beside an open old one. Under run-level delivery
this matters more, not less: the run is now expected to hold its own grants, so
everything else it holds should be there deliberately.

### 13.1 The Sandbox Environment Denylist

`internal/infra/sandbox/env_scrub.go` strips secret-shaped variables from any
sandboxed child — an exact list including `GITHUB_TOKEN` and `GH_TOKEN`, plus a
suffix rule matching `_TOKEN`, `_KEY`, `_SECRET`, `_PASSWORD`, `_PASSWD`, and
`_PWD`. `Bash.childEnv` applies it whenever the sandbox is active, which on the
worker baseline is always.

It is the right instinct at the wrong altitude, and it blocks this design
outright: a Team's declared `GH_TOKEN` grant never reaches the shell, and
because the sandbox defaults off on the CLI baseline and on for workers, the
same Agent configuration works locally and fails silently in a pod. The suffix
rule also swallows any similarly named variable an operator legitimately set.

It becomes a policy:

- deny by default, so the worker's inherited environment does not reach
  model-chosen commands;
- always deny BuildMax's own credentials — `BUILDMAX_RUN_TOKEN`,
  `BUILDMAX_JWT_SECRET`, `BUILDMAX_API_KEY` — which are never Team Secrets and
  are never legitimately granted to a run; and
- allow exactly the names this run's grants declared.

Deleting the file is not the fix. The run token is a live credential for the
managed inference gateway and the worker routes; letting it into every Bash
child would be a regression this design must not pay for.

### 13.2 Object Storage

Workers receive long-lived object-store credentials. The replacement is
Server-mediated object transfer, run-prefix presigned requests, or cloud
workload identity — respectively costing Server bandwidth, request
orchestration, or portability on MinIO deployments. Artifact upload already goes
through a run-scoped Server route; workspace and run-state transfer decide
whether the remaining credential can be removed entirely. This is prerequisite
work, not something the Team Secret feature absorbs.

### 13.3 Direct Model Credentials

Managed inference is the recommended cloud-worker mode because the provider key
and upstream details stay on the Server. Direct mode may remain for trusted
local execution, but a cloud worker should not receive a deployment-wide
provider key by default. The plaintext `llm_model.api_key` migrates to the
encrypted backend of §9.1 or an external operator reference. It stays
deployment-scoped and Server-only; it does not become a Team Secret.

### 13.4 Run Token Delivery

The worker clears `BUILDMAX_RUN_TOKEN` from its environment after reading it,
but Kubernetes keeps the value in the Job specification
(`internal/infra/k8s/job.go`). A per-run immutable Kubernetes Secret mounted as
a file, with an owner reference and restrictive RBAC, removes it from the Job
environment and lets garbage collection follow the Job. This is defense in
depth: the run token stays run-scoped, expires, and is refused by terminal-state
checks either way.

## 14. Failure Semantics

| Failure | Run behavior |
|---|---|
| Required declaration unsatisfiable | Fail before the Agent starts, naming the Secret and the entry or parameter |
| Secret disabled or destroyed | Fail before materialization |
| File render refused by a write constraint | Fail before the Agent starts, naming the constraint; do not fall back to environment delivery |
| Embedded KEK unavailable | Server health degraded; refuse affected materialization without treating the value as empty |
| External provider unavailable | Retry within a bounded startup budget, then fail with a sanitized provider-class error |
| Run token invalid or run terminal | Refuse without revealing whether a Secret exists outside the run grant |
| Exchanged credential or lease expires mid-run | Renew where the provider allows it; otherwise fail rather than continue as unauthenticated work |
| Revocation fails after terminal state | Preserve the terminal outcome, record a sanitized revocation failure, retry out of band within a bound |

## 15. API Shape

User-facing operations are conventional resource APIs that are write-only for
values: list and get Secret metadata; create a Secret with its first value or
external reference; replace the value to create a version; disable, re-enable,
and destroy; list version metadata without values; create and revoke Agent
revision declarations; create, edit, and delete file templates; and inspect
declaration health for a revision, which reports a named entry that no longer
exists in the current version and a template parameter nothing satisfies.

No response includes a value. Create and replace return metadata and the new
version number only. Request fields carrying values are excluded from request
logging, validation errors, and audit details.

The worker-facing surface returns only the grant set already computed for its
run — no list, get-by-name, provider lookup, or version selection.

Route strings live in `internal/server/handlers/routes.go`, and
`internal/server/static/openapi.json` must describe them, including the absence
of a reveal operation.

## 16. Package Boundaries

| Area | Responsibility |
|---|---|
| `internal/core/model` | Secret metadata, entry-set versions, declarations, templates, run grants, errors, and narrow store interfaces |
| `internal/service/secret` | Lifecycle rules, declaration validation, parameter resolution, materialization, exchange, and revocation |
| `internal/infra/secret` | AEAD/envelope implementation, external provider adapters, and credential-exchange clients |
| `internal/infra/db` | Row structs and metadata/ciphertext persistence; no provider calls |
| `internal/server/handlers` | User and worker authentication, Team authorization, request/response shaping |
| `internal/agentapp/taskrun` | Consume an authorized in-memory grant set, place declared environment grants, render declared files into the run's `HOME` |
| `internal/infra/sandbox` | Apply the §13.1 deny-by-default environment policy and admit exactly this run's declared names |

`internal/core` imports no configuration, cryptography provider,
infrastructure, GORM, Server code, or Agent application assembly. Configuration
selects provider implementations during bootstrap; it does not resolve Team
resources. The Secret service takes a small KEK/provider interface so embedded,
Vault, and cloud implementations do not leak into handlers or the runtime.

## 17. Alternatives Rejected

### Per-Consumer Injection Only

Deliver values only into named processes — a stdio MCP server's environment, a
typed HTTP authorization slot, a file handed to one consumer — and refuse
delivery to model-chosen commands.

It is the only rejected option that would have kept a value out of the general
run environment, and it would have made a release digest an enforceable
delivery condition. It is rejected because it reaches only the consumers
BuildMax has adapted, which is the wrong side of an unbounded set: the Agent's
central capability is invoking tools it selects at run time, and under this
model those receive nothing. It remains the right shape for a future consumer
that genuinely is one named process, and §5.3's removed fields return with it.

### Credential Helpers And `PATH` Wrappers

Register a credential helper, an AWS `credential_process`, or a `kubectl` exec
plugin pointing at a BuildMax broker; or place same-name wrapper binaries on the
run's `PATH` so a token reaches only the child.

Both are per-tool adaptation against an unbounded set of tools, for the same
reason as above. A version-control credential helper configured through a
run-scoped configuration file is a special case that costs nothing, because it
is one line of generated configuration rather than an adapter — where that is
true it belongs under §8.3's file rendering, not as a mechanism of its own.

### Credential Injection At The Sandbox Egress Proxy

Terminate TLS on the existing loopback proxy (`internal/infra/sandbox/proxy.go`),
attach an `Authorization` header per target host, and re-establish TLS upstream.
The value would stay in worker memory and never enter the run.

This is the only evaluated option that would change §3's honest statement: the
Agent could exercise the authority without ever holding the value, and because
the proxy sees method and path, request-level policy would become possible. It
also scales per target host rather than per tool.

It is rejected on cost, not merit. It requires a run-scoped CA and a deliberate
man-in-the-middle inside the run; certificate-pinning tools break; the CA must
be installed per language runtime, which is its own list; and it does nothing
for non-HTTP protocols. It is recorded here because the pieces it needs — the
loopback proxy, `HostMatcher`'s per-host patterns, and `Manager.ChildEnv`'s
proxy routing — already exist, so revisiting it later is a smaller step than it
looks.

### A `kind` Field On The Secret

Tag each Secret with a credential family — `github_token`, `kubeconfig` — and
use it to pick a default file renderer.

Rejected on two counts. It duplicated a decision §5.5 already owns, leaving a
validation rule that depended on a field existing only to supply someone else's
default. Worse, it asserted that a Secret's value *is* one whole rendered file,
which sealed a kubeconfig's non-secret server URL and CA certificate inside an
encrypted blob no operator could read or diff. Entry names carry whatever
structure a credential has, and the template is chosen where it is used.

### A Secret-Or-Config Flag On Each Entry

Mark every entry as write-only or as readable metadata, so non-secret
configuration could live beside the credential and still be visible — the split
Kubernetes draws between Secret and ConfigMap, and GitHub Actions between
secrets and variables.

Rejected because it charges a per-entry classification to every Team on every
write to serve a case that placement already answers. A group holds what should
be write-only; §5.4's template literals hold what a cluster shares and should be
readable; §5.5's declaration literals hold what one Agent varies. Each piece of
configuration is visible in the place that owns it, and creating a Secret stays
nothing but typing names and values.

### Deployment Environment Variables Only

Keep the current model and ask operators to put every credential in the worker
environment. No new database, crypto, UI, or provider code — and no Team scope,
no per-Agent account choice, no rotation, and no usable audit. Every run of
every Agent gets every value.

### A Deployment-Global Secret Scope

See §4. Refused because a global value has no owner to attribute it to, no Team
to revoke it from, and no answer to which Teams' runs may read it.

### External Secret Managers Only

Requiring Vault or a cloud provider, with the worker retrieving values through a
workload identity, gives strong provider lifecycle and no BuildMax value
storage. It violates the out-of-the-box private-deployment promise, differs in
authorization per provider, and hands workers provider lookup ability unless
carefully brokered. External backends belong under §9.2, not instead of it.

## 18. Phases

### Phase 0 — Unblock And Isolate The Run

- replace the `env_scrub` denylist with §13.1's deny-by-default,
  allow-declared policy, keeping BuildMax's own credentials denied;
- give each run its own operating-system `HOME`, created empty and removed with
  the run;
- replace deployment-wide object-store credentials with Server-mediated or
  run-scoped access (§13.2);
- make managed inference the cloud-worker default (§13.3); and
- move Kubernetes run-token delivery out of the Job environment (§13.4).

The first two are prerequisites for any delivery at all. The rest must be
explicit before BuildMax claims a worker holds only what its run needs.

### Phase 1 — Embedded Team Secrets With Environment Delivery

- the `secret`, `secret_version`, `agent_secret_env`, and `task_run_secret`
  rows, with the entry set versioned as a unit;
- envelope encryption with a portable mounted KEK;
- owner-only create, replace, disable, destroy, and binding operations;
- `env` declaration on an Agent revision, and delivery into the run;
- Portal metadata, rotation, and binding-health surfaces, carrying §3's two
  consequences in the copy;
- audit actions and per-run exact-value redaction; and
- failure before the Agent starts on a missing or unusable required grant.

Environment delivery is first because it is universal and needs no template.

### Phase 2 — Credential File Delivery

- `secret_file_template`, `agent_file_render`, and `agent_file_render_param`,
  with three-source parameter resolution;
- the built-in templates of §5.4, beginning with the families that have no
  usable environment form — `kubeconfig`, container-registry credentials — then
  those where a file additionally helps: version control, AWS, npm;
- every §8.3 write constraint, including refusal of shell-startup and
  command-bearing targets; and
- one entry usable as a variable and a template parameter at once.

### Phase 3 — Short-Lived Credential Exchange

- exchange at run start where the target supports it: installation tokens, STS
  `AssumeRole`, Vault leases;
- `expires_at` on the run grant, surfaced;
- renewal behavior for runs that outlive the credential; and
- prefer an exchanged credential over a stored one whenever both are possible.

Placed before external providers deliberately: under run-level delivery,
credential lifetime carries more of this design's safety than provider breadth
does.

### Phase 4 — External Provider References

Operator-configured provider records and path ceilings; Vault first, then
evidence-driven AWS and Google; exact provider versions and access outcomes
recorded; dynamic leases where exposed; backup, outage, and rotation operations
documented.

### Phase 5 — Workload Identity

A dedicated OIDC issuer with rotating signing keys and JWKS; immutable TaskRun
subject and audience claims; exchange for Vault, AWS, or Google short-lived
authority; stored static cloud credentials removed from supported paths.

## 19. Acceptance Evidence

Phase 1 does not ship until these hold:

1. a database backup alone cannot recover a value;
2. no user-facing operation reveals a value;
3. a worker cannot obtain a Secret outside its stored TaskRun grants, and a run
   receives nothing its Agent revision did not declare;
4. a declaration referencing another Team's Secret is refused at write time,
   as is one naming an entry the current version does not have;
5. rotating a multi-entry Secret is atomic: no run resolves one entry from one
   version and another entry from the next;
6. the worker's inherited environment does not reach model-chosen commands, and
   BuildMax's own credentials are never delivered as grants;
7. rotation affects the next run rather than mutating one in flight;
8. a terminal run cannot materialize again;
9. audit and trace metadata explain the grant without containing credential
   material; and
10. every surface describing the feature states that the run can read what it
    was granted.

Supporting work: a schema and crypto review covering nonce generation, AEAD
associated data, DEK wrapping, KEK loss, rotation, and destruction; an
operational drill restoring an encrypted backup with the correct KEK and failing
safely without it; a rendered-file test proving every write constraint,
including refusal of a template targeting a shell-startup file or a path outside
the run's `HOME`; cross-Team and cross-run authorization matrix tests;
failure-injection for KMS and Vault timeouts, provider denial, lease expiry, and
redaction paths; and latency measurement for Secret and object-storage
brokerage so the boundary does not create an unexamined bottleneck.

Because these rows live in `internal/infra/db`, the scope needs
`./make test mysql` with a real DSN; a green `./make test` says nothing about
them.

## 20. Open Questions

1. May an `admin` see Secret metadata and binding health without gaining value
   or binding authority, or is listing owner-only?
2. Is an external-provider locator encrypted with the value, or is
   database-visible provider metadata necessary for operation and audit?
3. Does the embedded KEK baseline accept an environment value for Compose
   convenience, or a mounted file only?
4. Which object-storage replacement preserves large-file performance across
   local-process, Compose, and Kubernetes deployments?
5. Does a materialization append to the existing audit trail, a dedicated access
   ledger, or both with one explicitly derived from the other?
6. How is a lease renewed when the Agent loop is blocked in a long tool call,
   and what happens when renewal cannot complete?
7. Which OIDC claims are stable and useful to Vault, AWS, and GCP without
   exposing mutable names?
8. Does a Secret version stay decryptable while a queued or retryable run
   references it, and what retention stops an abandoned run from blocking
   destruction?
9. What artifact behavior is honest when a credential-bearing process writes the
   value, or a transformed form of it, into an output file?
10. Are Team-defined file templates worth their write-path risk in Phase 2, or
    should it ship built-in templates only until a deployment names a family
    BuildMax does not cover?
11. Does the run's environment baseline need an operator escape hatch for
    deployments that legitimately pass ambient configuration into runs, or does
    declaring a grant cover every real case?
12. May an Agent declare a Secret requirement the Team has not yet bound, so
    Portal can show the gap, or must declaration and binding be written
    together?
