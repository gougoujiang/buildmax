- A failed run keeps the reason its worker reported. The worker records the
  failure and then exits non-zero, and the scheduler used to overwrite the
  record with the process error — so "context deadline exceeded calling the
  model" became "exit status 1". The scheduler now leaves a run alone once it
  has reached a terminal status.
