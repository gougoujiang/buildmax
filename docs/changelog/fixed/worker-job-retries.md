- A worker that finds its run already belongs to someone else now exits cleanly
  instead of reporting a failed dispatch. It had exited `2` so the scheduler
  could tell that case from a run that failed to start, but nothing read the
  code: under Kubernetes `2` is non-zero like any other failure, so the Job
  restarted a pod that could only refuse the run again, and under the local
  runner it made the scheduler mark a run `FAILED` while another worker was
  still executing it. The Job's retry budget stays at three, which is what
  recovers a worker that died before claiming its run — while reading its
  configuration, fetching the run, or resolving its model — since the run is
  still `SCHEDULED` for a fresh pod to take.
