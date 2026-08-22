- Operators can restrict where plugins may come from with a
  `plugins.allowed_sources` block in `policy.yaml`. A plugin an operator
  excluded shows as `refused` with the reason rather than disappearing, and a
  directory whose provenance cannot be established does not pass a policy that
  names one.
