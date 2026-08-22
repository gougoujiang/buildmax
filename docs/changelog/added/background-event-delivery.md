- Background events can wake the conversation in the TUI: `run_in_background`
  calls accept `deliver_result` to have the completion delivered as its own
  turn, and a `Monitor` started with `react` sends each delivered line back
  for analysis. Delivered payloads are marked as untrusted observations,
  recorded with non-user provenance, and never run user-prompt hooks.
