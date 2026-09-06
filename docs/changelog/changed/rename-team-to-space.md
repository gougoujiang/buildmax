- The ownership and authorization boundary is renamed from **Team** to
  **Space** across the product: API routes move from `/api/teams/{team_id}`
  to `/api/spaces/{space_id}`, the `team_id` JSON field becomes `space_id`,
  and Portal, the CLI, and stored data use Space throughout. This is a
  breaking API change with no compatibility shim, as the Alpha allows.
