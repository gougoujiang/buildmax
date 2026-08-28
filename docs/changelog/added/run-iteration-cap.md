- Make the agent loop's iteration cap configurable with `agent.max_iterations`
  in `settings.yaml` and `--max-iterations` for one run, so a long unattended
  task is not cut off at the fixed 200 that suited interactive work. A run that
  reaches the cap now exits `7` with error kind `iteration_cap` rather than
  sharing `4` with a failed model call, so a script or harness can tell a spent
  budget from a fault worth retrying.
