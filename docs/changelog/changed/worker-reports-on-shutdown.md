- A task run whose worker is shut down — a drained node, an evicted pod, a
  restarted deployment — now stops, uploads what it produced, and reports
  `FAILED` with a message naming the shutdown, instead of sitting in `RUNNING`
  until the stale-run reaper closes it hours later. Its output and artifacts are
  kept and shown the way a cancelled run's are.
