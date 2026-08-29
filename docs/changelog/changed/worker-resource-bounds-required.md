- A deployment running workers as Kubernetes Jobs must now set all four
  `worker.k8s.resources` bounds, and the server refuses to start when one is
  missing, is not a Kubernetes quantity, is zero or negative, or names a limit
  below its own request. The error names the key to edit. Previously a typo such
  as `memory_limit: 4 gigabytes` was logged and dropped, which left worker pods
  running model-chosen commands with no memory limit while the configuration
  looked correct.
