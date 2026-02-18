# Design 063: Add ingress controller to kind

## Goal

Enable Ingress-based access in the kind cluster by adding ingress-nginx, kind port mapping for 80/443, and documentation for local `/etc/hosts`, so that `./make setup` yields a cluster ready for Ingress testing.

## Modules

| Module | Responsibility | Owns |
|--------|----------------|------|
| **setup/kind-config.yaml** | Kind cluster definition with Ingress-ready control-plane (port mapping, node label). | YAML schema; no code. |
| **setup/setup.sh** | Idempotent setup orchestration; new step deploys ingress-nginx and logs /etc/hosts hint. | Functions `ensure_*`, `main`, `log`. |
| **setup/README.md** (optional) | Short setup documentation including Ingress and /etc/hosts. | Single doc for setup directory. |

## Structure

**Files**

- `setup/kind-config.yaml` — Kind cluster config: control-plane + worker; control-plane gets `extraPortMappings` (80, 443) and `ingress-ready=true` via kubeadmConfigPatches.
- `setup/setup.sh` — Adds `ensure_ingress()` and calls it from `main()` after `ensure_kind_cluster`; extends final log with Ingress /etc/hosts line.
- `setup/README.md` — Optional: one short section describing Ingress and the need to add `127.0.0.1 buildmax.kind.local` (and similar) to `/etc/hosts`; note that existing clusters need unsetup + setup for port mapping.

**No new Go packages or binaries.** All changes are under `setup/`.

## Kind config design

**File**: `setup/kind-config.yaml`

- Keep `kind: Cluster`, `apiVersion: kind.x-k8s.io/v1alpha4`, and `nodes` list.
- **Control-plane node** (first entry under `nodes`):
  - Add `kubeadmConfigPatches` with one patch that sets `nodeRegistration.kubeletExtraArgs.node-labels: "ingress-ready=true"` (InitConfiguration).
  - Add `extraPortMappings`:
    - `containerPort: 80`, `hostPort: 80`, `protocol: TCP`
    - `containerPort: 443`, `hostPort: 443`, `protocol: TCP`
- **Worker node** — unchanged (role only).
- Add a top-level comment (or short comment above control-plane) stating that clusters created before this change do not have these ports; run `./make unsetup` then `./make setup` to recreate.

Exact structure follows the [ingress-nginx kind documentation](https://kubernetes.github.io/ingress-nginx/deploy/#kind): control-plane gets the patch and port mappings so the kind-specific ingress-nginx DaemonSet can bind to 80/443 on the host.

## Setup script design

**File**: `setup/setup.sh`

**New function: `ensure_ingress()`**

- **Idempotency**: If the ingress-nginx controller deployment (or namespace) already exists, log "Ingress controller already present" (or equivalent) and return. Otherwise apply and wait.
- **Apply**: `kubectl apply -f https://raw.githubusercontent.com/kubernetes/ingress-nginx/main/deploy/static/provider/kind/deploy.yaml`
- **Wait**: After apply, wait for the controller to be ready. The kind deploy creates namespace `ingress-nginx` and a Deployment (name as in the official manifest, e.g. `ingress-nginx-controller`). Use:
  - `kubectl wait --for=condition=Available deployment/ingress-nginx-controller -n ingress-nginx --timeout=120s` (or the actual deployment name from the manifest; typically `ingress-nginx-controller`).
- **Logging**: Log "Applying ingress-nginx (kind)..." before apply; log "Ingress controller ready" or similar after wait. On idempotent skip, log once that controller is already present.

**`main()` changes**

- After `ensure_kind_cluster` and `kubectl get nodes`, call `ensure_ingress()` so Ingress is ready before storage/MySQL/port-forwards (order is flexible; after cluster is sufficient).
- In the final "Setup done" block, add one line:
  - e.g. `log "For Ingress hostnames (e.g. buildmax.kind.local), add '127.0.0.1 buildmax.kind.local' to /etc/hosts."`

**No changes** to `ensure_brew`, `ensure_kind_cluster`, `ensure_storage`, `ensure_mysql`, `ensure_bucket_via_portforward`, `ensure_test_job`, or to unsetup.

## Documentation

- **Primary**: Setup script’s final log includes the /etc/hosts line as above.
- **Optional**: Create `setup/README.md` with a short "Local setup" section that:
  - Describes `./make setup` and that it installs ingress-nginx.
  - States that for Ingress hostnames under `*.kind.local`, add one line per host to `/etc/hosts`, e.g. `127.0.0.1 buildmax.kind.local`.
  - Notes that if the cluster was created before Ingress was added, run `./make unsetup` then `./make setup` to get port 80/443 mapping.

## How they work together

**Flow**

1. User runs `./make setup` → invokes `setup/setup.sh`.
2. `ensure_kind_cluster` creates or reuses cluster from `kind-config.yaml` (with 80/443 and ingress-ready label when created from updated config).
3. `ensure_ingress` applies the ingress-nginx kind manifest and waits for the controller deployment; subsequent runs skip if already present.
4. Rest of setup (storage, MySQL, port-forwards, test job) runs as today.
5. Final log prints MinIO/MySQL info plus the Ingress /etc/hosts hint.

**Dependencies**

- `setup.sh` reads `SCRIPT_DIR/kind-config.yaml` only in `ensure_kind_cluster` (unchanged). No new file reads for ingress (manifest is from URL).
- kind and kubectl are already required by existing setup.

**Edge cases**

- **Existing cluster without port mapping**: If the cluster was created before kind-config had extraPortMappings, the controller will run but host 80/443 will not reach it. Documented in kind-config comment and (if present) setup/README: user must `./make unsetup` then `./make setup`.
- **Network**: `kubectl apply -f <url>` requires network access; same as today for `brew install` and in-cluster pulls.

## Changes for review

- **Modified**: `setup/kind-config.yaml` — Add control-plane `kubeadmConfigPatches` (node label `ingress-ready=true`) and `extraPortMappings` for 80 and 443; add comment about recreating existing clusters.
- **Modified**: `setup/setup.sh` — New function `ensure_ingress()` (apply ingress-nginx kind manifest, wait for deployment, idempotent skip); call `ensure_ingress` from `main()` after `ensure_kind_cluster`; add one line to final log for /etc/hosts.
- **New** (optional): `setup/README.md` — Short doc with Ingress and /etc/hosts instructions and note on existing-cluster recreation.
