- A workflow run now uses the agent definitions it started with. Steps are
  dispatched one at a time as the previous step's task run finishes, and each
  dispatch re-read the agent, so editing an agent while a run was in flight
  changed what its later steps sent to the model — a run could execute two
  different versions of the same agent, with nothing recording that it happened.
  Each step run now stores the agent name, description, and instructions as they
  were when the run started, and dispatches from that copy. Runs already in
  flight when this ships keep the old behavior, since their steps carry no
  snapshot. The captured definition is returned with the workflow run detail and
  shown per step in Portal.
