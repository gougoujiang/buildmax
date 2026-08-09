# Project Tasks / Board Example Data

A task board export, for finding overdue work, counting by assignee or project,
and following status flow.

## Files

| File | Contents |
|---|---|
| `tasks.csv` | Task ID, project, assignee, status, due date, priority |

## Columns

- **task_id** — task ID
- **project** — project name
- **assignee** — owner
- **status** — `todo`, `in_progress`, `done`, or `overdue`
- **due_date** — due date
- **priority** — `high`, `medium`, or `low`

## Query ideas

- Overdue work: `due_date` in the past and `status` not `done`
- Task counts per project or per assignee
- Filter by status, such as in progress or to do
- Sort or group by priority
