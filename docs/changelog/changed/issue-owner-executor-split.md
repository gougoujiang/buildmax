- Issue owner and executor are now independent fields instead of one combined
  assignee, so an issue can have an accountable person and a selected Agent or
  Workflow at the same time. `owner_id` replaces the person case of the old
  `assignee_kind`/`assignee_id` pair, and `executor_kind`/`executor_id`
  replace the agent and workflow cases.
