# Local setup (kind + MinIO + MySQL + Ingress)

From the repo root, run:

```bash
./make setup
```

This script is idempotent and will:

- Install kind, helm, kubectl, awscli via Homebrew if missing
- Create a kind cluster from `kind-config.yaml` (with Ingress port mapping 80/443)
- Deploy ingress-nginx from local manifest `ingress-nginx-kind.yaml` (no network to GitHub required)
- Deploy whoami in namespace `test` for ingress testing
- Create namespace `storage`, deploy MinIO, create bucket `bmstore`
- Deploy MySQL in namespace `db` and start port-forward to localhost:3306
- Start MinIO port-forwards (9000, 9001) and create the S3 bucket
- Run a small test job

Use `./make unsetup` to tear down the cluster and stop port-forwards.

## Ingress and /etc/hosts

The setup installs [ingress-nginx](https://kubernetes.github.io/ingress-nginx/) so you can expose services via Ingress on host ports 80 and 443.

For Ingress hostnames under `*.kind.local`, add one line per host to `/etc/hosts` (there is no wildcard support). Example:

```
127.0.0.1 buildmax.kind.local
127.0.0.1 whoami.kind.local
```

Then apply the Ingress manifests for the hosts you use (see below).

**Existing clusters:** If the kind cluster was created before Ingress was added to this setup, it will not have port 80/443 mapped. Run `./make unsetup` then `./make setup` to recreate the cluster with Ingress support.

### Access buildmax-server at buildmax.kind.local

To reach the buildmax HTTP server (running on the host on port 5678) via `http://buildmax.kind.local`:

1. Add to `/etc/hosts`: `127.0.0.1 buildmax.kind.local`
2. Start the server on the host: `./make run server`
3. Apply the Ingress and proxy: `kubectl apply -f setup/buildmax-ingress.yaml`

Traffic flows: browser → localhost:80 → ingress-nginx → buildmax-proxy pod → `host.docker.internal:5678` → your server. On Docker Desktop (Mac/Windows) `host.docker.internal` is available; on Linux you may need to configure the proxy or run the server inside the cluster.

To remove: `kubectl delete -f setup/buildmax-ingress.yaml`

### Test Ingress with whoami

`./make setup` deploys whoami in namespace `test` automatically. Add to `/etc/hosts`:

```
127.0.0.1 whoami.kind.local
```

Then request:

```bash
curl http://whoami.kind.local
```

You should see whoami’s response (hostname, headers, etc.). Remove the test resources with:

```bash
kubectl delete -f setup/whoami-ingress-test.yaml
```

**If you get ERR_CONNECTION_RESET:** The ingress controller must run on the control-plane node (so host port 80 reaches it). Our manifest pins it with `nodeSelector: ingress-ready: "true"`. If your cluster was created before that fix, re-apply and restart the controller:

```bash
kubectl delete deployment ingress-nginx-controller -n ingress-nginx
kubectl apply -f setup/ingress-nginx-kind.yaml
kubectl wait --for=condition=Available deployment/ingress-nginx-controller -n ingress-nginx --timeout=120s
```

If the cluster was created before `kind-config.yaml` had `extraPortMappings` (80/443), recreate the cluster: `./make unsetup` then `./make setup`.
