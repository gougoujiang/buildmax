- The reference Kubernetes manifests now separate the worker control API from
  the public API: a `buildmax-api` Service behind the Ingress, an internal
  `buildmax-worker-api` ClusterIP on port 5679, a `NetworkPolicy` that admits
  only labelled worker pods to that port, and the worker-api CA mounted
  read-only into each worker Job.
