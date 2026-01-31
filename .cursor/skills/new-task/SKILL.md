---
name: new-task
description: Adds a new task to the BuildMax task dashboard (tasks/000-TOC.md) as a TODO item and creates a numbered task file under tasks/. Use when the user wants to create a new task, add a planned task, or start a new task document.
---

# New Task

Creates a new planned task: adds it to the TODO section of the task dashboard and creates a task file with correct numbering.

## Workflow

1. **Get the task title** – Use the short title the user provides (e.g. "TUI chat interface", "Persist session to disk").
2. **Determine the next task number** – List `tasks/*.md`. Find all files matching three digits + `.md` (e.g. `001.md`, `007.md`). Do **not** count `000-TOC.md` or `*-design.md`. Next number = max number + 1 (e.g. if 001–007 exist, next is 008).
3. **Update the TOC** – Edit `tasks/000-TOC.md`:
   - In the TOC, **every task row links to its task file** so users can open requirement details quickly. Use format: `| NNN | [Short title](NNN.md) |` (relative path `NNN.md`; TOC is in `tasks/`).
   - If the **TODO** section has only the placeholder `*(Planned tasks will be added here.)*`, replace it with a table and one row:
     ```markdown
     | # | Title |
     |---|-------|
     | NNN | [Short title](NNN.md) |
     ```
   - If **TODO** already has a table, append a row: `| NNN | [Short title](NNN.md) |`
4. **Create the task file** – Create `tasks/NNN.md` with:
   - **First line:** `# Task NNN: Short title` (use the same short title as in the TOC).
   - **Blank line.**
   - **Short description** – One or two sentences describing the task. Keep it brief; the user will add more requirements later.

## Task file format

```markdown
# Task NNN: Short title

Brief description of what the task is about. One or two sentences; user will fill in detailed requirements.
```

Do not add long sections, checklists, or design content—only the title and a short description.

## Summary

- Add one TODO row to `tasks/000-TOC.md` with the short title **as a link**: `| NNN | [Short title](NNN.md) |`.
- Create `tasks/NNN.md` with first line `# Task NNN: Short title` and a short description.
- Use the next available number (max existing NNN in `NNN.md` + 1).
- **TOC convention**: Finished and TODO task rows always use `[Title](NNN.md)` so the dashboard links to each task’s requirement details.
