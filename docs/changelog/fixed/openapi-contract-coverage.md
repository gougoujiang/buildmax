- `openapi.json` now describes every route the server registers, 117 operations
  instead of 40, and corrects the schemas that called a timestamp an integer:
  `created_at`, `started_at`, and `ended_at` are RFC 3339 strings, which is what
  the API has always sent. Tests now hold the document to an exact match with
  the registered routes.
