- `buildmax -p --output json` and `--output jsonl` now report `trace_id` and
  `trace_path`, so a script can open the trace that run wrote instead of
  guessing at the newest file in the session's trace directory.
