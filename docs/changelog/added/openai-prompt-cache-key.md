- OpenAI Responses calls now carry a scoped `prompt_cache_key`, and can ask for
  24-hour retention with `cache_control: {ttl: 24h}`. The API caches on its own
  either way, so the key does not turn caching on — it decides which prefixes
  are looked up together, which matters because callers sharing a credential
  otherwise share one bucket. The key is derived from the credential, the model,
  the team on a managed call, and fingerprints of the system prompt and tool
  definitions; it carries none of them in readable form and is never written to
  a ledger, trace, or log. Retention vocabulary is per provider: `5m` and `1h`
  are Anthropic's and `24h` is OpenAI's, and asking for one where it is not
  documented is refused at startup rather than sent and ignored.
