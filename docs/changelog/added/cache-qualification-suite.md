- `./make cache-qualify` checks prompt caching against a real provider. Every
  other cache test runs against a fake upstream, which proves what BuildMax
  sends and nothing about what a provider does with it — a request can be
  perfectly shaped while the provider declines to cache it, for a minimum prefix
  length, an unsupported model, or an expired retention window. The suite runs
  first write, sequential read, changed prefix, long-history lookback,
  streaming, concurrent cold starts, and retention, and prints what the provider
  reported for each. Name the target with `BUILDMAX_CACHE_QUALIFY_PROVIDER`,
  `_MODEL`, `_API_KEY`, and optionally `_BASE_URL`; it calls a paid provider, no
  check runs it, and it skips when none is named. A model entry can also name an
  `integration` for an OpenAI-compatible gateway whose cache behaviour has been
  qualified — none has, so every value is currently refused.
