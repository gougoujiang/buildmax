- Add `evaluation/harbor/`: pinned Harbor, dataset, and adapter versions, the
  custom-Agent adapter that runs the built CLI against Terminal-Bench 2.1 inside
  a task container, and `./make eval harbor --job <dir>`, which files a finished
  job as BuildMax trial bundles and reports it in the same contract as the local
  suite — same subject tuple, same failure taxonomy, same pass rate with its
  uncertainty. Harbor stays the harness and its verifier stays authoritative:
  BuildMax neither re-runs the benchmark nor re-grades it, and an agent timeout,
  a verifier timeout, and a container that never started stay three different
  facts. `./make doctor harbor` reports what a run needs, reading the pinned
  versions rather than restating them, and prints the fix for each missing piece
  instead of installing anything.
