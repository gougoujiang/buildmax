- `./make e2e` now requires a suite instead of defaulting to `kind`. The default
  was the suite with the heaviest prerequisite, so a bare invocation reported a
  missing cluster rather than a missing argument; it now prints the six suites
  and what each one needs.
