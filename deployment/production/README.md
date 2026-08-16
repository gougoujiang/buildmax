# Private Deployment Reference

> **Audience:** operators · **Status:** current — reference, not an installer

[`buildmax.yaml`](buildmax.yaml) is a complete BuildMax deployment written to be
read and adapted. It is not applied as-is: every `REPLACE_ME` must be replaced,
and a `kubectl apply` of the unedited file fails to start rather than coming up
against the wrong dependencies.

There is no Helm chart and no kustomize base. The manifest is plain YAML so it
can be converted into whatever your cluster is already managed with, rather than
arriving with a structure you have to undo.

## What BuildMax Brings, And What You Bring

BuildMax deploys the server, the Portal, and the worker Jobs the server creates
per task run. It does not install a database, an object store, an ingress
controller, or certificates.

`deployment/buildmax-deploy.yaml` is the other path: a self-contained stack with
its own MySQL and MinIO, used by `./make kind up`. That one is a development
environment. Do not adapt it for production — it exists to be started and thrown
away, and its manifest hardcodes in-cluster dependency addresses that only
resolve there.

## The Dependency Contract

### MySQL

| Requirement | Value |
|---|---|
| Version | 8.0 or later |
| Character set | `utf8mb4` — the DSN asks for it, and the schema assumes it |
| Privileges | Full DDL on its own schema: BuildMax creates and alters its tables at startup |
| TLS | Set `database.tls: "true"` so the connection is encrypted and the certificate verified |

The DDL privilege is not optional. The schema is applied by the server on
start — `AutoMigrate` for additive changes plus an ordered migration list for
everything else — so a user restricted to DML cannot bring the server up. Grant
it on the BuildMax schema alone, not server-wide.

Sizing is driven by task runs and messages rather than by user count. A team
running a few hundred task runs a day is a small database.

### Object storage

| Requirement | Value |
|---|---|
| API | S3, or an S3-compatible implementation |
| Bucket | One, dedicated. BuildMax prefixes its keys but does not expect to share |
| Access | Read, write, and list on the whole prefix |
| Credentials | IRSA, workload identity, or an instance profile preferred; static keys supported |

Leave `endpoint` empty for AWS S3, so the SDK resolves the regional endpoint and
uses virtual-host addressing. Set it to the base URL of a store you run, which
also switches addressing to bucket-in-path. `path_style` overrides that
derivation for a compatible store that uses virtual-host addressing.

Leaving the access key and secret key unset is the better configuration where
your platform supports it. Workers are handed the storage credentials to read
and write run state, and a worker executes model-chosen shell commands — so a
static key is a long-lived credential inside that blast radius, while a
projected identity is not.

Lifecycle rules are yours to set. BuildMax never deletes run state or artifacts.

### Ingress and TLS

The manifest assumes one origin serves both the Portal and the API, which is
what `BUILDMAX_API_BASE: "/"` depends on. Serving them from two hosts means
setting an absolute API base and configuring `cors_origin` accordingly.

No ingress class is assumed and no controller-specific annotations are included.
Set `ingressClassName`, and add whatever annotations your controller needs.

`/healthz` and `/readyz` are deliberately **not** routed through the Ingress. The
kubelet reaches them directly on the pod; publishing them only exposes
dependency status to anyone who asks.

### Model access

The reference points `conversation.model` at an OpenAI-compatible endpoint. A
deployment that would rather not distribute provider keys can serve
operator-approved models through the managed gateway instead; see
[`docs/design/llm-gateway.md`](../../docs/design/llm-gateway.md) for what is
implemented today.

## Upgrades

Schema changes are applied by the server at startup and move **forward only**.
There are no down migrations.

What is supported is rolling the **binary** back one release: schema version N
keeps serving code from release N-1. So an upgrade that goes wrong is recovered
by redeploying the previous image tag — which is why the manifest says to pin a
version rather than track `:latest`.

Rolling the *database* back is not supported. Recovery from a bad schema change
is a restore from backup, so take one before an upgrade that crosses a release
carrying migrations.

The rollout sets `maxUnavailable: 0`, so the new pod must pass readiness before
an old one goes away. The `startupProbe` gives the first pod room to finish
migrations before liveness starts counting.

## What This Reference Does Not Cover

Stated rather than left to be discovered:

- **NetworkPolicy.** Worker pods reach the model endpoint, object storage, and
  the server. Restricting egress is worth doing and is not settled here — see
  [`docs/proposals/agent-execution-policy.md`](../../docs/proposals/agent-execution-policy.md).
- **Backups.** Database and bucket backups are yours. BuildMax has no export or
  import command.
- **Horizontal scaling of workers.** Worker Jobs are created per task run and
  bounded by their own resource settings, not by a replica count. Cluster
  capacity is what limits concurrency.
- **Multi-region, HA MySQL, and SSO.** Out of scope for this reference.
- **Verification against a live cloud provider.** This manifest is checked in
  CI for whether its configuration parses into the config the server actually
  reads, and its container hardening is the same set `./make kind up` applies
  and exercises on every run. It has not been applied against a real RDS or S3
  account — the dependency contract above is what stands in for that.
