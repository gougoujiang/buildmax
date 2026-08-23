- A run trace now reaches disk one record at a time, so a run that is
  interrupted — killed, crashed, or given up on mid-turn — keeps its final
  records instead of losing up to 4KB of them. Previously the tail stayed
  buffered until the run closed cleanly, which made the last recorded event
  appear seconds before the last one that actually happened. Shutting the
  runtime down also closes the trace of a run still in flight, marking it
  abandoned rather than leaving it open with no `run_end`.
