- A deployment can now have System Administrators: an authority over the
  deployment itself, held by an account and separate from every Team role.
  `buildmax-server admin grant | revoke | list` manages it on the machine that
  already holds the database credentials, which is also what recovers a
  deployment that has lost every administrator — there is no configuration
  value and no break-glass credential. A grant carries no access to any team's
  issues, conversations, artifacts, files, or run traces; those stay behind
  team membership. Granting, revoking, and — new — account creation, password
  setting, and login-code issuance from `buildmax-server user` are all recorded
  in the audit trail, which those three commands previously wrote nothing to.
