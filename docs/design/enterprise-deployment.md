# Enterprise Deployment Loop

## Status

- roadmap_priority: `P3`
- status: `in_progress` — M1 (config contract cleanup) is done; M2–M5 are open
- follows: P2 Portal outcome surface (complete; plan retired)
- roadmap: [../ROADMAP.md](../ROADMAP.md)
- created_at: `2026-05-17`

## 1. Decision

After P2 makes Portal results first-class, the next platform move is P3:
make private deployment boring, repeatable, and diagnosable.

BuildMax already has the main runtime pieces:

- server binary
- worker binary
- scheduler
- Portal image
- MySQL persistence
- local FS and MinIO/S3 blob storage
- kind setup
- Kubernetes deployment YAML

The gap is deployment closure. A new environment should be able to reach this
happy path without reading code:

1. start infrastructure
2. start server and Portal
3. log in
4. create team work
5. run a worker task
6. view the produced result

## 2. Product Goal

A company or self-hosting user can deploy BuildMax privately with a recommended
path that is explicit, observable, and recoverable.

The deployment loop should feel like:

> I can install it, verify it, and understand failures from clear messages.

not:

> I need to inspect bootstrap code, manifests, and historical docs to discover
> which variables still matter.

## 3. Current Baseline

Current deployment assets:

- `./make setup` creates a kind cluster, MinIO, MySQL, Redis, ingress-nginx, and
  port-forwards.
- `./make pub_images` builds `buildmax:local` and `buildmax-portal:local`, then
  loads them into kind.
- `./make deploy` builds, loads images, applies
  `deployment/buildmax-deploy.yaml`, and restarts server and Portal deployments.
- `deployment/docker/Dockerfile.buildmax` builds the Go binaries into a
  container image.
- `deployment/docker/Dockerfile.portal` builds and serves the Portal static app.
- `docs/deploy/local-kind.md` documents the local kind path.
- `internal/bootstrap/server.go` starts DB, storage, HTTP server, and scheduler.
- `internal/bootstrap/worker.go` loads `server.yaml`, gets a scheduled run from
  the server, executes the agent runtime, and uploads artifacts.
- `internal/config/server_config.go` is the active server/worker config loader.

Current config reality:

- `BUILDMAX_HOME` locates the application data directory.
- `BUILDMAX_HOME/server.yaml` is the main server and worker config file.
- `BUILDMAX_JWT_SECRET` can override `jwt_secret` in `server.yaml`.
- `internal/config/env_spec.go` intentionally lists only bootstrap env vars.
- Docs and manifests were realigned to `server.yaml` (see §4.1).

## 4. Main Gaps

### 4.1 Config Contract Drift — RESOLVED

**This gap is closed.** It was worse than described here: the manifest's env
vars were not merely confusing, they were dead. Nothing read `BUILDMAX_DB_*`,
`BUILDMAX_MINIO_*`, or `BUILDMAX_WORKSPACES_DIR` any more, no `server.yaml` was
supplied to the container, and a deployed server fell back to built-in defaults
and crash-looped against MySQL on `localhost`. Worker Jobs had the same problem.

What shipped:

- `deployment/buildmax-deploy.yaml` carries a `buildmax-config` ConfigMap with
  `server.yaml`, mounted by subPath so the rest of `BUILDMAX_HOME` stays writable
- credentials come from `buildmax-secret` through env overrides on
  `database.password`, `storage.minio.access_key`/`secret_key`, `worker.token`,
  and `conversation.model.api_key`
- worker pods mount the same ConfigMap via `worker.k8s.config_map`, with
  `BUILDMAX_HOME` set to `worker.k8s.home_dir`
- `.env.example` references are gone; `docs/reference/configuration.md` is the
  config reference
- `TestDeploymentConfigMapLoads` fails the build if the manifest and
  `internal/config/server_config.go` drift apart again

Not verified against a live cluster — see §10.

### 4.2 Missing Recommended Deployment Shape

The repo has a kind path, but not a crisp recommended production shape.

We need one blessed architecture:

- Server Deployment
- Portal Deployment
- Worker Jobs launched by the scheduler
- MySQL
- MinIO or external S3
- ConfigMap for non-secret `server.yaml`
- Secret for JWT, worker token, LLM API key, DB password, S3 secret
- Ingress for Portal and API

### 4.3 Health And Startup Diagnostics Are Thin

The server exposes `/healthz`, but P3 needs more explicit readiness:

- database reachable
- object storage reachable
- worker launch mode valid
- worker token configured
- LLM config available for conversation title/runtime paths where required
- storage bucket/prefix readable and writable

