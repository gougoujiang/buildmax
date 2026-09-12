- A server can now expire old run traces: `server.yaml` `trace.retention_days`
  (0, the default, keeps every trace forever) runs an hourly sweep that removes
  the trace of a run that ended longer ago than the window and records a
  `traces.pruned` audit event, so a trace missing by policy is distinguishable
  from one that was lost.
