- A finished background task now always gets reported back to its conversation.
  The reply used to be a one-shot attempt: a model call that failed, a busy
  conversation, or a server restart between the task finishing and the reply
  being written meant the conversation was simply never told. The report is now
  recorded as owed and retried until it succeeds, or until it has failed enough
  times to be given up on with the reason kept. The result itself was never at
  risk — the task's card reads it directly.
