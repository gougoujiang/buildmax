- A run's output now keeps everything the agent said during the turn, not only
  its closing message: text the model wrote before a tool call — its narration
  of what it is about to do — is joined with the text it wrote after, so an
  Agent's TaskRun shows the whole turn rather than dropping the earlier part.
  This also removes the flicker where streamed narration appeared and then
  vanished when the run finished.
