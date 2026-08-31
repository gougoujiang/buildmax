- `./make test mysql` runs the store tests against a real MySQL on a database
  it creates and drops, and every pull request now runs it against a pinned
  server. The tests existed but skipped themselves without a DSN, so schema,
  query, and transaction behavior was outside ordinary change review.
