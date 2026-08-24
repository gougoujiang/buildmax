- CLI and Desktop now keep a login's access and refresh tokens in the
  operating system's credential store (Keychain, Credential Manager, Secret
  Service) instead of as plaintext in `auth.json`. A file written before this
  change is moved on first read, and a machine with no usable credential store
  falls back to the file as before; `buildmax login`, `buildmax whoami`, and
  `buildmax doctor` say which one a login is actually using. Set
  `BUILDMAX_CREDENTIAL_STORE=file` to keep the previous behavior.
