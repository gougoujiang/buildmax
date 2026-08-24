# Run-Scoped Secret Broker And Workload Identity

> **Audience:** contributors, operators, plugin publishers, and security reviewers · **Status:** proposal — under discussion
>
> **Opened:** 2026-08-24

Related: [../ROADMAP.md](../ROADMAP.md) P0.5, P3, and P4; [worker run
token](../design/worker-run-token.md); [team and worker plugin
distribution](../design/plugin-team-distribution.md); [sandbox
boundaries](../design/sandbox-boundaries.md); [durable run
trace](../design/durable-run-trace.md); [team
governance](../design/team-governance.md); [managed LLM
gateway](../design/llm-gateway.md); and [configuration
reference](../reference/configuration.md).

## Problem

BuildMax is a cloud execution platform whose task-run workers execute
model-chosen commands and plugin processes on behalf of a Team. Useful plugins
often need credentials: an MCP server may need a GitHub installation token, a
hook may need an internal API credential, and a future deployment agent may
need short-lived cloud authority.

There is no Team-scoped Secret resource today. The plugin distribution design
deliberately stops at reporting the environment variable names a release reads.
It does not decide who may supply their values, where those values live, or how
a run receives them. A plugin that needs a credential therefore fails in a
worker unless the deployment happens to expose the value through the worker's
ambient environment.

That ambient environment is already too authoritative:

- a worker still receives the deployment's long-lived object-store credential;
- a direct-inference worker receives a provider API key;
- the run token is placed in a Kubernetes Job's environment and therefore in
  the readable Job specification until the Job is removed; and
- managed model credentials are kept in plaintext in the `llm_model` table,
  although general catalog reads deliberately exclude the column.

BuildMax has the beginnings of the right boundary. Every worker route accepts a
run-scoped token rather than a deployment-wide worker credential. Managed
inference keeps an upstream endpoint, model identifier, and provider credential
on the Server. Plugin releases and Agent definitions are pinned to a TaskRun.
Each worker starts with a run-scoped `BUILDMAX_HOME`, and trace output is
bounded and redacted.

The missing decision is how to extend that boundary to credentials without
turning a Team into a bag of environment variables available to every process a
model can start.

## Decision Sought

Should BuildMax introduce a Server-side Secret Broker that:

1. represents a Secret as a Team-owned, versioned resource or as a reference to
   an external secret manager;
2. binds its use to an immutable Agent revision, an exact plugin release, and a
   declared consumer name;
3. snapshots that authorization onto one TaskRun;
4. materializes only the values computed for that run; and
5. injects each value into its named consumer rather than the worker's ambient
   environment?

The recommended answer in this proposal is yes. The embedded store is the
out-of-the-box path for a small private deployment. Vault and cloud secret
managers remain the system of record in larger deployments, with BuildMax
providing the common authorization, binding, materialization, and audit model.

This is a proposal, not current behavior or committed roadmap work.

## Goals

- Let a Team run authenticated plugins without copying credentials into a
  repository, Agent instruction, task prompt, workspace, or deployment-wide
  worker environment.
- Preserve Team as the ownership and authorization boundary while narrowing
  each use to one Agent revision, plugin release, TaskRun, and consumer.
- Keep credential values write-only in user-facing APIs: create, replace,
  disable, and destroy, but never reveal.
- Give small private deployments an embedded encrypted store without requiring
  Vault or one particular cloud.
- Let enterprise deployments keep values in Vault, AWS Secrets Manager, Google
  Cloud Secret Manager, or a later provider rather than copying them into
  BuildMax.
- Prefer workload identity and short-lived provider credentials over stored
  long-lived credentials when the target supports federation.
- Make every administrative change and runtime materialization attributable
  without putting a value, provider response, or sensitive locator in the audit
  trail.
- Keep the Agent Core free of storage, encryption, provider, HTTP, and GORM
  dependencies.
- Make rotation semantics deterministic: a run records the exact Secret version
  or provider version it resolved, and an in-flight run does not change beneath
  itself.
- State which attackers the design does not defend against.

## Non-Goals

- Reimplementing the full policy language, dynamic-secret catalog, PKI engine,
  replication, or disaster-recovery behavior of Vault.
- Protecting a value from a deployment operator who controls the BuildMax
  process, its memory, its root encryption key, or the external provider
  identity. An external KMS with separation of duties can narrow that trust; the
  embedded mode cannot remove it.
- Making a malicious executable safe after it has legitimately received a
  credential. Code with a value and network access can transmit it.
- Treating log masking as data-loss prevention. Exact values can be encoded,
  split, transformed, encrypted, or sent directly over the network.
- Passing arbitrary credentials to model-chosen Bash commands in the first
  slice.
- Storing user login passwords, refresh tokens, webhook keys, or the run token
  in the Team Secret model. Those credentials have their own lifecycle and
  verification contracts.
- Automatically synchronizing local CLI or Desktop environment variables into
  the Server. A local credential remains local unless a user explicitly creates
  a Team Secret.
