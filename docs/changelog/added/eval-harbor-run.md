- Add `./make eval harbor run`, which starts a Terminal-Bench run rather than
  only importing one. It assembles the Harbor command from
  `evaluation/harbor/pins.json` — dataset ref, adapter import path, and the
  `PYTHONPATH` that lets Harbor import the adapter — checks the toolchain the
  way `./make doctor harbor` does, cross-builds the `linux/amd64` CLI if it is
  missing, and imports the finished job. Tasks are selected with `--task`,
  `--canary`, `--limit`, or `--all`, and there is no default: the default would
  be all 89. `--oracle` runs each task's own reference solution to prove the
  environment, and `--dry-run` prints the command without running it. Harbor
  still owns the tasks, the containers, and the verdict.
