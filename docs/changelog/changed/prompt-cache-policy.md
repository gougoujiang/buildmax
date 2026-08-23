- Prompt caching is now a policy rather than a boolean, and Anthropic agent
  turns cache by default. A model entry takes `cache_control: {mode, ttl}` —
  `mode: auto` (the new default) asks on an agent turn, whose prefix goes out
  again on the next iteration, and never on a one-shot call such as title
  generation or compaction, where a cache write costs more than it can ever
  save; `off` never asks and `force` always does. `ttl` selects retention where
  the provider documents it — `5m` or `1h` on Anthropic, `24h` on OpenAI — and
  is refused at startup anywhere else rather than sent and ignored, as is
  `force` on a provider that takes no cache instructions at all. The
  `prompt_cache` boolean it replaces is removed rather than kept as a
  shorthand; write `cache_control: {mode: off}` where it said `false`. Managed
  deployments get the same policy per catalog model through `--cache-mode` and
  `--cache-ttl` on `buildmax-server model add`.
