- Enabling or disabling a catalog model with `buildmax-server model enable` or
  `model disable` now records the model's name in the audit trail, which the
  equivalent `/api/admin` route already did. The trail distinguishes a catalog
  change by who made it, not by where it was made.
