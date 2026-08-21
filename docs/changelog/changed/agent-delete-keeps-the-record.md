- Deleting an agent no longer removes the record. Tasks, workflow step runs, and
  revisions all name an agent by ID, so dropping the row left dangling
  references and broke any workflow run still in flight at its next step. The
  agent is now marked deleted: it disappears from listings and from everything
  that starts new work — a run cannot be started with it, an issue cannot be
  assigned to it, and a workflow definition naming it is refused — while records
  that already refer to it still resolve and an in-flight run finishes on the
  snapshot it started with. Deleting an agent a *published* workflow still names
  is refused with `409` naming those workflows, so the breakage surfaces at the
  delete rather than at the next run; draft and archived workflows do not block
  it. There is no undelete: the row exists so references resolve, not as a
  recycle bin.
