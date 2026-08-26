- A run that fails part way through now reports what it did before it failed.
  The workspace, the model, the elapsed time, the tool calls, and the tokens
  already spent were all dropped on the failure path, so `buildmax -p` closed
  with `Tool calls: 0`, `Duration: 0ms`, an empty `Workspace:`, and no token
  line even when the run had edited files and been charged for the calls that
  got it there. `--output json` reported the same blanks. The session's own
  totals missed them too, so a conversation resumed after a failed turn counted
  from zero for good.
