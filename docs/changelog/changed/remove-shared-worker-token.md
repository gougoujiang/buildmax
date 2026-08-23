- The deployment-wide `worker.token` is gone. Every `/api/worker/*` route now
  takes only the run token the server mints at dispatch, so a worker can read
  and write its own run and nothing else. `worker.token` and
  `BUILDMAX_WORKER_TOKEN` are no longer read, and the secret has been dropped
  from the Compose and Kubernetes manifests — remove it from yours. Upgrade the
  server before the worker image: a run dispatched without a run token now
  fails immediately instead of falling back.
