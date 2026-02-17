# Development Guide

## Tech stack
- language: golang

## Build, test and run
- please use ./make script
- testing-sandbox will be created in local workspace for testing runs

## LLM
- We use openrouter free models, might got 429 if called too frequently

## Setup Kind, MinIO and MySQL

One-click setup (idempotent, safe to run multiple times):

```bash
./make setup
```

This installs (via Homebrew if missing) kind, helm, kubectl, awscli; creates a kind cluster (default name `buildmaxdev`, override with `BUILDMAX_KIND_CLUSTER`); deploys **MinIO** in namespace `storage` and **MySQL** in namespace `db`; starts port-forwards (MinIO 9000/9001, MySQL 3306); creates bucket `s3://bmstore`; and runs the in-cluster S3 test job. It also runs `docker compose down` so any existing Compose MySQL is stopped and port 3306 is free. Tear down:

```bash
./make unsetup
```

This deletes the kind cluster and stops MinIO/MySQL port-forwards. Brew-installed tools are not removed.

All resource files live under `setup/`:

- `setup/kind-config.yaml` — kind cluster config (control-plane + worker)
- `setup/minio.yaml` — MinIO Deployment and Service
- `setup/mysql.yaml` — MySQL 8.0 Deployment and Service (same defaults as former docker-compose: user `buildmax`, DB `buildmax`)
- `setup/test-minio-job.yaml` — Job that lists S3 from inside the cluster

After setup, connect to MySQL at `localhost:3306` (or in-cluster: `mysql.db.svc.cluster.local:3306`).

Manual steps (for reference; normally use `./make setup`):

- Cluster: `kind create cluster --name buildmaxdev --config setup/kind-config.yaml`
- MinIO: `kubectl create namespace storage` then `kubectl apply -f setup/minio.yaml`
- MySQL: `kubectl apply -f setup/mysql.yaml` (creates namespace `db`)
- Port-forwards: `kubectl port-forward svc/minio 9000:9000 9001:9001 -n storage`; `kubectl port-forward svc/mysql 3306:3306 -n db`
- Env: `AWS_ACCESS_KEY_ID=minio`, `AWS_SECRET_ACCESS_KEY=minio123`, `AWS_DEFAULT_REGION=us-east-1`
- Bucket: `aws --endpoint-url http://localhost:9000 s3 mb s3://bmstore` then `s3 ls`
- In-cluster MinIO: `http://minio.storage.svc.cluster.local:9000`
- Test job: `kubectl apply -f setup/test-minio-job.yaml` then `kubectl logs job/s3-test`

### BuildMax container image

To build a container image for the BuildMax binary and load it into your kind cluster (e.g. for running the server or executor in-cluster):

- **Dockerfile**: `Dockerfile.buildmax` at repo root (for the binary; a future `Dockerfile.portal` will be for the Portal).
- **Build**: `docker build -f Dockerfile.buildmax -t buildmax:local .`
- **Load into kind**: `kind load docker-image buildmax:local --name buildmaxdev` (use `BUILDMAX_KIND_CLUSTER` if you override the cluster name).
- **One-step**: `./make pub_images` builds the image and loads it into the kind cluster used by `./make setup`.

To build for a different platform (e.g. `linux/amd64` on an Apple Silicon Mac), set `BUILDMAX_IMAGE_PLATFORM` before running: `BUILDMAX_IMAGE_PLATFORM=linux/amd64 ./make pub_images`.