- Contributor-local configuration now lives in one gitignored `.local/`
  directory, created by `./make setup local` from the committed templates. The
  repository-root `.env` became `.local/env`, `settings.local.yaml` became
  `.local/settings.yaml`, and the local copy of
  `deployment/buildmax-secret.example.yaml` became `.local/buildmax-secret.yaml`.
  `./make doctor` reports whether the directory is there, and the command moves
  files left at the old paths rather than duplicating them.
  `deployment/compose/.env` is unchanged: Compose reads it from the compose
  file's own directory.
