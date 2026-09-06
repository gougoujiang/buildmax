- `buildmax-server` sheds two routine operations now that `buildmax admin`
  covers them: `user set-password` is removed (issue a login code and let the
  person choose their own password — the safer equivalent), and `admin list` is
  removed (use `buildmax admin list`, or the Portal). Creating accounts and
  issuing login codes, and granting or revoking administrators, stay on
  `buildmax-server` as the break-glass path.
