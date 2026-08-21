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
2. creates the `bmstore` bucket from an in-cluster Job
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

## Daily Commands

```bash
./make kind smoke   # rerun the end-to-end assertions without rebuilding
./make kind smoke managed  # the same, with task runs reaching models through the gateway
./make kind images  # rebuild and load local images without applying manifests
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

## Smoke Versus Real Providers

The local command deliberately overlays `deployment/smoke/server.kind.yaml`
and an in-cluster OpenAI-compatible mock. This keeps contribution checks
deterministic and ensures CI never needs a provider credential.

For a private deployment, use `deployment/buildmax-deploy.yaml` as the readable
baseline, create `buildmax-secret` from
`deployment/buildmax-secret.example.yaml`, and configure a real model endpoint.
Do not use the generated smoke Secret or mock model outside local verification.

## Why Compose Still Exists

Compose and kind verify the same user-visible flow but different execution
contracts:

| Path | Worker | Storage | Best for |
|---|---|---|---|
| Compose | local process in the server container | shared local filesystem | API, Portal, scheduler, and most backend changes |
| kind | one Kubernetes Job per TaskRun | MinIO shared by server and workers | Jobs, RBAC, Ingress, object storage, and manifests |

Keeping both makes the common contributor loop quick while preserving a real
deployment check for the distributed path.
