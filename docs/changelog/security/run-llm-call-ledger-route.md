- `GET /api/teams/{team_id}/task-runs/{task_run_id}/llm-calls` lists what a run
  spent and on which approved alias. The ledger recorded this from the day it
  existed and had no route, so reading it meant querying the database —
  diagnosing a run should not require the database password. It carries no
  prompts or generated content, and omits the catalog entry an alias resolved
  to, which is the operator's routing rather than the team's.
