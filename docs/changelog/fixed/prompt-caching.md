- Prompt caching is now available with `prompt_cache: true` on a model, or
  `--prompt-cache` on a catalog model. For `anthropic` it places cache
  breakpoints around the tool definitions and system prompt, which do not change
  between calls in a run; the OpenAI protocols already cache on their own. It is
  off by default because a cache write costs more than not caching and only pays
  back over several calls. Every provider now reports `cache_read_tokens` and
  `cache_write_tokens`, and a managed deployment records both on the call
  ledger. They break the prompt count down rather than adding to it, so a spend
  report must not sum them alongside it.
