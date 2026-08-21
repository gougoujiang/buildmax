- The administration area has a Models page, and `GET /api/admin/llm/models`
  behind it, showing which upstreams the deployment will call and which aliases
  point at each one — a model that is enabled with no alias is unreachable by
  every team, which is the most common reason an operator's model appears not
  to work. Models can be retired and restored from there. Adding one stays
  `buildmax-server model add`: it carries a provider credential, and doing that
  over HTTP puts the key in a request body, a proxy log, and whatever the
  browser did with the form.
