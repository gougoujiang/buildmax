- The Portal "Run details" view now shows what became of a run's workspace: a
  Workspace section reports whether the run restored its base checkpoint and
  whether its result checkpoint committed, with the bounded reason on a failure,
  so an operator can see a run's continuity state without reading the database.
