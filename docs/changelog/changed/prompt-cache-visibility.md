- Prompt-cache token counts now reach every surface that shows what a run
  spent. Providers already reported them and the managed ledger already stored
  them, but they stopped there: run statistics, run traces, session totals, the
  CLI's summary and `--format json` output, Desktop's run status, the team
  run-ledger route, and Portal's run-spend view all dropped them, so a cached
  run was indistinguishable from an uncached one. Cached counts remain a
  breakdown of the prompt rather than an addition to it, and each surface shows
  them only where a provider actually reported some — a provider that reports
  nothing is not a provider that missed.
