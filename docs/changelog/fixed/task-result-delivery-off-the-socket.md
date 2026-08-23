- A finished background task now reports back whether or not anyone is watching.
  The reply used to be sent through the creator's first open browser connection,
  which meant it was skipped entirely when they had none and reached only one
  tab when they had several. Every connection on the team is now told the task
  changed, and the reply itself is written to the conversation independently of
  any connection.
