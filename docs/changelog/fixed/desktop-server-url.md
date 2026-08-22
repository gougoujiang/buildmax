- The Desktop sign-in form shows the Server URL field instead of hiding it
  behind a collapsed "Server" toggle, and fills it from `settings.yaml`'s
  `server_url` the way `buildmax login` already did. A deployment that does not
  answer on `http://localhost:5678` — the kind cluster serves Portal and API
  from `http://localhost:8080` — is now reachable without hunting for the field.
