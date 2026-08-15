# Local Kubernetes Deployment

> **Audience:** contributors and operators · **Status:** beta
>
> Use this path for Kubernetes worker Jobs, RBAC, Ingress, MinIO, and manifest
> changes. For ordinary server and Portal work, the faster
> [Compose smoke](compose.md) covers the same product flow.

## Requirements

- Docker with at least 6 GB available
- kind
- kubectl

The command does not install system packages or start background
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

Open <http://localhost:8080>. Portal and API share that origin, so no
`/etc/hosts` entries or CORS pairing are needed. The command prints a fresh
single-use code for `deployment-smoke@buildmax.local` after verification.

## Daily Commands

```bash
./make kind smoke   # rerun the end-to-end assertions without rebuilding
./make kind logs    # pods, jobs, events, server, Portal, and worker logs
./make kind down    # delete the selected cluster
```

`./make setup`, `./make deploy`, and `./make unsetup` remain compatibility
aliases for `kind up`, `kind up`, and `kind down`.

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