### 4.4 Worker End-To-End Path Needs A Single Verification

The acceptance path is not just "server starts".

It must prove:

- scheduler claims pending task runs
- worker starts in the chosen mode
- worker can call server worker API
- worker can materialize team home
- worker can write `artifacts/result.md`
- server can surface the result through artifact endpoints

## 5. In Scope

### 5.1 Deployment Config Standardization

Define one active deployment config contract:

- `server.yaml` is the canonical file for server and worker config.
- env vars are only for:
  - `BUILDMAX_HOME`
  - secret overrides like `BUILDMAX_JWT_SECRET`
  - test-only values
- Kubernetes uses:
  - ConfigMap for `server.yaml`
  - Secret for sensitive values
  - env vars only where the code explicitly supports overrides

### 5.2 Local Kind Path

Make the local kind path fully end-to-end:

```sh
./make setup
./make deploy
```

Then verify:

- Portal opens at `http://buildmax.kind.local`
- API health works at `http://buildmax-api.kind.local/healthz`
- login works
- a task can run through a worker Job
- a result is visible in Portal

### 5.3 Production Reference Path

Add production-oriented deployment docs:

- required services
- config file example
- secrets example
- image tags
- migration expectations
- storage persistence expectations
- ingress/TLS expectations
- backup boundaries

This does not need to become a full Helm chart in the first slice.

### 5.4 Startup Errors And Health Checks

Improve startup and health diagnostics so common misconfiguration has a clear
failure mode.

Required checks:

- `jwt_secret` present
- `workspaces_dir` present
- DB connection succeeds
- storage backend config is valid
- MinIO/S3 client can reach bucket when configured
- worker mode is valid
- `worker.binary` exists for `local_process`
- Kubernetes job creator is available for `k8s_job`
- worker token is present when worker API is enabled

### 5.5 Initial Admin / Team / Quota Story

The first logged-in user and team setup must be obvious.

Minimum story:

- signup/login creates or locates the user's personal team
- default quota tier is seeded
- default team role is `owner`
- the docs explain whether public signup is acceptable for private deployment
- future admin bootstrap is identified if not implemented in P3

## 6. Out Of Scope

- Full Helm chart as the first required deliverable.
- Multi-region deployment.
- HA MySQL design.
- Enterprise SSO.
- Billing.
- Advanced secrets manager integration.
- Full audit/event product.

## 7. Target Deployment Architecture

Recommended MVP topology:

```text
User
  |
  v
Ingress
  |---------------------------|
  v                           v
Portal Service                API Service
Portal Deployment             buildmax-server Deployment
                              |
                              v
                         Scheduler
                              |
                              v
                         Worker Job(s)

Shared dependencies:
  - MySQL
  - MinIO/S3
  - Kubernetes Secret
  - Kubernetes ConfigMap containing server.yaml
```

The server process owns HTTP API and scheduler. Workers are separate processes
or Jobs. Both server and worker read the same config contract.

## 8. Configuration Shape

### 8.1 `server.yaml`

The deployment ConfigMap should mount a complete `server.yaml`.

Example shape:

```yaml
port: 5678
cors_origin: "https://buildmax.example.com"
workspaces_dir: "/buildmax/workspaces"
default_quota_tier: "free_trial"

database:
  host: "mysql"
  port: 3306
  user: "buildmax"
  password: ""
  name: "buildmax"

storage:
  persist_backend: "minio"
  artifact_backend: "minio"
  minio:
    endpoint: "http://minio:9000"
    region: "us-east-1"
    access_key: ""
    secret_key: ""
    bucket: "bmstore"
    prefix: "workspaces"

worker:
  run_mode: "k8s_job"
  server_url: "http://buildmax.buildmax.svc.cluster.local:5678"
  token: ""
  k8s:
    namespace: "buildmax"
    image: "buildmax:local"

conversation:
  model:
    model: "openai/gpt-4o-mini"
    api_url: "https://openrouter.ai/api/v1"
    api_key: ""
    context_window: 0
    call_timeout: 60
```

P3 should decide whether secrets are written into this mounted file, substituted
before mount, or supplied through supported env overrides. The clean target is:

- non-secrets in ConfigMap
- secrets in Secret
- small bootstrap code support for secret env overrides where necessary

### 8.2 Secret Values

Secret values:

- `jwt_secret`
- `worker.token`
- DB password
- S3 access key and secret key
- conversation model API key

