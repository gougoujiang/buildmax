- The sandbox now probes its backend with a real confined command before
  trusting it, instead of only checking that `bwrap`/`sandbox-exec` is on
  `PATH`. A backend that cannot actually confine a command now reports
  unavailable, so `fail_if_unavailable` refuses to start the run instead of
  silently executing commands unsandboxed.
