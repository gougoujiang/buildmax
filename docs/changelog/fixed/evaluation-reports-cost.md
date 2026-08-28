- Report what an evaluation run spent. The trial home a trial runs under carried
  a model entry with no prices, so `./make eval` had always reported cost as
  unavailable however the model was configured; it now carries the price list
  from the same `settings.yaml` entry it takes the endpoint from. A
  Terminal-Bench run takes one through the new `pricing` agent kwarg, passed
  explicitly rather than read from the machine, so the figure is reproducible.
