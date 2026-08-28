- Add `./make setup harbor`, the write half of `./make doctor harbor`: it
  installs uv when it is missing, installs the Harbor version pinned by
  `evaluation/harbor/pins.json`, cross-builds the `linux/amd64` CLI a trial
  uploads, and finishes by re-running doctor's own probes, so what setup
  installs and what a benchmark run requires cannot drift apart. Steps already
  done are skipped. Installing uv runs Astral's installer, and the exact command
  is printed before it runs. A trial sandbox stays yours to choose: setup reports
  that Docker or a `DAYTONA_API_KEY` is missing rather than picking one. Doctor
  still installs nothing. Both commands now name `linux/amd64` rather than the
  host's architecture, because the architecture that matters is the task image's:
  an arm64 binary uploaded into an emulated image fails with an exec format error
  once the trial is already running.
