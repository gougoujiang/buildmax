- The server now stops in order on SIGINT or SIGTERM: it reports itself
  unready so a load balancer stops sending it work, ends the streams watching a
  run so the Portal reopens them elsewhere, refuses new conversation turns and
  waits for the ones running, drains in-flight requests, and only then stops its
  background loops. The whole budget is `shutdown_grace` in `server.yaml`,
  default 25s; the reference manifests set a matching
  `terminationGracePeriodSeconds` and a `preStop` pause.
