- Fixed the Portal task view briefly showing a previous turn's reply before the
  new run's output arrived: the live output stream is now scoped per run, so a
  finished run's buffered text is never replayed to the next turn's watchers.
