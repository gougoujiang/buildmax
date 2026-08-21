- Each task run is now dispatched with its own gateway credential rather than
  sharing the deployment-wide worker token. The scheduler mints a short-lived
  run token naming the run's user, team, and task, delivers it in
  `BUILDMAX_RUN_TOKEN`, and the managed inference route accepts nothing else —
  a run token presented against another run's URL is refused, and so is the
  shared worker token. Every managed call a worker makes is therefore recorded
  in the `llm_call` ledger against a user as well as a team, which a shared
  secret could never support. Lifetime is `worker.run_token_ttl`, 24h by
  default; it is not renewable, so it must outlast your longest run. A run token
  cannot be used as a user login, and a user access token cannot be used as a
  run token. The token is cleared from the worker's environment once read, so
  model-chosen shell commands cannot print it — the sandbox that would otherwise
  strip it is off by default.
