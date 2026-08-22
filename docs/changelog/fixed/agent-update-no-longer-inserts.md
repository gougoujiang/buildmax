- Editing an agent no longer fails with a duplicate-key error. The update wrote
  a row rebuilt from the domain model, which no longer carries the database's
  own key, so it was saved as a new agent rather than as a change to the
  existing one; the unique index caught it and refused the edit. The update now
  addresses the agent by its identifier.
