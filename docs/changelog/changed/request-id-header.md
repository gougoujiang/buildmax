- The server answers every request with an `X-Request-Id` header, and its logs
  now record each request when it finishes rather than when it starts — with the
  status, the duration, and that same id. A bug report can name the request, and
  every log line the request produced can be found by that name. The log level
  follows the status, so filtering at Error selects server faults without
  sweeping up refused requests.
