- A task run whose worker never reported an outcome no longer stays `SCHEDULED`
  or `RUNNING` forever. Only the worker moves a run out of those states, so an
  evicted pod, a killed process, or a run outliving its credential left work
  that Portal showed as in progress and nothing would ever close. The server now
  records such a run as failed after `worker.run_timeout` (6h by default), with
  an error that names the timeout rather than guessing which of those happened.
