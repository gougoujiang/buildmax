- Fixed the worker Bash sandbox being silently unconfined under
  `worker.run_mode: local_process` (Compose): the marker that gates the
  strict worker baseline never reached that worker's filtered environment,
  so model-chosen commands ran with no filesystem confinement at all. A
  Compose deployment also needs the Job pod's seccomp override for `bwrap`
  to build its sandbox at all; `deployment/compose/compose.yaml` now sets
  it.
