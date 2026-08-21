- Portal's **Run details** now shows the managed model calls a run made, beside
  the trace the run wrote itself. The ledger has had a route since it became
  readable, and nothing displayed it: the two records answer different
  questions, and the difference is the point. The trace is what the agent did;
  this is what the deployment was asked to serve, on which operator-approved
  alias, and it is what a team's quota is computed from. An empty list is not
  reported as "spent nothing" — a run whose trace shows model calls that the
  ledger never saw is a run that reached a provider directly, and it says so.
  A call whose provider reported no usage is counted as unreported rather than
  as zero.
