- The Compose stack derives `cors_origin` from `BUILDMAX_PORTAL_PORT` through
  the new `BUILDMAX_CORS_ORIGIN` override, so moving the Portal off `8080` — to
  run it beside a kind cluster, which cannot move — no longer needs a hand edit
  of `server.yaml` to keep the browser from blocking every request.
