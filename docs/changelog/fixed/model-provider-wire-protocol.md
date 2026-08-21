- A model entry can now name the wire protocol its endpoint speaks, so BuildMax
  reaches a provider's own API instead of only OpenAI-compatible gateways. Set
  `provider` on a `settings.yaml` model — `openai_compatible` (OpenAI Chat
  Completions, the default), `openai` (OpenAI's Responses API), or `anthropic`
  (the Anthropic Messages API) — and optionally `max_tokens` to cap one
  response. The value names a protocol, not a vendor: Claude through OpenRouter
  is `openai_compatible`, and Claude from `api.anthropic.com` is `anthropic`. An
  operator can serve the same three from the managed gateway with
  `buildmax-server model add --provider --max-tokens`, and `model list` now
  shows each row's provider. Existing configuration is unchanged: an entry that
  names no provider keeps calling what it always called.
