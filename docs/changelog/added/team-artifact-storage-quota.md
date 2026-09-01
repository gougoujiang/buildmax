- A quota tier can now cap what a space's artifacts hold in total with
  `max_storage_bytes`, refusing an upload that would cross it; space settings
  report storage alongside runs and tokens. The seeded tiers set no limit, so
  an existing deployment is unaffected until an operator chooses one.
