- Deployment administrator authority is now safe under concurrency: grants can
  no longer be duplicated by a race; two administrators can no longer revoke or
  disable at the same time and leave the deployment with no one able to reach its
  admin area; a disabled account no longer counts as a holder; and granting a
  role to a disabled account is refused instead of stored as unusable authority.
