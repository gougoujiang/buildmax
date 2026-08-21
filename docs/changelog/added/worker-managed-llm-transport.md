- Task runs can reach models through the managed LLM gateway instead of calling
  a provider themselves. Set `worker.llm.transport: buildmax` in `server.yaml`,
  optionally with `worker.llm.alias`, and a worker no longer receives
  `BUILDMAX_CONVERSATION_MODEL_API_KEY` at all — it reaches operator-approved
  models through the server. The server states the transport and alias in the
  run's worker-API response, so a worker never chooses its own model, and it is
  told nothing else about it: the endpoint, the upstream model identifier, and
  the credential stay server-side. Direct mode is unchanged and remains the
  default; there is no automatic fallback between the two, because a server
  outage must not silently redirect governed traffic to a personal provider key.
  A deployment that asks for managed runs it cannot serve — `transport:
  buildmax` with no `llm.aliases`, or an alias no team may call — now fails at
  startup rather than at every run's first model call.
