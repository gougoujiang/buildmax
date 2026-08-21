- Agents and workflows keep a numbered, append-only history. Every edit records
  the definition it produced and who wrote it, `GET
  /api/teams/{team_id}/agents/{agent_id}/revisions` and the matching workflow
  route read it back, and `POST .../revisions/{revision}/restore` writes an
  earlier version's content back — which appends a new revision rather than
  erasing the ones after it. Saving without changing anything records nothing.
  Restoring a workflow leaves its `draft`/`published`/`archived` state alone, so
  bringing back an old definition cannot unpublish a workflow a team is running,
  and the definition is revalidated so a version whose agents were since deleted
  is refused rather than restored into a plan that cannot run. A workflow run
  records the workflow revision it expanded and each step records the agent
  revision it ran under. Existing agents and workflows are given a revision 1
  holding their current content on upgrade. Portal shows the history on the
  workflow page and on each agent, with restore in place.
