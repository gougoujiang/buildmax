- The worker control listener now supports TLS, and a worker reaches the server
  through one HTTP client that verifies the server certificate against a
  configured CA (`worker.server_ca_file`) with no insecure fallback. A
  `k8s_job` whose `worker.server_url` is `http://` is refused at startup unless
  `worker.allow_insecure_http` is set.
