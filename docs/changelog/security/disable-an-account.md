- An account can be disabled, and disabling it stops access immediately rather
  than when a token expires. Every credential the account holds is refused:
  password, login code, refresh token, the access token it is already carrying,
  and its webhook keys. Live sessions are revoked at the same time, and work the
  account queued but that has not started fails at dispatch instead of running.
  Previously there was no way to stop an account short of editing the database,
  and an issued access token stayed usable for its full lifetime — seven days by
  default. Disabling is not deletion: nothing is removed, and enabling reverses
  the state and nothing else.
