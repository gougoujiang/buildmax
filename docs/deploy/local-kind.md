# Local setup (kind + MinIO + MySQL + Redis + Ingress)

From the repo root, run:

```bash
./make setup
```

This script is idempotent and will:

- Install kind, helm, kubectl, awscli via Homebrew if missing
- Create a kind cluster from `kind-config.yaml` (with Ingress port mapping 80/443)
- Deploy ingress-nginx from local manifest `kind-ingress-nginx.yaml` (no network to GitHub required)
- Deploy whoami in namespace `test` for ingress testing
- Create namespace `storage`, deploy MinIO, create bucket `bmstore`
- Deploy MySQL in namespace `db` and start port-forward to localhost:3306
- Deploy Redis in namespace `db` and start port-forward to localhost:6379
- Start MinIO port-forwards (9000, 9001) and create the S3 bucket
- Run a small test job

Use `./make unsetup` to tear down the cluster and stop port-forwards.

## Ingress and /etc/hosts

The setup installs [ingress-nginx](https://kubernetes.github.io/ingress-nginx/) so you can expose services via Ingress on host ports 80 and 443.

For Ingress hostnames under `*.kind.local`, add one line per host to `/etc/hosts` (there is no wildcard support). Example:

```text
127.0.0.1 buildmax-api.kind.local
127.0.0.1 buildmax.kind.local
127.0.0.1 whoami.kind.local
```

- **buildmax-api.kind.local** — API server (when deployed in cluster via `./make deploy`).
- **buildmax.kind.local** — Reserved for the portal (React) app.

Then apply the Ingress manifests for the hosts you use (see below).

**Existing clusters:** If the kind cluster was created before Ingress was added to this setup, it will not have port 80/443 mapped. Run `./make unsetup` then `./make setup` to recreate the cluster with Ingress support.

### Deploy buildmax server in cluster

> Configuration comes from the `buildmax-config` ConfigMap in
> `deployment/buildmax-deploy.yaml`, mounted as `/buildmax/server.yaml` in both
> the server pod and every worker Job pod. Credentials come from
> `buildmax-secret` as environment overrides — see
> [overview.md](overview.md) and
> [reference/configuration.md](../reference/configuration.md). Edit the
> ConfigMap, not the container environment, when changing hosts or backends.

To run the buildmax API server inside the kind cluster (using in-cluster MySQL and MinIO):

1. **Prereq**: Run `./make setup` once (cluster, MinIO, MySQL, ingress-nginx, bucket must exist).
2. **Deploy**: From repo root run `./make deploy`. This builds the binaries, builds and loads the `buildmax:local` image into kind, and applies `deployment/buildmax-deploy.yaml` (namespace `buildmax`, Secret, Deployment, Service, Ingress).
3. **Secrets**: The manifest includes a dev Secret with placeholder values. For production, create your own before applying:

   ```bash
   kubectl create secret generic buildmax-secret -n buildmax \
     --from-literal=BUILDMAX_JWT_SECRET="$(openssl rand -base64 32)" \
     --from-literal=BUILDMAX_WORKER_TOKEN="$(openssl rand -hex 24)" \
     --from-literal=BUILDMAX_DATABASE_PASSWORD=buildmax \
     --from-literal=BUILDMAX_STORAGE_MINIO_ACCESS_KEY=minio \
     --from-literal=BUILDMAX_STORAGE_MINIO_SECRET_KEY=minio123 \
     --from-literal=BUILDMAX_CONVERSATION_MODEL_API_KEY=your-llm-api-key
   ```

   Or delete the Secret from the YAML and apply it separately.
4. **Hosts**: Add to `/etc/hosts`: `127.0.0.1 buildmax-api.kind.local`. Then open **<http://buildmax-api.kind.local>** (e.g. `/healthz` or the Portal pointing its API base to this host). The portal host **buildmax.kind.local** is reserved for when you deploy the portal (e.g. via the `deployment/docker/Dockerfile.portal` image).

To remove the buildmax app: `kubectl delete -f deployment/buildmax-deploy.yaml`

### Test Ingress with whoami

`./make setup` deploys whoami in namespace `test` automatically. Add to `/etc/hosts`:

```text
127.0.0.1 whoami.kind.local
```

Then request:

```bash
curl http://whoami.kind.local
```

You should see whoami’s response (hostname, headers, etc.). Remove the test resources with:

```bash
kubectl delete -f setup/test-ingress-whoami.yaml
```

**If you get ERR_CONNECTION_RESET:** The ingress controller must run on the control-plane node (so host port 80 reaches it). Our manifest pins it with `nodeSelector: ingress-ready: "true"`. If your cluster was created before that fix, re-apply and restart the controller:

```bash
kubectl delete deployment ingress-nginx-controller -n ingress-nginx
kubectl apply -f setup/kind-ingress-nginx.yaml
kubectl wait --for=condition=Available deployment/ingress-nginx-controller -n ingress-nginx --timeout=120s
```

If the cluster was created before `kind-config.yaml` had `extraPortMappings` (80/443), recreate the cluster: `./make unsetup` then `./make setup`.
