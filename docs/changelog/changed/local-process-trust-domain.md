- `worker.run_mode: local_process` is now documented as what it is — a
  single-machine topology where a task run is a child process of the server
  under the same uid, sharing its trust domain — instead of being called a
  development path the Compose deployment contradicts. The startup warning says
  the same. Nothing about how a worker is launched changed; a deployment that
  needs its server separated from model-chosen code still runs
  `worker.run_mode: k8s_job`.
