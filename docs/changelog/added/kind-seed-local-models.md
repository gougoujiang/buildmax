- `./make kind seed` fills the local kind cluster's model catalog from the
  repository-root `settings.local.yaml` and grants the deployment's teams an
  alias for each model, so the CLI and Desktop can drive the managed transport
  against real inference. The cluster's own Portal conversations and task runs
  keep answering from the deterministic mock.
