- A run whose sandbox resolved weaker than its surface's own baseline — or
  fell back to unconfined because the OS backend was unavailable — now logs a
  warning at startup and marks the run's trace and `SessionStart` hook
  payload as downgraded, instead of proceeding silently.
