# Local Kubernetes Deployment

> **Audience:** contributors and operators · **Status:** beta
>
> Use this path for Kubernetes worker Jobs, RBAC, Ingress, MinIO, and manifest
> changes. For ordinary server and Portal work, the faster
> [Compose smoke](compose.md) covers the same product flow.

## Requirements

- Docker with at least 6 GB available
- kubectl

kind itself is not a prerequisite: `cmd/mk` pins it and runs it through
`go run`, so every cluster is created, inspected, and deleted by the same
version. The command does not install system packages or start background
port-forwards. It always addresses the selected cluster through an explicit
kubectl context.

## Start And Verify

```bash
./make kind up
```

This creates the `buildmaxdev` cluster, then:

1. installs ingress-nginx, MySQL, and MinIO
2. creates the `bmstore` bucket and widens the MySQL dev grant, each from an
   in-cluster Job
3. builds and loads the server, Portal, and deterministic mock-model images
4. generates an ephemeral local Secret and applies the BuildMax manifests
5. waits for every Deployment to become ready
6. creates a real TaskRun, executes it in a Kubernetes worker Job, and verifies
   its artifact through the API

The cluster config and the dependency manifests it applies live in
`deployment/dev-kind/`; the orchestration is `cmd/mk/kind.go`. They are
development-only and are not part of a real deployment.

Open <http://localhost:8080>. Portal and API share that origin, so no
`/etc/hosts` entries or CORS pairing are needed. The command prints a fresh
single-use code for `deployment-smoke@buildmax.local` after verification.

That origin is also the **Server URL** for the Desktop app and `buildmax login`.
The default both offer, `http://localhost:5678`, is the port a server started on
this machine listens on; nothing publishes it here, because the ingress is the
only way in.

That code is spent the first time it is used, and is printed once. When it is
gone, `./make kind info` issues another one — it does not, and cannot, show the
old one.

## Daily Commands

```bash
./make kind smoke   # rerun the end-to-end assertions without rebuilding
./make kind smoke managed  # the same, with task runs reaching models through the gateway
./make kind seed    # put the models in settings.local.yaml into the cluster's catalog
./make kind images  # rebuild and load local images without applying manifests
./make kind info    # endpoints, plus a fresh login code for the smoke account
./make kind forward # forward the in-cluster MySQL and MinIO to 127.0.0.1
./make kind status  # read-only summary of the cluster, ingress, and workloads
./make kind logs    # pods, jobs, events, server, Portal, and worker logs
./make kind down    # delete the selected cluster
```

`smoke managed` swaps the `buildmax-config` ConfigMap for
`deployment/smoke/server.kind.managed.yaml`, restarts the server, and reruns the
same assertions with task-run inference going through the gateway. It proves
what the default run cannot: a worker Job completes a real task holding no
provider credential, and its run token reaches the pod through the Job spec.
The cluster stays in managed mode afterwards — rerun `./make kind up` to return
it to direct.

`info` prints the cluster, the Portal URL and its health, the MinIO credentials,
and issues a single-use login code — for `deployment-smoke@buildmax.local`
by default, or for the account named as `./make kind info alice@example.com`.

`status` changes nothing. It prints the selected cluster and context, probes
<http://localhost:8080/healthz> through the ingress, and lists nodes plus the
Deployments, Jobs, and Pods in `ingress-nginx`, `db`, `storage`, and
`buildmax`. Use it to tell a missing cluster from an unhealthy one before
reading the much longer `kind logs` output.

Use an isolated cluster name when another contributor or task owns the default:

```bash
BUILDMAX_KIND_CLUSTER=buildmax-my-change ./make kind up
BUILDMAX_KIND_CLUSTER=buildmax-my-change ./make kind down
```

The cluster uses host ports `8080` and `8443`. Stop another local service on
`8080`, or use Compose at a different `BUILDMAX_PORTAL_PORT`, before creating
the cluster.

## Read The Data A Run Wrote

MySQL and MinIO have ClusterIP Services and the cluster publishes only the
ingress ports, so neither is reachable from this machine on its own.

```bash
./make kind forward     # publishes both to 127.0.0.1 until you stop it
```

It forwards MySQL to `3306` and MinIO to `9000` (API) and `9001` (console), and
prints how to connect to each. Every line the forwards write is tagged with the
target it came from. A target whose host port is already taken on this machine
is skipped with a warning — a local MySQL on `3306` costs you that forward, not
MinIO's — and the warning names the kubectl command that forwards it to a port
of your choosing.

