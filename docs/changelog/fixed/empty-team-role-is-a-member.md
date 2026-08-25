- A team membership record with no role is now read as a member everywhere.
  Team-scoped routes previously refused such a record entirely while resource
  routes admitted it, so the same account could be a member for one request and
  a stranger for the next. No release could create one, so this affects only a
  database written before the role was defaulted.
