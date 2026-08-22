- Entity relationships are stored as numeric keys rather than as repeated
  identifier strings. Every reference that names exactly one kind of row is now
  a `bigint`, the public handle each row shows the outside world is a separate
  `binary(12)` column, and translating between the two happens inside the store
  and nowhere else. Nothing about the API changed — a handle is still what
  every request, response, token, log line, and object key carries. What
  changed is underneath: a team's task list is answered by reading an index
  backwards instead of sorting rows, usage aggregation is answered from indexes
  without reading rows at all, and identity no longer depends on the database's
  text collation. References that cannot be one number — a polymorphic actor, a
  provider's tool-call ID, an agent session naming a file — stay text
  deliberately, and a test refuses a new one added without that reason.