While it runs, connect to MySQL with any client — `mysql -h 127.0.0.1 -P 3306
-ubuildmax -pbuildmax buildmax`, or the DSN
`buildmax:buildmax@tcp(127.0.0.1:3306)/buildmax`. Those are the development
credentials in `deployment/dev-kind/mysql.yaml`; the database is `emptyDir` and
goes away with the cluster. The account may use any schema, not just `buildmax`,
so `database.name` in a local `server.yaml` can say whatever you like — the
server creates the schema it is pointed at on first start. The same DSN in
`BUILDMAX_TEST_DSN` is what runs the store integration tests under
`internal/infra/db` against a real MySQL.

MinIO's console is <http://127.0.0.1:9001> with `minio` / `minio123`, and the
run artifacts are in the `bmstore` bucket.

For a single query, skip the forward and use kubectl directly:

```bash
kubectl --context kind-buildmaxdev -n db exec deployment/mysql -- \
  mysql -ubuildmax -pbuildmax buildmax -e "select * from task_run\G"
```

## Smoke Versus Real Providers

The local command deliberately overlays `deployment/smoke/server.kind.yaml`
and an in-cluster OpenAI-compatible mock. This keeps contribution checks
deterministic and ensures CI never needs a provider credential.

For a private deployment, use `deployment/buildmax-deploy.yaml` as the readable
baseline, create `buildmax-secret` from
`deployment/buildmax-secret.example.yaml`, and configure a real model endpoint.
Do not use the generated smoke Secret or mock model outside local verification.

### Drive The Cluster With Your Own Models

`./make kind seed` puts every provider model in the repository-root
`settings.local.yaml` into the cluster's catalog. It exists so the CLI and
Desktop can exercise the managed transport — `transport: buildmax` — against
real inference without a hosted deployment to point at.

```bash
./make kind up      # the stack, still answering from the mock
./make kind seed    # your models in its catalog
```

The command adds each model with `buildmax-server model add` and stops there: a
catalog row is callable as soon as it exists, so nothing needs restarting and no
configuration changes. It then prints the managed model entries to paste into
your own `BUILDMAX_HOME/settings.yaml`; sign in with `buildmax login` against
<http://localhost:8080> first, because a managed entry takes its credential from
the login rather than from the file.

A model is named by the `name` it was added under, which is its display name in
`settings.local.yaml`, or its model id when it has none. The printed entries and
`buildmax models --server` both show the name to use.

What it deliberately does not touch is the cluster's own inference.
`conversation.model` and the worker keep answering from the in-cluster mock, so
Portal conversations and `./make kind smoke` stay deterministic and cost
nothing. A rerun is safe: a model whose name is already in the catalog keeps
its row and its ID. Changing a seeded model's endpoint or credential means
renaming it in `settings.local.yaml`, or rebuilding the cluster — `add` does not
update a row.

A real credential in `settings.local.yaml` reaches the cluster's MySQL in plain
text. That database is thrown away with the cluster, and this path is for local
verification only.

### A real model without a provider key

Between the mock and a hosted provider there is a third option: point the
deployment at an Ollama daemon **on this machine**. Real inference, real tool
calls, no credential, no bill — and the gateway, the `llm_call` ledger, and
quota all run for real.

Do not put the daemon in the cluster. A pod cannot reach the host's GPU, so
inference falls back to the CPU of the VM the cluster runs in. Leave it on the
host and give the deployment an address that reaches it — under Docker Desktop
that is `host.docker.internal`, which resolves inside pods and forwards even to
a daemon bound to the host's loopback. `kind seed` rewrites a loopback address
to that name for you; by hand it is:

```bash
# a catalog target; it is callable by name as soon as the row exists
kubectl --context kind-buildmaxdev -n buildmax exec deployment/buildmax-server -- \
  buildmax-server model add --name "Host Ollama" --provider ollama \
      --api-url http://host.docker.internal:11434 \
      --model qwen3:8b --context-window 32000
```

`deployment/buildmax-deploy.yaml` carries the same thing for `conversation.model`
as a commented block. On a Linux host the address is the Docker bridge gateway
(`docker network inspect kind`) and the daemon needs `OLLAMA_HOST=0.0.0.0`.

## Why Compose Still Exists

Compose and kind verify the same user-visible flow but different execution
contracts:

| Path | Worker | Storage | Best for |
|---|---|---|---|
| Compose | local process in the server container | shared local filesystem | API, Portal, scheduler, and most backend changes |
| kind | one Kubernetes Job per TaskRun | MinIO shared by server and workers | Jobs, RBAC, Ingress, object storage, and manifests |

Keeping both makes the common contributor loop quick while preserving a real
deployment check for the distributed path.
