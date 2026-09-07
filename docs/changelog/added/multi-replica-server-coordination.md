- A `coordination` server setting shares live streaming, connection events, and
  conversation turn serialization across replicas through Redis, so a deployment
  can run more than one server replica correctly; `mode: local` (a single
  replica) stays the default, and `mode: redis` fails closed when Redis is
  unreachable.
