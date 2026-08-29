# DigitalOcean Qualification Infrastructure

> **Audience:** operators · **Status:** current
>
> This is the lowest-cost external infrastructure prepared for the BuildMax
> beta gate. It provisions infrastructure only; it does not yet deploy the
> BuildMax application or constitute beta-gate evidence.

`./make ocean` manages a disposable DOKS cluster and managed MySQL cluster with
OpenTofu. It deliberately reuses three resources that an operator creates and
keeps outside OpenTofu:

| Resource | Default | Ownership |
|---|---|---|
| DigitalOcean Project | `buildmax-beta` | Persistent, operator-managed |
| VPC | `buildmax-beta` in `sgp1` | Persistent, operator-managed |
| Spaces bucket | `buildmax-beta` in `sgp1` | Persistent, operator-managed |
| DOKS cluster | `buildmax-beta-doks`, one `s-2vcpu-4gb` node | Disposable, OpenTofu-managed |
| Managed MySQL | `buildmax-beta-mysql`, one `db-s-1vcpu-1gb` node | Disposable, OpenTofu-managed |
| MySQL firewall and `buildmax` database | Attached to managed MySQL | Disposable, OpenTofu-managed |

DOKS high availability, auto-upgrade, and surge upgrade are explicitly off.
The DOKS and MySQL resources accrue charges until `./make ocean down` succeeds;
the Project and VPC are free, and the persistent Spaces subscription continues
independently. Confirm current provider prices before creating the resources.

## Prerequisites

Install [OpenTofu](https://opentofu.org/docs/intro/install/) and `kubectl`. Copy
the repository example and add the three credentials:

```bash
cp .env.example .env
chmod 600 .env
```

The DigitalOcean API token needs only the capabilities used by these resources:

- account and actions: read
- regions and sizes: read
- Kubernetes: create, read, update, delete, access cluster
- databases: create, read, update, delete, view credentials
- Project: read and assign resource
- VPC: read

Project and VPC create, update, and delete are unnecessary because OpenTofu
reads them as existing resources. Scope the Spaces key to the `buildmax-beta`
bucket with read, write, and delete; BuildMax workers need all three during the
qualification run.

Verify the local setup without calling DigitalOcean or writing a file:

```bash
./make ocean doctor
```

## Create And Inspect

Review the create plan, then apply it:

```bash
./make ocean plan
./make ocean up
```

`up` repeats the plan and requires typing the Project name before it applies
anything. Once it completes:

```bash
./make ocean info
./make ocean status
export KUBECONFIG="$HOME/.buildmax/qualification/ocean/kubeconfig.yaml"
kubectl get nodes
```

`info` prints resource names and endpoints, but never the database password or
kubeconfig contents.

The OpenTofu source is under [`deployment/ocean/`](../../deployment/ocean/).
Names and the region can be changed without editing it:

| Variable | Default |
|---|---|
| `BUILDMAX_OCEAN_PROJECT` | `buildmax-beta` |
| `BUILDMAX_OCEAN_VPC` | `buildmax-beta` |
| `BUILDMAX_OCEAN_BUCKET` | `buildmax-beta` |
| `BUILDMAX_OCEAN_REGION` | `sgp1` |
| `BUILDMAX_OCEAN_STATE_DIR` | `~/.buildmax/qualification/ocean` |

All three persistent resources must use the configured region.

## State And Secrets

OpenTofu state contains the generated MySQL password and DOKS kubeconfig. The
command therefore refuses a state directory inside the Git checkout. The
state, saved plans, provider working directory, and generated kubeconfig live
under `BUILDMAX_OCEAN_STATE_DIR`, whose directory and sensitive files are set
to owner-only permissions.

Treat that directory as a credential:

- never upload it to the application Spaces bucket
- back it up securely while the managed resources exist
- do not delete it before `./make ocean down`
- rotate database credentials and Kubernetes access if it is disclosed

The repository `.env` also remains local and gitignored. OpenTofu reads its
credentials through the environment populated by `./make`; no credential is
written into the OpenTofu source.

## Destroy

When the qualification session is over:

```bash
./make ocean down
```

The command shows a destroy plan and requires the Project name again. It
destroys the disposable stack declared in `deployment/ocean`: DOKS, managed
MySQL, the MySQL firewall/database, the cluster's Project assignment, and
DigitalOcean resources associated with that DOKS cluster. It retains the
existing Project, VPC, Spaces bucket, and every Route 53 record.

Check the DigitalOcean control panel after teardown. If an application
deployment created a load balancer outside this OpenTofu state, remove it
separately after preserving the beta-gate evidence.

## Next Qualification Step

This command establishes the real external dependency boundary, but the beta
gate still requires a pinned BuildMax candidate to be deployed and exercised.
Before adding that application phase, BuildMax must support verifying
DigitalOcean Managed MySQL with its provider CA certificate; using
`skip-verify` would not satisfy the TLS requirement. DNS remains a manual
Route 53 record after the Kubernetes load balancer receives an address.

Record the eventual deployment, smoke results, failure drills, restore, and
rollback in the [Beta readiness record](beta-readiness.md).
