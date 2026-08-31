- A background agent definition can now declare a network sandbox tier
  (none/registries/open) and a filesystem tier (workspace/shared-read/
  external-write) that its worker runs apply, without an operator
  hand-editing `policy.yaml` per agent.
