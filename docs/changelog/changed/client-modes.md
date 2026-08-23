- The CLI and Desktop now run in one of two modes, decided by whether you are
  signed in. Signed out, the models are the ones in `settings.yaml` and each
  call goes straight to its provider; signed in, they are the ones that
  deployment offers and every prompt goes there. `buildmax login` and
  `buildmax logout` switch, and nothing else configures it — `models[].transport`
  and `models[].server_url` are gone, because a session is in one mode or the
  other rather than holding both kinds of entry. A new `default_model` key names
  which entry a session starts with while signed out; a deployment names its
  own. `buildmax models`, the `/model` pickers, and the TUI footer all say which
  mode you are in, and `buildmax doctor` reports it as a check of its own.
  Desktop no longer opens on a sign-in form: local mode is a working state, so
  the workbench opens directly and signing in is an action in the account menu.
  Neither mode covers for the other — a deployment that is down, or a login that
  has expired, stops the session and says so rather than quietly sending the
  next prompt to a provider you did not choose for it.
