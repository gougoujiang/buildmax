- Background jobs write a durable event log under `<traces>/jobs/`: launch
  provenance (owning session, parent run and tool call, sandbox fact),
  monitor lines with drop accounting, and the terminal state. Logs are
  redacted, bounded, and always end with how the job ended.
