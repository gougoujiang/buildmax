- Closing the runtime now waits for the background job trace writer to drain,
  so a job's final record always lands in `traces/jobs/` before exit.
