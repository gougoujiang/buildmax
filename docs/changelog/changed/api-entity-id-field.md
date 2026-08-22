- An API resource now names its own identifier `id` rather than repeating its
  type — a task returns `{"id": ..., "team_id": ...}` where it used to return
  `{"task_id": ..., "team_id": ...}`. Relationships are unchanged and keep their
  semantic names, so only the field naming the resource itself moved. Users,
  teams, artifacts, audit events, managed model catalog entries, system grants,
  managed LLM call records, and webhook keys are affected; most other resources
  already used `id`. Agent and workflow revisions no longer return an identifier
  at all: a revision is addressed by its parent plus its revision number, which
  is what the restore route already used. Catalog plugins and plugin releases
  likewise drop theirs, being addressed by name and by name plus version.
