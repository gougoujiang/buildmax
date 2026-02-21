# Design 065: Deployment configs for buildmax server on kind cluster

## Goal

Provide plain Kubernetes YAML to deploy the buildmax server (and in-pod worker) into an existing kind cluster with in-cluster MinIO and MySQL; add `make deploy` (build → pub_images → apply); add a Dockerfile for the React portal. API is exposed at `buildmax-api.kind.local`; `buildmax.kind.local` is reserved for the portal.

## Modules

| Area | Responsibility | Artifacts |
|------|----------------|-----------|
| **setup/** | Kubernetes manifests for buildmax app in kind | Namespace, Secret, Deployment, Service, Ingress |
| **make** | Build, load image, apply manifests in one command | New target `deploy` |
| **Portal image** | Build and serve React/Vite static assets | `Dockerfile.portal` |

No new Go packages; config and server binaries unchanged.

## Structure

### 1. Manifest layout

Place all buildmax-app manifests under **`setup/`** (alongside existing `minio.yaml`, `mysql.yaml`, `test-ingress-whoami.yaml`). Two options:

- **Option A (recommended)**: Single file `deployment/buildmax-deploy.yaml` containing Namespace, Secret, Deployment, Service, and Ingress separated by `---`. Easier for `make deploy` (one `kubectl apply -f`).
- **Option B**: Split files, e.g. `setup/buildmax-namespace.yaml`, `setup/buildmax-secret.yaml`, `setup/buildmax-deployment.yaml`, `setup/buildmax-service.yaml`, `setup/buildmax-ingress.yaml`; `make deploy` applies a directory (`kubectl apply -f setup/` or a list of files). Order matters for namespace first, then Secret, then workload.

**Choice**: Option A — one file `deployment/buildmax-deploy.yaml` for simplicity. If the file grows too large, split later.

### 2. Namespace

- **Name**: `buildmax`.
- Single `Namespace` resource; all buildmax app resources (Secret, Deployment, Service, Ingress) use `namespace: buildmax`.

### 3. Secret

- **Name**: `buildmax-secret` (or `buildmax-server-secret`).
- **Namespace**: `buildmax`.
- **Keys** (opaque or `type: Opaque`; values base64-encoded):
  - `BUILDMAX_JWT_SECRET`: JWT signing secret (required by server).
  - `BUILDMAX_WORKER_TOKEN`: Token for worker-to-server auth (required for `/api/worker/*`).
- **Documentation**: In `setup/README.md` (or comment in YAML), document how to create/update the Secret, e.g.:
  - `kubectl create secret generic buildmax-secret -n buildmax --from-literal=BUILDMAX_JWT_SECRET=dev-secret --from-literal=BUILDMAX_WORKER_TOKEN=dev-worker-token`
  - Or embed a placeholder in the YAML (base64) for dev; user can replace with `kubectl edit secret buildmax-secret -n buildmax` or recreate.

### 4. Deployment

- **Name**: `buildmax-server` (or `buildmax`).
- **Namespace**: `buildmax`.
- **Replicas**: 1.
- **Pod template**:
  - **Container name**: e.g. `server`.
  - **Image**: `buildmax:local` (same as `./make pub_images`).
  - **Command**: Override image ENTRYPOINT to run the server binary: `["/usr/local/bin/buildmax-server"]`. (Image default is `buildmax`; we need the server.)
  - **Port**: `containerPort: 5678`.
  - **Env** (from ConfigMap or inline; secrets from Secret):
    - **MySQL**: `BUILDMAX_DB_HOST=mysql.db.svc.cluster.local`, `BUILDMAX_DB_PORT=3306`, `BUILDMAX_DB_USER=buildmax`, `BUILDMAX_DB_PASSWORD=buildmax`, `BUILDMAX_DB_DATABASE=buildmax`.
    - **MinIO**: `BUILDMAX_MINIO_ENDPOINT=http://minio.storage.svc.cluster.local:9000`, `BUILDMAX_MINIO_ACCESS_KEY=minio`, `BUILDMAX_MINIO_SECRET_KEY=minio123`, `BUILDMAX_MINIO_BUCKET=bmstore`, `BUILDMAX_MINIO_REGION=us-east-1`, `BUILDMAX_MINIO_PREFIX=workspaces`.
    - **Storage**: `BUILDMAX_PERSIST_STORAGE=minio`, `BUILDMAX_ARTIFACT_STORAGE=minio`.
    - **Server**: `BUILDMAX_SERVER_PORT=5678`, `BUILDMAX_WORKSPACES_DIR=/buildmax/worker/workspaces`, **`BUILDMAX_CORS_ORIGIN=http://buildmax.kind.local`** (so the portal at buildmax.kind.local can call the API); `BUILDMAX_JWT_SECRET` and `BUILDMAX_WORKER_TOKEN` from Secret (`valueFrom.secretKeyRef`).
    - **Worker (same pod)**: `BUILDMAX_SERVER_URL=http://buildmax.buildmax.svc.cluster.local:5678` (Service name `buildmax` in namespace `buildmax` — see Service below). Same MinIO and token env.
  - **Volume**: One volume `worker-workspaces`, type `emptyDir`, mounted at `/buildmax/worker/workspaces`.
  - **Resources**: Optional `requests`/`limits` (e.g. memory 256Mi, cpu 100m) to avoid resource issues; keep minimal for dev.
- **Service name** used for in-cluster URL: `buildmax` (so FQDN is `buildmax.buildmax.svc.cluster.local`).

### 5. Service

- **Name**: `buildmax`.
- **Namespace**: `buildmax`.
- **Type**: ClusterIP.
- **Selector**: Match Deployment pod labels (e.g. `app: buildmax` or `app: buildmax-server`).
- **Port**: `5678` → `targetPort: 5678`, name e.g. `http`.

### 6. Ingress

- **Name**: e.g. `buildmax-api`.
- **Namespace**: `buildmax`.
- **Annotations**: `kubernetes.io/ingress.class: nginx` (or omit if cluster default is nginx).
- **ingressClassName**: `nginx` (match setup’s ingress-nginx).
- **Rules**: Single rule, host `buildmax-api.kind.local`, path `/` (Prefix), backend Service `buildmax` port 5678.
- **Documentation**: User must add `127.0.0.1 buildmax-api.kind.local` to `/etc/hosts`; `buildmax.kind.local` reserved for portal.

### 7. Dockerfile for portal

- **Location**: Repo root **`Dockerfile.portal`** (parallel to `Dockerfile.buildmax`). Build context: repo root so we can `COPY portal/ ./portal/` and optionally `COPY` shared files if needed; portal build only needs `portal/`.
- **Stage 1 — build**: 
  - Base: `node:20-alpine` (or match portal’s Node version; LTS).
  - `WORKDIR /app`; `COPY portal/package.json portal/package-lock.json* ./`; `RUN npm ci --omit=dev` or `npm install` for deps; `COPY portal/ ./`; `RUN npm run build`. Output: `dist/` (Vite default per `portal/vite.config.ts`: `outDir: 'dist'`).
- **Stage 2 — serve**:
  - Base: `nginx:alpine` (or `caddy`, or minimal `node` with `npx serve`; nginx is simple and small).
  - Copy built assets from stage 1: `COPY --from=build /app/dist /usr/share/nginx/html` (nginx default) or equivalent.
  - For SPA (React Router): nginx config to serve `index.html` for non-file requests (e.g. `try_files $uri $uri/ /index.html`). Options: (a) custom `nginx.conf` in repo, copied into image; (b) default nginx.conf may need a `location / { try_files $uri $uri/ /index.html; }` — add a small `portal/nginx.conf` or `setup/portal-nginx.conf` and `COPY` into image.
- **Image tag**: e.g. `buildmax-portal:local`; not loaded into kind in this task (document only).

### 8. `make deploy`

- **Target**: `deploy` in root `make` script.
- **Steps** (in order):
  1. Build Go binaries: call existing `cmd_build` or run `./make build` (subshell).
  2. Build and load server image into kind: call `cmd_pub_images` or run `./make pub_images`.
  3. Apply buildmax manifests: `kubectl apply -f "$SCRIPT_DIR/deployment/buildmax-deploy.yaml"`. Use default kubeconfig (kind cluster already selected) or set `KUBECONFIG` / context via `BUILDMAX_KIND_CLUSTER` if needed; kind typically sets context by cluster name, so no extra switch if user has run `./make setup` and context is current.
- **Prerequisites**: Document that cluster and dependencies must exist (`./make setup`); Secret must exist or be created (document in README).
- **Usage**: Add to `make` usage string: `deploy — Build, load image, and deploy buildmax server to kind cluster (run ./make setup first)`.
- **Implementation**: New function `cmd_deploy()` that runs the three steps; add `deploy)` case in the `case "$cmd" in` block.

## Method design

### make script

- **cmd_deploy()**: 
  - `cmd_build` (or invoke `./make build`); on failure exit 1.
  - `cmd_pub_images` (or invoke `./make pub_images`); on failure exit 1.
  - `kubectl apply -f "$SCRIPT_DIR/deployment/buildmax-deploy.yaml"`; on failure exit 1.
  - Echo success message (e.g. “Deployed. Ensure 127.0.0.1 buildmax-api.kind.local is in /etc/hosts, then open http://buildmax-api.kind.local”).
- **usage()**: Add line for `deploy`.
- **case**: Add `deploy) cmd_deploy ;;`.

### Dockerfile.portal (concrete)

- **Build stage**: `FROM node:20-alpine AS build`; `WORKDIR /app`; `COPY portal/package.json portal/package-lock.json* ./`; `RUN npm ci` (or `npm install` if no lock); `COPY portal/ ./`; `RUN npm run build`.
- **Serve stage**: `FROM nginx:alpine`; `COPY --from=build /app/dist /usr/share/nginx/html`; add config for SPA fallback. Minimal nginx override: create `portal/nginx-default.conf` with `server { root /usr/share/nginx/html; location / { try_files $uri $uri/ /index.html; } ... }` and `COPY portal/nginx-default.conf /etc/nginx/conf.d/default.conf` (replace default) so React Router works.
- **Expose**: `EXPOSE 80`; `CMD ["nginx", "-g", "daemon off;"]`.

## How they work together

1. User runs `./make setup` (once): kind cluster, MinIO, MySQL, ingress-nginx, bucket exist.
2. User creates Secret (documented): `kubectl create secret generic buildmax-secret -n buildmax ...` or apply a manifest that contains the Secret (with placeholder or generated values).
3. User runs `./make deploy`: build → pub_images → `kubectl apply -f deployment/buildmax-deploy.yaml`. Namespace `buildmax` is created; Secret must already exist or be in the same YAML; Deployment, Service, Ingress are applied.
4. In-cluster: Pod runs `buildmax-server`; env points to `mysql.db.svc.cluster.local` and `minio.storage.svc.cluster.local`; worker subprocess uses `BUILDMAX_WORKSPACES_DIR=/buildmax/worker/workspaces` (emptyDir). Scheduler spawns worker; worker calls `http://buildmax.buildmax.svc.cluster.local:5678` and uses same MinIO/DB env.
5. From host: User has `127.0.0.1 buildmax-api.kind.local` in `/etc/hosts`. Requests to `http://buildmax-api.kind.local` hit ingress-nginx (port 80) → Ingress → Service `buildmax` → Pod 5678.
6. Portal: `docker build -f Dockerfile.portal -t buildmax-portal:local .` produces an image that serves the built portal; future task can deploy it at `buildmax.kind.local`.

## Tests

- No automated tests required by task. Manual verification: after `make deploy`, `kubectl get pods -n buildmax` shows Running; `curl http://buildmax-api.kind.local/healthz` (or equivalent) returns 200 after `/etc/hosts` is set. Portal image: `docker build -f Dockerfile.portal -t buildmax-portal:local .` and `docker run -p 8080:80 buildmax-portal:local`; open http://localhost:8080.

## Changes for review

- **deployment/buildmax-deploy.yaml** (new): Single YAML with Namespace `buildmax`, Secret `buildmax-secret` (or documented separate creation), Deployment (buildmax-server container, image buildmax:local, command buildmax-server, env as above, emptyDir at /buildmax/worker/workspaces), Service `buildmax` (ClusterIP 5678), Ingress host buildmax-api.kind.local → Service buildmax.
- **make**: Add `cmd_deploy` (build, pub_images, kubectl apply -f deployment/buildmax-deploy.yaml); add `deploy` to usage and case.
- **Dockerfile.portal** (new, repo root): Multi-stage; stage 1 Node build portal (npm ci, npm run build), stage 2 nginx serving dist; include nginx config for SPA fallback (e.g. portal/nginx-default.conf).
- **portal/nginx-default.conf** (or similar, new): Minimal nginx server block, root /usr/share/nginx/html, try_files for SPA.
- **setup/README.md**: Update “Access buildmax-server” to “Deploy buildmax server in cluster”: prereq `./make setup`; create Secret (command or YAML); run `./make deploy`; add `127.0.0.1 buildmax-api.kind.local` to /etc/hosts; API at http://buildmax-api.kind.local. Note `buildmax.kind.local` reserved for portal. Remove or adjust old “run server on host + buildmax-ingress” if that file does not exist; align with in-cluster deploy.
