- A signed-in CLI whose deployment rejects the stored credential with 401 (the
  session was revoked, or the server no longer trusts the token) now reports the
  login as expired and names `buildmax logout` to return to local mode, instead
  of failing with a bare `list the models ... : server 401: unauthorized`.
