- An agent working a team issue can now read it and report back. `GetIssue`
  returns the issue, its sub-issues, and recent discussion; `ReportToIssue`
  posts one bounded comment on the thread. Both are scoped to the issue the run
  was started for, and neither can change its status, assignee, or sub-issues.
