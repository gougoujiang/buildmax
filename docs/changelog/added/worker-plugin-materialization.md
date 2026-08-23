- A background run now loads the plugins its agent names. The server resolves
  the team's activations when the worker claims the run, and the worker fetches
  exactly those releases, verifies each against its pinned digest before
  extraction, and refuses to start rather than run without one it was told to
  have.
