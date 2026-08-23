- Every stored moment in time is now a `DATETIME(6)` column and an RFC 3339
  string in the API, replacing Unix seconds in `bigint` columns and JSON
  numbers. Audit `since` and `until` query parameters take RFC 3339 too. An
  existing Alpha database is recreated rather than converted; no conversion
  migration ships.
