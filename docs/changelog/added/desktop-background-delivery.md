- Desktop delivers background events into the conversation: a completion
  requested with `deliver_result` or a `react` monitor line runs as its own
  turn when the owning session is on screen and idle, and is parked — not
  lost — while that session is busy or another one is open. The transcript
  labels delivered events as background observations, collapsed by default.
