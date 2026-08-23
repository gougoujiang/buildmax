- A run trace now records what each model call cost, not just what the run did.
  `llm_end` carries that call's own token counts and its estimated cost;
  `run_end` carries the run's. The per-call figures matter because the running
  totals cannot answer which turn was expensive, and subtracting consecutive
  records to find out goes wrong the moment a call in between failed. It also
  makes the shape of caching visible: the turn that writes a cache entry costs
  more than it would have uncached, and only a later turn reading it back puts
  the run ahead. Costs are absent when the model was unpriced, which is not the
  same fact as a call that cost nothing, and `cost_incomplete` on `run_end`
  says a call did work that could not be priced.
