- A managed model is now named by its catalog name rather than by a team alias.
  `server.yaml` loses `llm.aliases` and `llm.default_alias` and gains
  `llm.default_model`, which names one of the models `buildmax-server model add`
  created, or is left empty to use the first enabled one. `worker.llm.alias`
  becomes `worker.llm.model`. A name that matches no catalog row now stops the
  server at startup, while an empty catalog does not — rows are added while the
  server runs. In `settings.yaml`, a `transport: buildmax` entry drops `team_id`
  and puts the catalog name in `model`; every model a deployment offers is
  available to every user of it, so `buildmax models --team <id>` becomes
  `buildmax models --server`. The gateway routes move from
  `/api/teams/{team_id}/llm/...` to `/api/llm/...`, and a managed call is
  recorded against the person who made it rather than against a team.
