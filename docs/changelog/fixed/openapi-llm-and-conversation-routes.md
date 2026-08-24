- The served OpenAPI document no longer describes routes that do not exist.
  Managed inference is `GET /api/llm/models` and `POST /api/llm/completions`,
  not the team-scoped paths it listed, and listing or creating a conversation is
  team-scoped rather than `/api/conversations`.
