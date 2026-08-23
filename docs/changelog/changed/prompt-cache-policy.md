- Prompt caching is now a policy rather than a boolean, and Anthropic agent
  turns cache by default. A model entry takes `cache_control: {mode, ttl}` —
  `mode: auto` (the new default) asks on an agent turn, whose prefix goes out
  again on the next iteration, and never on a one-shot call such as title
  generation or compaction, where a cache write costs more than it can ever
  save; `off` never asks and `force` always does. `ttl` selects retention where
  the provider documents it — `5m` or `1h` on Anthropic — and is refused at
  startup anywhere else rather than sent and ignored, as is `force` on a
  provider that takes no cache instructions at all. The `prompt_cache` boolean
  still works: `true` means `force`, `false` means `off`, and leaving it out
  means `auto`. Managed deployments get the same policy per catalog model
  through `--cache-mode` and `--cache-ttl` on `buildmax-server model add`; an
  existing catalog row with `prompt_cache` unset now defaults to `auto`, so set
  `--cache-mode off` on any model that should not cache. A managed request
  carries only what the call is for; the cache policy is resolved server-side
  from the approved model, so a client cannot select retention the operator did
  not choose.
