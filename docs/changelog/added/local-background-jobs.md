- Background commands in the TUI and Desktop: `Bash` accepts
  `run_in_background` to detach long builds, tests, or servers as local jobs,
  and the new `JobList`, `JobOutput`, and `JobStop` tools inspect and stop
  them. Jobs pass the normal permission and sandbox checks before detaching
  and end with the application.
