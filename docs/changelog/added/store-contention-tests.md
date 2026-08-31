- The store's four conditional-update claims — task claiming, run transition,
  result-delivery claiming, and cancellation beside a worker's report — are now
  tested against a real MySQL under contention, so a run cannot quietly be
  claimed twice or a task summary delivered twice.
