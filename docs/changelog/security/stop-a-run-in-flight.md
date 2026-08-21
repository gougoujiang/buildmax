- A task run in flight can be stopped. Portal's Issue Detail shows **Stop Run**
  while an agent task is pending or running, backed by
  `POST /api/teams/{team_id}/tasks/{task_id}/cancel`. A run nobody has picked up
  yet ends immediately as `CANCELED`. A run a worker is already executing is
  asked to stop: the worker notices within seconds, ends its agent loop, uploads
  what the run produced, and reports `CANCELED` — so a canceled run keeps its
  output and artifacts instead of throwing away the work already done. The run
  records who asked and when. Nothing is left hanging if the worker is gone:
  the server finishes a run whose cancel goes unconfirmed for two minutes, which
  is the same sweep that closes abandoned runs. Cancelling twice while a run is
  stopping is not an error, and a canceled task can be run again.
