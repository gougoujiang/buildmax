# Private Production Operations

> **Audience:** operators, contributors, and SREs · **Status:** proposal — under discussion

Related: [../ROADMAP.md](../ROADMAP.md) P3, [enterprise deployment design](../design/enterprise-deployment.md), [deployment overview](../deploy/overview.md), and [local Kubernetes deployment](../deploy/local-kind.md).

## Problem

Compose and kind now exercise a real login, TaskRun, worker, and artifact path.
They are valuable contribution checks, but they use local or ephemeral
dependencies and deliberately do not define a production operating contract.
A private operator still needs to know which components BuildMax owns, which
dependencies it must operate, and how to upgrade, recover, observe, and secure
the complete system.

## Goals

- Define one supported private Kubernetes reference topology for the beta path.
- State the contract for external MySQL and S3-compatible storage.
- Specify TLS/Ingress, secrets, resource requests/limits, health/readiness,
  database migration, backup/restore, and upgrade/rollback expectations.
- Keep Compose and kind as fast local development and verification paths.
- Provide a repeatable acceptance check that does not require a provider key.

## Non-Goals

- A multi-region or active-active architecture.
- Operating a hosted BuildMax SaaS.
- A mandatory Helm chart before the reference architecture is understood.
- Taking responsibility for managed database, object-store, or certificate
  operations outside the documented integration boundary.

## Options To Evaluate

| Option | Strength | Main concern |
|---|---|---|
| Publish YAML plus an operations guide | Transparent and close to existing manifests | Consumers must adapt it carefully for their platform |
| Ship Helm as the first production artifact | Familiar installation surface | Can conceal unresolved lifecycle and security decisions behind values |
| Support only a managed platform recipe | Narrow test surface | Excludes many private deployment environments |
| Define a platform-neutral contract, then add packaging | Separates the real requirements from installer choice | Requires disciplined documentation and verification |

The likely direction is a platform-neutral reference contract expressed first
through readable Kubernetes manifests and an operator guide. Helm or Kustomize
may package that contract after its lifecycle guarantees are agreed.

## Questions To Resolve

- What availability and recovery targets are realistic for the first beta?
- Which database migrations are automatic, and when must an operator take a
  backup or approve a disruptive migration?
- Which server, Worker, Portal, and dependency metrics are required for a
  deployment to be supportable?
- How are JWT, Worker, database, storage, and model credentials injected and
  rotated without embedding them in ConfigMaps or images?
- Which versions of Kubernetes, MySQL, and S3-compatible storage form the
  supported matrix?

## Evidence Needed For A Decision

- An end-to-end deployment against external MySQL and object storage.
- A restore exercise proving that a backed-up database and artifact store can
  recover a team and completed run.
- An upgrade and rollback exercise across at least one schema change.
- Readiness tests that fail when a required dependency is unavailable.

## Likely Destination If Accepted

The committed architecture and operational guarantees belong in the P3 design
and deployment documentation. Packaging, observability, and lifecycle work
should then become separately verifiable implementation Issues.
