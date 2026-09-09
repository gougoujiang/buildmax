- Request logs now redact credential-bearing query parameters, so a WebSocket
  upgrade's `?token=` JWT and similar secrets no longer appear verbatim in logs
  or in artifacts that capture them.
