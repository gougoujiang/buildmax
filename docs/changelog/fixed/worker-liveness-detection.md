- A task run whose worker was killed without warning is now failed within
  minutes instead of hours. A worker already polls its own run route every few
  seconds so it can hear about a cancel; the server now records that poll, and
  the stale-run reaper fails a `RUNNING` run that has gone quiet for two
  minutes. Before this, only `worker.run_timeout` — six hours by default — ever
  closed such a run, so a SIGKILL, an OOM kill, or a lost node left the Portal
  showing work in progress for the rest of the day. The timeout stays as the
  backstop for a run that never reached `RUNNING` or never reported at all.
  Nothing is re-run: a worker that died may already have caused side effects,
  and whether the task is safe to repeat is not the server's call.
