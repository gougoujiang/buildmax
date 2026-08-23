- Refresh the built-in context_window fallback table: several ids had drifted
  from what providers now report (some by a lot, e.g. deepseek-r1), one
  Anthropic id had a hyphen where the real id uses a dot and never matched,
  and OpenAI/Anthropic now cover their fast/mid/premium tiers instead of one
  model each.
