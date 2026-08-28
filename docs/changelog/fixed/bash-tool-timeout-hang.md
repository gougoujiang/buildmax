- Stop a Bash command that leaves a background process behind from hanging the
  agent forever. The tool reads output through a pipe, and a server or daemon
  the command started inherits the write end and holds it open, so the wait
  outlived both the command and its timeout — a run was observed sitting on one
  call for two hours under a documented 120-second budget. The tool now stops
  waiting shortly after the command ends or its deadline passes, and says so
  when output was cut short.
