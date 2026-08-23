- A deployment can serve a local Ollama model: `--provider ollama` on
  `buildmax-server model add`, or `provider: ollama` under
  `conversation.model`, with no credential in either place. Real inference and
  real tool calls reach the gateway, the `llm_call` ledger, and quota without a
  provider key or a bill. The daemon stays on the host — a pod cannot use the
  host's GPU — and the deployment names an address that reaches it, which under
  Docker Desktop is `host.docker.internal`.