Current code only explicitly supports `BUILDMAX_JWT_SECRET` as an env override.
If Kubernetes should keep all secrets out of `server.yaml`, P3 must add env
override support for the other secret fields or a secret-file merge path.

## 9. Implementation Plan

### M1. Config Contract Cleanup — DONE

- ✅ Deployment manifests mount `server.yaml` from a ConfigMap.
- ✅ Stale env vars removed; the ones that remain are overrides the code binds.
- ✅ Documented sample: `config-examples/server.example.yaml` plus
  `docs/reference/configuration.md`.
- ✅ `.env.example` references retired.
- ✅ `README.md`, `CONTRIBUTING.md`, and the kind guide (now
  `docs/deploy/local-kind.md`) match the YAML config contract.

Acceptance, both met:

- a new reader can tell exactly where each config value comes from —
  file for shape, environment for credentials
- `deployment/buildmax-deploy.yaml` matches `internal/config/server_config.go`,
  now enforced by a test rather than by review

### M2. Kind End-To-End Path

- Ensure `./make deploy` creates/mounts valid config for server and worker.
- Ensure server and worker containers share the same image/config assumptions.
- Add a smoke checklist or script for:
  - `/healthz`
  - login
  - create conversation or issue
  - create task run
  - wait for worker
  - fetch result/artifact
- Keep the script idempotent where possible.

Acceptance:

- `./make setup && ./make deploy` reaches a visible task result in Portal

### M3. Health And Readiness

- Keep `/healthz` as a cheap liveness check.
- Add a readiness endpoint or enrich health output behind an authenticated/admin
  route.
- Check DB and storage dependencies.
- Add clear startup errors for worker-mode and storage misconfiguration.
- Add Kubernetes readiness/liveness probes.

Acceptance:

- a broken DB, missing bucket, missing worker token, or invalid worker mode has
  a clear failing check

### M4. Production Reference Guide

- Add a deployment guide under `deployment/README.md`.
- Document:
  - images
  - config
  - secrets
  - storage
  - database
  - ingress/TLS
  - worker mode
  - backup boundaries
  - upgrade steps
- Keep the guide concrete but not cloud-specific.

Acceptance:

- a private deployment can be planned from docs without reading Go bootstrap
  code

### M5. Admin Bootstrap Story

- Document current signup/team creation behavior.
- Decide whether private deployments need:
  - invite-only signup
  - first-user-is-admin
  - bootstrap admin config
- Implement the smallest required control if public signup is not acceptable.

Acceptance:

- operators understand how the first user, team, role, and quota tier are
  created

## 10. Validation

Code validation:

```sh
go test ./internal/config ./internal/bootstrap ./internal/server/handlers ./internal/server/scheduler
```

Full validation:

```sh
./make test
```

Deployment validation:

```sh
./make setup
./make deploy
curl http://buildmax-api.kind.local/healthz
```

Manual product validation:

1. Open Portal.
2. Log in.
3. Create or select a team.
4. Create an issue or conversation task.
5. Run work.
6. Confirm the worker completes.
7. Confirm result/artifact is visible.

## 11. Risks

- **Config split confusion**: YAML plus env overrides can become unclear unless
  docs and code define precedence precisely.
- **Secret leakage**: current dev manifests include placeholder secrets; P3 must
  clearly mark dev-only values and provide a production-safe path.
- **Kind success hiding production gaps**: the local path should be a reference,
  not the only documented shape.
- **Worker mode drift**: local process and Kubernetes Job modes must use the
  same run lifecycle and storage assumptions.
- **Silent storage failure**: storage checks must be explicit because result
  visibility depends on artifacts.

## 12. Open Questions

1. Should BuildMax support env overrides for all secret fields in `server.yaml`,
   or should Kubernetes mount a rendered secret-backed config file?
2. Should P3 introduce `GET /readyz`, or should `/healthz` become dependency
   aware?
3. Should Redis remain in setup if the current server path does not require it?
4. Should the recommended production path use Kubernetes Jobs only, or document
   `local_process` as a single-node option?
5. Should private deployments allow self-signup by default?

## 13. Recommended First PR

The first P3 PR should fix the config/deployment mismatch:

1. Add a deployment `server.yaml` sample.
2. Mount it in `deployment/buildmax-deploy.yaml`.
3. Move unsupported env-var config out of the manifest.
4. Keep secrets explicit and dev-only values clearly labeled.
5. Update setup/deployment docs.
6. Verify `./make setup && ./make deploy` reaches `/healthz`.

That gives the rest of P3 a stable deployment contract to build on.
