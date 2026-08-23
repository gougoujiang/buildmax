- Stopping a server in `local_process` worker mode no longer waits for the agent
  run it dispatched. The scheduler stops claiming immediately, asks the worker it
  spawned to stop — which reports what the run produced — and gives up on a
  worker that will not go, within the same `shutdown_grace` budget. In
  `k8s_job` mode the Jobs already outlive the server, and still do.
