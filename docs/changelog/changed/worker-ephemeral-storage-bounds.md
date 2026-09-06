- Kubernetes worker pods now carry an ephemeral-storage request and limit, and
  each of their scratch volumes is capped at that limit, so a runaway workspace
  is evicted cleanly instead of filling the node. A `k8s_job` deployment must add
  `ephemeral_storage_request` and `ephemeral_storage_limit` under
  `worker.k8s.resources`, which are now required alongside the CPU and memory
  bounds.
