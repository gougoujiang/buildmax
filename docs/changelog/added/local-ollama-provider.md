- Local models through Ollama: `provider: ollama` on a model entry calls a
  local daemon's own API with no `api_key` at all. It sends the context window
  on every call, so the daemon no longer applies its own default and quietly
  truncates the system prompt and tool definitions out of a longer request —
  the failure that made small local models look like they could not call tools.
  `buildmax init --ollama` writes the entry, `buildmax models --local` lists
  what is installed and which models can call tools, and `buildmax doctor`
  reports a daemon that is not running or a model that is not pulled with the
  command that fixes it.
