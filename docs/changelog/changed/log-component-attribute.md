- Log records name their subsystem in a `component` attribute instead of a
  prefix on the message. Four background loops in the scheduler alone had spelled
  it four different ways, one of them not at all, so no filter selected a
  subsystem. Levels follow one rule now — Error means a unit of work failed or
  was lost, Warn means degraded but finished — which makes a threshold select
  something. A worker stamps its run id onto every record it writes, including
  those from the agent loop and the tools.
