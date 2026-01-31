---
name: finish-task
description: Moves a task from the TODO section to the Finished section in the BuildMax task dashboard (tasks/000-TOC.md). Use when the user wants to mark a task as done, finish a task, or move a task from TODO to finished.
---

# Finish Task

Moves one task from the **TODO** section to the **Finished** section in `tasks/000-TOC.md`.

## Workflow

1. **Identify the task** – Use the task number the user gives (e.g. 008, 009). If the user names a title, match it to the corresponding row in the TODO table.
2. **Read the TOC** – Read `tasks/000-TOC.md` to get the current TODO and Finished sections.
3. **Find the task in TODO** – In the **TODO** section, find the table row for that task (e.g. `| 008 | Short title |`). If TODO has only the placeholder `*(Planned tasks will be added here.)*`, there is nothing to move; tell the user.
4. **Update the TOC**:
   - **Finished**: Append one row to the Finished table. Use link format so the dashboard links to the task file: `| NNN | [Title](NNN.md) |`. If the TODO row already has a link `[Title](NNN.md)`, use that; otherwise use the plain title to form `[Title](NNN.md)`.
   - **TODO**: Remove that row from the TODO table. If no rows remain in the TODO table, replace the table with the placeholder: `*(Planned tasks will be added here.)*`
5. **Write** – Save the updated `tasks/000-TOC.md`.

## TOC structure (reminder)

- **Finished** has a table: header `| # | Title |`, separator `|---|-------|`, then rows `| NNN | [Title](NNN.md) |` (each title links to the task file).
- **TODO** either has the same table format (with links) or the placeholder `*(Planned tasks will be added here.)*`

Only the specified task row is moved; all other rows stay unchanged.

## Summary

- Move exactly one task from TODO to Finished by number (and title).
- Add one row to the Finished table with link format: `| NNN | [Title](NNN.md) |`; remove that row from the TODO table.
- If TODO is empty after the move, use the placeholder again.
