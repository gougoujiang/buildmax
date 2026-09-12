- The space audit trail now records space creation (with its quota tier),
  webhook key creation and revocation, agent definition create/update/delete,
  and workflow create/publish/archive/unpublish; and every event carries the
  task run it was recorded on behalf of, so an investigation can pivot between
  an audit event and the run that caused it.
