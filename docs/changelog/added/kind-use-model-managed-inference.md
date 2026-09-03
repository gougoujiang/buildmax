- A running deployment can now flip its own conversations and task runs
  between a seeded catalog model and the free mock by environment alone —
  `BUILDMAX_WORKER_LLM_TRANSPORT`, `BUILDMAX_LLM_DEFAULT_MODEL`, and
  `BUILDMAX_CONVERSATION_MODEL_TARGET` override the matching `server.yaml`
  fields, `conversation.model_target` accepts a model name as well as an ID, and
  `./make kind use-model <name>` / `./make kind mock` switch a kind cluster.
