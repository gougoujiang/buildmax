- A model can now reason before it answers, and keep that reasoning across tool
  calls. Set `reasoning: low`, `medium`, or `high` on a `settings.yaml` model,
  or pass `--reasoning` to `buildmax-server model add`, and an `anthropic` model
  uses extended thinking at that effort while an `openai` model keeps its
  reasoning between turns. `openai_compatible` has no such state and ignores the
  setting. It is off by default, because it changes what a call costs and older
  models reject it. The reasoning itself never appears in the transcript:
  BuildMax stores it beside the assistant message and sends it back unread, and
  a session continued against a different provider drops what that provider
  cannot use rather than failing. CLI sessions, Portal conversations, and
  managed gateway calls all carry it, so a run keeps its continuity across a
  restart.
