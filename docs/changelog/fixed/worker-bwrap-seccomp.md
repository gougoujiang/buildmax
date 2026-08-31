- Worker pods now use a custom seccomp profile instead of Kubernetes'
  `RuntimeDefault`, and the sandbox re-binds `/proc` instead of mounting a
  fresh one — both were silently preventing the worker's Bash sandbox from
  running at all once deployed to a real cluster.
