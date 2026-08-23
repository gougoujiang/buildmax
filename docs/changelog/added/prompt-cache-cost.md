- A run can now report what it cost. A model entry takes a `pricing` block —
  currency plus four decimal rates per million tokens, for fresh input, cache
  reads, cache writes, and output — and the CLI prints a `Cost(session)` line,
  `--format json` carries the same figures, and the session file keeps a running
  total. A managed deployment sets the same rates per catalog model with
  `--currency`, `--input-price`, `--cache-read-price`, `--cache-write-price`,
  and `--output-price` on `buildmax-server model add`; the rates in force are
  copied onto each call's ledger row when it is accepted, so repricing a model
  does not restate what a team already spent. Portal's run view shows the run's
  cost and what caching saved against an uncached baseline. Cost is shown only
  where every rate needed for it was recorded — anything else reads
  `unavailable` rather than zero — and a saving is reported only when caching
  actually saved: a run that wrote cache entries nothing read back paid more
  than it would have uncached, and is shown as the cost it was.
