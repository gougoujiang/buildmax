- Portal can show a run's trace on deployments that keep run state on local
  disk. `local_fs` is the default persist backend, and it deliberately stores
  no copy of a run's global files — the worker has already written them under
  `workspaces_dir`. The trace endpoint asked only the backend, so it answered
  "this run's trace is no longer in storage" for every run on every default
  deployment, including the Compose quickstart, while the file sat on disk the
  whole time. It now falls back to disk, as the task conversation endpoint
  already did. Deployments backed by S3 were unaffected.
