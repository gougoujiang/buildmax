- Each session is now a folder under `sessions/<id>/` holding its metadata, an
  append-only conversation journal, and its own run traces, replacing the single
  JSON file per session and the `traces/` root. The conversation is written as
  it happens rather than rewritten after each reply, so an interrupted run keeps
  everything up to the moment it stopped, and BuildMax can tell a tool call that
  never started from one that may already have changed something. A session can
  be open in one place at a time; opening one already in use says so instead of
  letting two runs overwrite each other. Sessions from earlier versions are not
  migrated and are ignored.
