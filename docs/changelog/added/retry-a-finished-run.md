- A finished task run can be run again. Portal's Issue Detail shows **Retry Run**
  once a run is over, backed by
  `POST /api/teams/{team_id}/tasks/{task_id}/retry`. The retry carries the same
  input the previous run had, so recovering from a worker that died or a model
  that timed out no longer means retyping the instructions — and no longer
  invites retyping them slightly differently. The new run records which run it
  repeats, and the run it repeats is left exactly as it was, so what went wrong
  stays readable next to the attempt that followed it. A run still in flight is
  not retried but stopped first; a task that has never finished a run has
  nothing to repeat; and a workflow step is refused, because its workflow
  advances from that step's outcome and a run started outside it would report a
  second one.
