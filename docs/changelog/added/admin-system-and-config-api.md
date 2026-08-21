- `GET /api/admin/system` and `GET /api/admin/config` report what a deployment
  is doing: version, applied schema migrations, dependency health as `/readyz`
  sees it, worker run mode and model transport, task runs by status, and the
  effective `server.yaml` with every credential reduced to whether it is set.
  Not its length, not a prefix — presence only. The configuration view also
  computes warnings for states that are trade-offs elsewhere in the project:
  self-registration left open, the deprecated shared worker token still set,
  managed worker inference with no aliases, local-process run mode, a run
  timeout that outlives its run token, and run output on local disk.
