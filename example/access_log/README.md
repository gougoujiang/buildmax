# Access Log Example Data

A simplified web access log, for counting by path or status code, computing
error rates, and finding slow requests.

## Files

| File | Contents |
|---|---|
| `access_log.csv` | Timestamp, client IP, path, status code, duration in ms |

## Columns

- **timestamp** — request time
- **ip** — client IP address
- **path** — request path
- **status_code** — HTTP status: `200`, `401`, `404`, `500`
- **duration_ms** — response time in milliseconds

## Query ideas

- Request counts per `path`, to find the busiest endpoints
- Distribution of `status_code`, and the 4xx/5xx error rate
- Requests per `ip`, to find the most active clients
- Slow requests, where `duration_ms` exceeds a threshold
