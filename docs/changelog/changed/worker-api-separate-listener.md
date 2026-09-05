- The worker control API (`/api/worker/*`) is now served on a separate internal
  listener, off the public HTTP surface. It binds `127.0.0.1:5679` by default,
  so set `worker.server_url` to that listener (via `worker_api.listen`) rather
  than the public port; the public listener answers `404` for worker routes.