- Adding a Server-side Project or deployment-environment entity only to scope
  Secrets. Team is the current Server ownership boundary; Desktop Project is
  local UI state.
- Depending on the planned approval system. The first authorization model must
  be safe with the roles that exist today.

## Evidence From Existing Platforms

The products differ in interface, but the useful common decisions are stable:

| Platform | Relevant behavior | Lesson for BuildMax |
|---|---|---|
| [GitHub Actions](https://docs.github.com/en/actions/concepts/security/secrets) | Organization, repository, and environment scopes; a workflow explicitly maps a Secret into a job; environment protection may gate access; OIDC is preferred for supported cloud providers | Ownership alone does not imply delivery; the run must name the capability it needs |
| [GitLab CI/CD](https://docs.gitlab.com/ci/secrets/) | A job explicitly requests an external Secret; supported providers authenticate with an ID token; file injection is available; ordinary CI variables and masking carry weaker guarantees | External providers and workload identity should share the same consumer model as embedded values |
| [Modal](https://modal.com/docs/guide/secrets) | A named Secret or Secret bundle is explicitly attached to a Function or Sandbox and isolated by an Environment | The common case should remain simple even when the underlying authorization record is precise |
| [Kubernetes](https://kubernetes.io/docs/concepts/security/secrets-good-practices/) | RBAC and namespaces limit reads, but a principal able to create a Pod that consumes a Secret can cause that Pod to reveal it; short-lived credentials are recommended | “Cannot call the read API” is not the same as “cannot obtain the value through execution” |
| [Vault Agent Injector](https://developer.hashicorp.com/vault/docs/deploy/kubernetes/injector) | A workload identity obtains static or dynamic Secrets and renders them into a memory-backed shared volume; leases can be renewed and revoked | The workload should authenticate as itself, and lifecycle management belongs beside materialization |
| [AWS Secrets Manager for EKS](https://docs.aws.amazon.com/eks/latest/userguide/manage-secrets.html) | Pod identity is exchanged for a role and authorized values are mounted through a CSI provider; rotation can refresh the mount | A cloud credential should be derived from the run identity rather than stored as a Team value when possible |
| [Google Secret Manager](https://docs.cloud.google.com/secret-manager/docs/best-practices) | IAM, workload identity, version references, data-access logs, and environment separation are recommended | BuildMax should record the provider version and make provider access an auditable workload action |

The comparison supports five constraints:

1. delivery is explicit;
2. scope is narrower than ownership;
3. values are not recoverable through normal management APIs;
4. workload identity is preferable to long-lived shared credentials; and
5. masking is defense in depth, not the confidentiality boundary.

## Threat And Trust Boundaries

### Assets

The protected assets are:

- static Secret values and their old versions;
- external Secret locators when the locator itself discloses account,
  environment, or infrastructure structure;
- the key-encryption key and external-provider workload identity;
- run-scoped and provider-issued credentials;
- the authorization record explaining which code was allowed to receive which
  value; and
- audit evidence of administration, materialization, denial, and destruction.

### Defended Cases

The candidate design should defend against:

- a database or backup disclosure without the key-encryption key;
- one Team reading or materializing another Team's values;
- one run using its valid token to request a different run's grants;
- a worker choosing an arbitrary Secret from its Team;
- a plugin receiving a credential it did not declare;
- a newly pinned plugin release silently inheriting a credential approved for
  older code;
- model-chosen Bash and unrelated child processes reading the worker's
  credentials through `printenv` or `/proc`;
- values accidentally appearing in structured logs, traces, streamed output,
  MCP errors, or tool results; and
- a terminal run continuing to fetch or spend credentials.

### Explicitly Trusted

The following remain trusted:

- the deployment operator in embedded mode;
- the Server process while it resolves, decrypts, and transmits a value;
- the external Secret provider and its administrator;
- the exact plugin release that is approved to receive a value; and
- the service represented by the credential to enforce the credential's own
  scope.

Operator eligibility, plugin inspection, release digest pinning, and network
policy reduce the chance and effect of trusting the wrong code. They do not
make a credential-bearing executable untrusted code.

### Confused Deputy Risk

Keeping a credential out of the model context prevents direct disclosure but
does not prevent an Agent from asking an authenticated MCP tool to perform an
authorized action. That is a confused-deputy problem, not a Secret-storage
problem. Tool permissions, MCP read-only annotations, hook policy, service-side
least privilege, and future autonomous-surface approvals remain necessary.

## Design Principles

### A Secret Is A Capability, Not Configuration

Non-secret configuration can be copied, rendered, inspected, diffed, cached,
and included in diagnostics. A Secret cannot safely inherit those behaviors.
The public model therefore carries a handle and metadata, never a value.

### Ownership, Authorization, And Materialization Are Separate

- A Team owns a Secret.
- An Agent revision binding authorizes a declared plugin consumer to use it.
- A TaskRun snapshot decides the exact authorized version.
- A materialization is the runtime event that releases plaintext or obtains a
  dynamic credential.

No one of those records substitutes for the others.

### The Server Resolves; The Worker Consumes

The worker does not list Team Secrets, query an external provider by arbitrary
path, or choose which version is current. The Server performs those decisions
using Team state, the resolved Agent revision, plugin pins, run status, and the
run token's claims. The worker receives a finished grant set, mirroring the
existing plugin-materialization boundary.

### Ambient Environment Is Not An Authorization Boundary

The worker process environment is visible to the process and, depending on the
platform and sandbox state, to children it launches. A model can ask Bash to
print it. Secret delivery therefore targets a specific process, request field,
or memory-backed file rather than calling `os.Setenv` or adding values to the
Job environment.

### A New Release Is New Code

A plugin activation pins a version and digest. A binding approved for that
digest does not automatically follow a pin move. Carrying a credential into new
code is a separate decision even when the manifest continues to declare the
same environment variable.

### Runs Are Reproducible; Credentials Are Rotatable

A binding may follow the current version of a logical Secret. The Server
resolves that selector when the TaskRun is claimed and records the exact
version. Rotation affects later runs, not one already executing.

## Credential Classes

One storage and injection mechanism should not absorb credentials whose
lifecycle is already different.

| Class | Examples | Owner | Consumer | Candidate handling |
|---|---|---|---|---|
| Deployment bootstrap | database password, JWT signing key, Secret KEK | operator | Server at startup | deployment injection; never a Portal Team Secret |
| Server-managed upstream | model API key, object-store administration credential | operator | one Server subsystem | encrypted store or external reference; never sent to Team workers |
| Team execution | GitHub token, Slack token, internal service credential | Team | an exact plugin consumer | Secret Broker binding and run grant |
| Ephemeral run authority | BuildMax run token, presigned object URL, STS credential, Vault lease | Server or external issuer | one TaskRun or plugin consumer | mint or exchange at runtime; short TTL; not a reusable Team Secret |
| User authentication | password verifier, refresh token, webhook key | user/account subsystem | Server verification | keep the existing hash, rotation, and revocation models |

The first feature slice concerns Team execution credentials. The design must
also remove deployment credentials from workers and migrate plaintext managed
model credentials so the new surface does not coexist with a broader, older
leak path indefinitely.

## Candidate Resource Model

The persisted row structs remain the eventual schema source of truth. The names
and fields below are proposed shapes, not current tables.

### `secret`

One Team-owned logical value:

| Field | Meaning |
|---|---|
| `id`, `public_id` | Numeric relational key and opaque public handle |
| `team_id` | Ownership and authorization boundary |
| `name` | Team-unique, non-secret display name |
| `description` | Optional bounded explanation; must not carry a value or locator |
| `provider` | `embedded` or an operator-configured external provider name |
| `state` | `active`, `disabled`, or `destroyed` |
| `current_version_id` | The version a following binding resolves for the next run |
| `created_by`, timestamps | Administrative attribution |

`(team_id, name)` is unique. Renaming changes display metadata, not identity.
Disabling refuses new run grants and new materializations. Destruction erases
recoverable version material after active references are gone; it does not
rewrite audit history.

### `secret_version`

One immutable write of an embedded value or external provider descriptor:

| Field | Meaning |
|---|---|
| `id`, `secret_id`, `version` | Identity and monotonically increasing logical version |
| `ciphertext`, `nonce` | AEAD-encrypted value or encrypted provider descriptor |
| `wrapped_dek`, `key_id` | Envelope-encryption metadata |
| `provider_version` | Exact external version when one is selected rather than followed |
| `created_by`, `created_at` | Rotation attribution |
| `destroyed_at` | Cryptographic erasure or confirmed external deletion |

A Secret value update appends a version and atomically moves
`current_version_id`. It never updates ciphertext in place. A rewrap caused by
KEK rotation may update only `wrapped_dek` and `key_id`; it does not create a
new logical credential version because the value did not change.

### `agent_secret_binding`

One authorization for an Agent revision's selected plugin:

| Field | Meaning |
|---|---|
| `agent_revision_id` | Immutable Agent definition that requests the capability |
| `plugin_activation_id` | Team activation that authorizes and pins the package |
| `plugin_name`, `consumer_name` | Manifest-declared environment name or future typed credential slot |
| `secret_id` | Team-owned logical Secret |
| `approved_digest` | Exact release digest reviewed for this delivery |
| `version_mode` | Follow `current`, or pin an exact logical version |
| `created_by`, `created_at`, `revoked_at` | Administrative lifecycle |

The Agent revision is the recommended binding level because two Agents may use
the same plugin with different accounts, and a run with no Agent already loads
no plugin. A Team-wide plugin binding is simpler but gives every Agent selecting
that plugin the same authority. It remains an option to evaluate rather than a
second implicit inheritance layer.

The write is valid only when:

- the Agent and Secret belong to the same Team;
- the Agent revision names the plugin;
- the Team has an enabled activation for the plugin;
- the activation digest equals `approved_digest`;
- inspection says that exact release declares `consumer_name`; and
- the caller has the required Team permission.

### `task_run_secret`

One non-secret snapshot of a run grant:

| Field | Meaning |
|---|---|
| `task_run_id`, `binding_id` | Run and decision that produced the grant |
| `secret_version_id` | Exact embedded logical version, if applicable |
| `provider_version` | Exact external version returned at resolution, if available |
| `plugin_digest`, `consumer_name` | Exact code and injection target |
| `status` | `pending`, `materialized`, `revoked`, `expired`, or `failed` |
| `materialized_at`, `revoked_at` | Runtime lifecycle evidence |

It contains no ciphertext, plaintext, lease token, provider error body, or
credential hash. A hash of the value would enable offline guessing for
low-entropy Secrets and is not needed to explain the run.

## Binding Semantics

### Why The Binding Belongs To An Agent Revision

Agent revisions are append-only and already answer which plugin selection and
instructions a run used. Binding to one gives the authorization a stable
definition without adding mutable Secret names to the task input.

An Agent edit that changes bindings creates a new revision. Replacing the value
behind a logical Secret does not: it is credential rotation, not an Agent
behavior edit. The TaskRun snapshot joins the two histories.

### Plugin Pin Changes

Moving a Team activation to a different digest invalidates existing bindings
for new runs. Portal should show them as needing review and name the Agent,
plugin, old digest prefix, new digest prefix, and consumer names. It never shows
the bound values.

| Policy | Benefit | Cost |
|---|---|---|
| Always require review after a digest change | Strong supply-chain boundary; simplest to explain | Operational friction for frequent plugin releases |
| Follow pin automatically | Lowest friction | A compromised or broader release silently inherits credentials |
| Allow an explicit follow policy per binding | Supports trusted first-party release trains | More policy and UI; a trusted publisher still does not make every future build safe |

The recommendation is always require review in the first release. Evidence from
real update frequency can justify a follow policy later.

### Missing And Optional Consumers

A required manifest environment entry without a valid binding fails before the
plugin starts. An optional entry remains unset. A binding whose consumer no
longer exists in the pinned release is invalid and cannot be materialized.

The error names the Secret handle or display name, plugin, and consumer name,
but never a value, locator, provider response, or key identifier.

## Run Lifecycle

```text
Team Owner                 Server                         Worker
    │                         │                              │
    ├─ write/rotate Secret ──►│ encrypt or record reference │
    ├─ bind Agent consumer ──►│ validate activation/digest  │
    │                         │                              │
    │                    dispatch TaskRun                    │
    │                         ├─ resolve Agent revision      │
    │                         ├─ resolve plugin pins         │
    │                         ├─ validate bindings/digests   │
    │                         └─ snapshot task_run_secret    │
    │                         │                              │
    │                         │◄──── claim with run token ───┤
    │                         ├─ require live matching run   │
    │                         ├─ resolve exact versions      │
    │                         └─ return computed bundle ────►│
    │                         │                              ├─ inject each consumer
    │                         │                              └─ never set ambient env
    │                         │                              │
    │                         │◄──── report terminal state ──┤
    │                         └─ revoke leases / close grant │
```

The snapshot should occur where plugin pins and the Agent revision are resolved
today: while the worker claims the run. Resolution cannot use values supplied by
the worker, and a worker cannot browse Team state.

The worker-facing operation conceptually returns the bundle computed for one
TaskRun. It must not accept arbitrary Secret IDs, names, provider paths, or
version selectors. The concrete route belongs in
`internal/server/handlers/routes.go` when implemented; this proposal does not
declare an HTTP route as current behavior.

Materialization requires all of:

- a valid run token whose run matches the path;
- a non-terminal run in the expected execution state;
- the stored TaskRun grant set;
- an enabled Secret and non-destroyed version;
- an unchanged plugin digest and consumer contract; and
- a configured, healthy backend.

The response must use TLS, set `Cache-Control: no-store`, bypass body logging,
and return only the bundle computed by the Server. Retrying a failed fetch may
be necessary for reliability; “read exactly once” is not useful if a lost
response permanently fails an otherwise valid run. Every successful
materialization is still recorded.

## Injection Targets

### Stdio MCP

The first supported target should be a stdio MCP server. The TaskRun runtime
passes the resolved values as explicit overrides only to that MCP process.

The current MCP transport merges overrides into `os.Environ()`. Before this can
be a confidentiality boundary, the child baseline must be a deliberate,
scrubbed environment rather than an implicit copy of the worker. The resolved
Secret map must not be installed with `os.Setenv`, serialized into `mcp.json`,
or left in the runtime-wide expansion table.

### HTTP MCP

An HTTP credential should be represented as a typed header or authentication
slot, not interpolated into a URL. The MCP client requests the current value
from the in-memory grant set when constructing the outbound request. Redirects
must not forward an authorization header to another origin.

The existing MCP schema does not provide this typed credential contract. It is
separate implementation work and may follow stdio support.

### Memory-Backed File

Certificates, private keys, and tools that only accept a path need a file mode.
The candidate contract writes the value into a run-private memory-backed
directory, uses `0400`, passes only the path to the consumer, and unlinks it when
the consumer exits. BuildMax must not persist the file under the workspace,
artifacts, or `BUILDMAX_HOME`.

A local-process worker may not have a true tmpfs. The first release may omit
file mode rather than claim an on-disk temporary file is memory-backed.

### Ambient Worker Environment

This proposal recommends no Team Secret mode that adds values to the worker's
ambient environment. Such a mode would make every Bash command, hook process,
plugin process, crash dump, and environment diagnostic a consumer.

If compatibility evidence later requires it, it should be named as an unsafe,
whole-run grant, require an explicit operator policy, require the worker
sandbox baseline to be active, and still document that the model can
intentionally print or transmit it. It must not be the default or the first
implementation.

### Bash And Skills

A skill that tells the model to run an authenticated `curl` requires the token
in a model-controlled process and often in generated command text. That defeats
the intended boundary. The first release should refuse such a consumer and
direct plugin authors toward an MCP process or a future brokered HTTP tool.

## Storage Backends

### Embedded Encrypted Store

The embedded backend is the recommended default for an out-of-the-box private
deployment. It stores only ciphertext and wrapped data-encryption keys in
MySQL.

Each `secret_version` receives a fresh random DEK and nonce. The value is
encrypted with AES-256-GCM or an equivalently reviewed AEAD. Associated data
binds the ciphertext to at least the deployment, Team public ID, Secret public
ID, and logical version, so moving a ciphertext row between owners fails
authentication.

The DEK is wrapped by a KEK. [Google Cloud's envelope-encryption
guidance](https://docs.cloud.google.com/kms/docs/envelope-encryption) recommends
one locally generated DEK per write, storing only wrapped DEKs, and using a
central KEK. [AWS KMS data
keys](https://docs.aws.amazon.com/kms/latest/developerguide/data-keys.html) use
the same separation.

The KEK never belongs in the database or `server.yaml`. Candidate sources are:

- a root-key file mounted read-only into the Server process for the portable
  baseline;
- AWS KMS, Google Cloud KMS, or a later KMS wrapper using workload identity; or
- a Vault transit key for a private deployment already operating Vault.

The Server must fail startup when encrypted data exists but its KEK is missing
or unusable. It must not silently generate a replacement key or mark values
empty. Losing the KEK means losing the values; backup documentation must say
so.

### External Secret Reference

An external backend stores an encrypted descriptor naming a provider,
provider-managed Secret, and version selector. BuildMax does not copy the value
into its database. At run resolution the Server authenticates using its
workload identity, retrieves the value or a dynamic credential, records the
exact provider version when available, and applies the same TaskRun grant and
injection rules as embedded mode.

Provider configuration is deployment-scoped and operator-managed. A Team cannot
submit an arbitrary Vault address or cloud endpoint that causes the Server to
send its provider identity elsewhere. The operator defines named providers, TLS
roots, regions, allowed path prefixes, and authentication methods; Team records
select only among allowed providers and paths.

Vault should be the first external integration because private deployment is a
product promise and Vault supports on-premises static and dynamic Secrets. AWS
and Google integrations can follow the same interface. Provider parity is not a
first-slice requirement.

### Dynamic Credentials And Workload Identity

The strongest design does not retrieve a long-lived cloud Secret at all. The
TaskRun obtains an audience-bound identity, and the target exchanges it for a
short-lived role credential or lease.

BuildMax could expose a dedicated OIDC issuer and JWKS endpoint and let a live
TaskRun request a token with:

- immutable deployment and Team identifiers;
- TaskRun and Agent revision identifiers;
- exact plugin release digest;
- caller-selected audience constrained by an operator allow-list;
- a unique `jti`; and
- a five- to ten-minute expiry.

This should not reuse the general user access-token contract or expose the
BuildMax run token to the plugin. It needs dedicated rotating signing keys,
issuer configuration, HTTPS, audience policy, and an external-provider trust
setup.

OIDC is a later slice. The first schema should avoid assuming every provider
result is a static versioned byte string so a lease can fit without a migration.

## Authorization

The existing roles are `owner`, `admin`, and `member`. An `admin` can manage
Agents and workflows, while only an `owner` can manage Team membership or read
the audit trail.

The recommended first policy is:

| Action | Owner | Admin | Member |
|---|---:|---:|---:|
| List Secret metadata and binding health | yes | yes | no |
| Create, replace, disable, or destroy a value | yes | no | no |
| Create or revoke an Agent Secret binding | yes | no | no |
| Save an Agent revision with unresolved consumer requirements | yes | yes | no |
| Trigger an already authorized Agent run | yes | yes | yes |
| Read Secret audit events | yes | no | no |
| Reveal a value | no | no | no |

Members may indirectly use a credential by running an Agent whose owner has
already authorized the plugin. That is necessary for shared automation and must
be visible in the binding and audit surfaces. A member does not gain a general
read or binding capability.

This policy intentionally keeps value and binding authority with the owner
until BuildMax has finer Team grants. If operator evidence shows that owners
cannot be the operational Secret managers, add an explicit `secret_manager`
grant rather than quietly widening `admin`.

The Portal must not depend on a manual approval feature that is not implemented.
A future high-risk or environment approval can gate materialization without
changing the stored Secret and binding model.

## Audit And Provenance

Administrative and runtime actions need stable audit names. Candidate actions
are:

- `secret.created`;
- `secret.rotated`;
- `secret.disabled`;
- `secret.destroyed`;
- `secret.binding_created`;
- `secret.binding_revoked`;
- `secret.materialized`;
- `secret.revoked`; and
- `secret.access_denied`.

An event names the actor, Team, Secret public ID, binding or TaskRun, action,
and a bounded non-sensitive detail such as a plugin name and digest prefix. It
never carries:

- plaintext or ciphertext;
- a hash of plaintext;
- a provider token, lease ID, response body, or full path;
- an HTTP header or environment value; or
- prompt, tool output, command arguments, or file contents.

The existing audit table can represent an event per Secret materialization by
using the Secret as target and the TaskRun as bounded detail. If expected volume
or query needs make that awkward, a dedicated append-only access ledger should
be evaluated. Two partially overlapping trails would be worse than either, so
the implementation must pick one authoritative answer to “which runs
materialized this Secret?”

TaskRun trace provenance should record handles, binding IDs, plugin digests,
consumer names, and materialization outcomes. It must not record provider
locators or values.

## Redaction And Data Flow

The runtime should register every materialized static value with a per-run
exact-value redactor before a consumer starts. Redaction should cover:

- application logs;
- worker status errors;
- MCP stdout and stderr;
- tool results before they enter model context;
- streamed output; and
- durable traces.

The existing shape-based redaction remains useful for credentials that did not
come through the Broker. Exact-value redaction should ignore empty and very
short values to avoid replacing ordinary output everywhere, and it should use a
bounded representation so a very large certificate or document does not make
every trace operation unbounded.

Redaction must be described as mitigation only. It cannot reliably catch
encoding, escaping, concatenation, truncation, encryption, an authenticated
tool taking an unwanted action, or direct network transmission. The primary
control remains withholding the value from the general Agent environment and
limiting which reviewed code receives it.

Artifact upload is particularly difficult. Automatically rewriting arbitrary
artifacts can corrupt outputs, while declaring every binary safe would be
false. The first release should not claim artifact DLP. It may exact-match scan
bounded text previews and warn or quarantine on a match, but artifact policy
needs a separate measured decision.

## Existing Credential Debt

Adding Team Secrets without removing broader deployment credentials from
workers would produce a narrow new door beside an open old one.

### Object Storage

Workers currently receive long-lived object-store credentials. The desired
replacement is one of:

| Option | Benefit | Cost |
|---|---|---|
| Server-mediated object transfer | Uniform run-token authorization and no storage credential in the worker | Server bandwidth and large-file pressure |
| Run-prefix presigned requests | Direct data path, short TTL, object/prefix scope | More request orchestration and expiry handling |
| Cloud workload identity | No stored access key and native provider audit | Provider-specific and harder for portable MinIO deployments |

Artifact upload already goes through a run-scoped Server route. Workspace and
run-state transfer determine whether the remaining storage credential can be
removed entirely. This should be prerequisite work, not silently delegated to
the Team Secret feature.

### Direct Model Credentials

Managed inference is the recommended cloud-worker mode because the provider key
and upstream details stay on the Server. A deployment may retain direct mode
for trusted local execution, but a cloud worker should not receive a
deployment-wide provider key by default.

The plaintext `llm_model.api_key` should migrate to the same encrypted backend
or an external operator Secret reference. It remains deployment-scoped and
Server-only; it does not become a Team Secret.

### Run Token Delivery

The worker clears `BUILDMAX_RUN_TOKEN` from its environment after reading it,
but Kubernetes stores the value in the Job specification. A per-run immutable
Kubernetes Secret mounted as a file, with an owner reference and restrictive
RBAC, removes it from the Job environment and lets garbage collection follow
the Job. It still stores a credential in Kubernetes and is an intermediate
step toward a projected workload identity.

This is defense in depth. The run token remains run-scoped, expires, and is
refused by terminal-state checks; moving it does not replace those controls.

## Failure Semantics

Secret failures happen before or during execution and must remain
distinguishable:

| Failure | Run behavior |
|---|---|
| Required binding missing | Fail before plugin startup, naming the plugin and consumer |
| Secret disabled or destroyed | Fail before materialization |
| Binding digest differs from activation | Fail as “binding needs review”; do not skip the plugin |
| Embedded KEK unavailable | Server health is degraded; refuse affected materialization without treating the value as empty |
| External provider unavailable | Retry within a bounded startup budget, then fail with a sanitized provider-class error |
| Run token invalid or run terminal | Refuse without revealing whether a Secret exists outside the run grant |
| Dynamic lease expires and cannot renew | Stop the affected consumer or fail the run; do not continue as unauthenticated work |
| Revocation fails after terminal state | Preserve the terminal outcome, record a sanitized revocation failure, and retry out of band within a bound |

A background run whose declared authenticated capability cannot start must fail
rather than continue without it. This matches plugin materialization: silent
degradation would produce an output that does not reflect the Agent definition.

## API Shape To Evaluate

User-facing operations should be conventional resource APIs while remaining
write-only for values. Candidate operations are:

- list and get Secret metadata;
- create a Secret with its first value or external reference;
- replace the value to create a version;
- disable, re-enable, and destroy;
- list version metadata without values;
- create and revoke Agent revision bindings; and
- inspect binding health against current plugin pins.

No response includes a value. Create and replace responses return the Secret
metadata and new version number only. Input fields containing values are
excluded from request logging, validation errors, and audit details.

The worker-facing surface returns only the grant set already computed for its
run. It provides no list, get-by-name, provider lookup, or arbitrary version
operation.

The exact route strings are not decided here. When implemented,
`internal/server/handlers/routes.go` remains their source of truth and the
OpenAPI document must describe the absence of a reveal operation.

## Runtime And Package Boundaries

The recommended ownership follows the existing dependency direction:

| Area | Responsibility |
|---|---|
| `internal/core/model` | Secret metadata, versions, bindings, run grants, errors, and narrow store interfaces |
| `internal/service/secret` | Lifecycle rules, binding validation orchestration, materialization, and revocation |
| `internal/infra/secret` | AEAD/envelope implementation and external provider adapters |
| `internal/infra/db` | Row structs and metadata/ciphertext persistence; no provider calls |
| `internal/server/handlers` | User and worker authentication, Team authorization, request/response shaping |
| `internal/agentapp/taskrun` | Consume an authorized in-memory grant set and route each value to a named consumer |
| `internal/infra/mcp` | Construct process/request-specific credential injection without ambient inheritance |

`internal/core` must not import configuration, cryptography providers,
infrastructure, GORM, Server code, or Agent application assembly. Configuration
selects provider implementations during bootstrap; it does not resolve Team
resources itself.

The Secret service should accept a small KEK/provider interface so embedded,
Vault, and cloud implementations do not leak into the handler or runtime.

## Options And Trade-Offs

### Option A: Deployment Environment Variables Only

Keep the current model and ask operators to put every plugin credential in the
worker environment.

| Strength | Concern |
|---|---|
| No new database, crypto, UI, or provider code | No Team scope, no per-Agent account choice, no usable audit, every worker child can inherit every value |

This is acceptable for trusted local use and unsuitable as the cloud execution
contract.

### Option B: Encrypted Team Store With Whole-Run Environment Injection

Add Portal management and encrypt values, then expose selected keys as worker
environment variables.

| Strength | Concern |
|---|---|
| Familiar UX and broad compatibility | Storage improves while runtime exposure remains broad; model-chosen Bash can print every value |

This is the common minimum implementation in CI products. It is not the
recommended first design for an Agent runtime whose shell is controlled by a
model rather than a reviewed workflow file.

### Option C: External Secret Managers Only

Require Vault or a cloud provider and let the worker retrieve values with a
workload identity.

| Strength | Concern |
|---|---|
| Strong provider lifecycle and no BuildMax value storage | Violates the out-of-the-box private-deployment promise; authorization differs by provider; workers gain provider lookup ability unless carefully brokered |

External-only is appropriate for some enterprises but not as the one product
path.

### Option D: Server-Side Broker With Embedded And External Backends

Build one Team binding and run-grant model, keep provider authority on the
Server, and inject values only into named consumers.

| Strength | Concern |
|---|---|
| Portable baseline, enterprise integration, narrow runtime exposure, common audit and rotation semantics | More design and implementation work; the Server handles plaintext; typed consumers need runtime changes |

This is the recommended direction.

### Option E: Workload Identity Only

Issue an OIDC identity for every run and require all targets to federate.

| Strength | Concern |
|---|---|
| No stored third-party credentials and short-lived authority | Many APIs and existing plugins accept only static tokens; private issuer setup and provider trust are substantial |

This is the desired long-term preference, not a complete first release.

## Staged Delivery If Accepted

### Stage 0: Remove Broad Worker Credentials

- replace deployment-wide object-store credentials with Server-mediated or
  run-scoped storage access;
- make managed inference the cloud-worker default and keep direct credentials
  out of untrusted workers;
- move Kubernetes run-token delivery from the Job environment to a per-run
  mounted Secret as an intermediate hardening step; and
- move managed model credentials out of plaintext database columns.

This stage may be split across existing P0.5 and P3 work, but its acceptance
must be explicit before BuildMax claims that a worker holds only what its run
needs.

### Stage 1: Embedded Static Team Secrets

- implement the resource, version, binding, and TaskRun snapshot rows;
- implement envelope encryption with a portable mounted KEK;
- add owner-only create, replace, disable, destroy, and binding operations;
- add Portal metadata, rotation, and binding-health surfaces;
- support process-specific stdio MCP injection;
- add audit actions and per-run exact-value redaction; and
- fail before plugin startup on missing, stale, or unusable bindings.

### Stage 2: External Provider References

- add operator-configured provider records and path ceilings;
- implement Vault first, then evidence-driven AWS and Google adapters;
- record exact provider versions and access outcomes;
- support dynamic leases where the provider exposes them; and
- document backup, outage, and rotation operations.

### Stage 3: Workload Identity

- add a dedicated OIDC issuer, rotating signing keys, and JWKS;
- define immutable TaskRun subject and audience claims;
- exchange identity for Vault, AWS, or Google short-lived authority;
- renew or reacquire within a bounded run lifecycle; and
- remove stored static cloud credentials from supported paths.

### Stage 4: Higher-Risk Consumers And Approvals

- evaluate typed HTTP authorization and memory-backed file injection;
- integrate future Team approvals or environment gates at materialization;
- decide whether any unsafe whole-run environment mode is justified; and
- qualify artifact scanning or quarantine without claiming comprehensive DLP.

## Acceptance Evidence

Before accepting the overall direction, the project needs:

- a threat-model review covering database disclosure, cross-Team access, run
  token theft, plugin replacement, provider compromise, process inheritance,
  and network exfiltration;
- a prototype proving a worker can start one authenticated stdio MCP server
  without the value appearing in the worker environment, Bash environment,
  generated configuration, trace, log, or session;
- a schema and crypto review covering nonce generation, AEAD associated data,
  DEK wrapping, KEK loss, rotation, and destruction;
- an operational drill restoring an encrypted database backup with the correct
  KEK and proving restoration fails safely without it;
- a plugin pin-move test proving new code does not inherit old bindings;
- a rotation test proving an in-flight run keeps its recorded version and the
  next run resolves the new one;
- cross-Team and cross-run authorization matrix tests;
- a terminal-run test proving materialization and managed spending are refused;
- failure-injection tests for KMS/Vault timeouts, provider denial, lease expiry,
  and redaction paths; and
- measurements of Server bandwidth and latency for Secret and object-storage
  brokerage so the security boundary does not create an unexamined bottleneck.

Stage 1 is not ready to ship until at least these claims hold:

1. a database backup alone cannot recover a value;
2. no user-facing operation reveals a value;
3. a worker cannot choose a Secret outside its stored TaskRun grants;
4. a pin move invalidates the old delivery authorization;
5. a Team value is absent from the worker ambient environment and model context;
6. rotation affects the next run rather than mutating one in flight;
7. a terminal run cannot materialize again; and
8. audit and trace metadata explain the grant without containing credential
   material.

## Open Questions

1. Is Agent revision the correct binding scope, or does evidence require a
   Team-wide plugin default with Agent-specific overrides?
2. Should listing Secret names be owner-only, or may an admin see metadata and
   missing-binding health without gaining value or binding authority?
3. Is always-review-on-plugin-digest-change acceptable operationally, or is an
   explicit trusted release-channel policy needed?
4. Should an external-provider locator be encrypted with the value, or is
   database-visible provider metadata necessary for operation and audit?
5. Does the embedded KEK baseline use a mounted file only, or also support an
   environment value for Compose convenience despite process-environment
   exposure?
6. Which object-storage replacement best preserves large-file performance in
   local-process, Compose, and Kubernetes deployments?
7. Can local-process workers support a defensible memory-backed file mode on
   every platform, or should file injection be Kubernetes-only at first?
8. Does a materialization append to the existing audit trail, a dedicated
   Secret access ledger, or both with one explicitly derived from the other?
9. How is a dynamic lease renewed when the main Agent loop is blocked in a
   long-running tool call, and what happens when renewal cannot complete?
10. Which OIDC claims are stable and useful to Vault/AWS/GCP without exposing
    mutable names or making plugin digests part of an external policy operators
    cannot maintain?
11. Should a Secret version remain decryptable while a queued or retryable run
    references it, and what maximum retention prevents an abandoned run from
    blocking destruction indefinitely?
12. What artifact behavior is honest when a credential-bearing process writes
    the value or a transformed form into an output file?

## Likely Destination If Accepted

Acceptance would:

- add a Secret Broker active plan under `docs/design/` and place its stages in
  the relevant roadmap priorities;
- revise the worker-run-token, plugin-distribution, sandbox, trace, managed LLM,
  data-model, deployment, and configuration records where their boundaries
  change;
- add operator and user documentation only as each configurable slice ships;
- create implementation Issues for worker credential removal, embedded storage,
  binding and runtime injection, external providers, and workload identity; and
- delete this proposal once its durable decisions have moved to the design
  record.
