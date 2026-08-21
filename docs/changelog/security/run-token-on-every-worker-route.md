- The run token now authenticates **every** `/api/worker/*` route, not only
  managed inference, so a run can read and write its own record and nothing
  else. The deployment-wide `worker.token` could name any run, which meant any
  worker could read the prompt text of every team's tasks, `PATCH` another
  team's run to SUCCEEDED with output it invented, or push deltas into another
  run's live stream. It is still accepted for one release, with a deprecation
  warning, because a server that has not restarted yet dispatches workers
  without minting a token; the next release removes it. Since every route needs
  one, every dispatched run is now given a token, direct-mode runs included.
  `buildmax-server run-token <task_run_id>` mints one for driving a worker route
  by hand, which is what copying the shared secret used to be for.
