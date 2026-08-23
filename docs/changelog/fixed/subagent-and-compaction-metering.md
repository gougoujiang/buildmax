- Session token counts and cost now include the work a run delegated to a
  subagent and what each context compaction cost; both were previously spent
  and never counted, so long sessions and ones that used `Task` under-reported
  themselves. A subagent's trace is also filed under the session that started
  it rather than under a discarded id, and `tool_end` records how a call failed.
