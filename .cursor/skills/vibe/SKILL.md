---
name: vibe
description: "Full development lifecycle skill: create tasks, clarify requirements, design, implement, finish, detect code smells, and build knowledge base. Sub-commands: start, clarify, design, code, done, smell, kb. All artifacts stored under .vibe/ directory."
---

# Vibe — Development Lifecycle

A single skill that drives the full development loop: **start → clarify → design → code → done**.

All task documents live under the `.vibe/` directory at the project root.

## Sub-commands

| Command | Purpose |
|---------|---------|
| `/vibe start <desc>` | Create a new task |
| `/vibe clarify [id]` | Clarify requirements and produce implementation-ready spec |
| `/vibe design [id]` | Technical design document |
| `/vibe code [id]` | Implement from design |
| `/vibe done [id]` | Mark task finished |
| `/vibe smell [path]` | Detect code smells and propose refactors |
| `/vibe kb [topic]` | Organize and maintain codebase knowledge base |

## Resolving the target task

Many sub-commands accept an optional `[id]` (e.g. `021`). Resolution order:

1. If the user supplies an explicit task id, use it.
2. Otherwise, read `.vibe/000-TOC.md` and pick the **last row** in the **TODO** table — that is the "current task".
3. If no TODO rows exist, ask the user to specify a task id or create a new task first.

---

## `/vibe start` — Create New Task

Creates a new planned task: adds it to the TODO section of the dashboard and creates a task file.

### Workflow

1. **Get the task title** — Use the short description the user provides.
2. **Determine the next task number** — List `.vibe/*.md`. Find all files matching three digits + `.md` (e.g. `001.md`, `007.md`). Do **not** count `000-TOC.md` or `*-design.md`. Next number = max number + 1.
3. **Update the TOC** — Edit `.vibe/000-TOC.md`:
   - Every task row links to its task file: `| NNN | [Short title](NNN.md) |` (relative path; TOC is in `.vibe/`).
   - If the **TODO** section has only the placeholder `*(Planned tasks will be added here.)*`, replace it with a table and one row:
     ```markdown
     | # | Title |
     |---|-------|
     | NNN | [Short title](NNN.md) |
     ```
   - If **TODO** already has a table, append a row.
4. **Create the task file** — Create `.vibe/NNN.md` with:
   - First line: `# Task NNN: Short title`
   - Blank line.
   - Short description — one or two sentences. User will add detail later.

### Task file format

```markdown
# Task NNN: Short title

Brief description of what the task is about.
```

Do not add long sections, checklists, or design content — only the title and a short description.

---

## `/vibe clarify` — Clarify Requirements

Takes a task and clarifies scope, context, and deliverables to produce an implementation-ready spec.

### When to use

- The task file (`.vibe/NNN.md`) has only a title and a short description
- The user wants to flesh out requirements before designing or coding

### Workflow

1. **Capture the raw requirement** — Read `.vibe/NNN.md` for the current content.
2. **Gather context** — From the codebase and project docs (AGENTS.md, README, existing code):
   - What already exists (APIs, packages, config)?
   - Where does this task fit (which package, which layer)?
   - What conventions or constraints apply?
3. **Clarify scope** — Define what **is** and **is not** in this task:
   - One clear **goal** (one sentence).
   - **In scope**: concrete deliverables.
   - **Out of scope**: explicitly list follow-ups.
4. **Write the spec** — Update `.vibe/NNN.md` using the [task spec template](templates/task-template.md). Fill every section; no "TBD" placeholders unless the user explicitly defers.
5. **Add acceptance** — End with checkboxes the implementer can verify.

### Clarification rules

- **Infer from code when possible.** Read `internal/`, config, and existing tasks rather than asking the user for details already in the repo.
- **One task, one goal.** If the requirement spans multiple features, pick the first slice or split tasks.
- **No hidden scope.** Uncertain items go to "Out of scope" with a note.
- **Acceptance = testable.** Each item should be verifiable.

### Output

- **Primary**: Updated `.vibe/NNN.md` containing the full spec, ready for design.
- **Optional**: Short summary in chat (goal + deliverables + out of scope) for confirmation.

---

## `/vibe design` — Technical Design

Produces a system design for the task: modules, structure, types, methods, and data flow.

### When to use

- The task spec (`.vibe/NNN.md`) is complete and ready for design
- The user wants to review *how* the task will be structured before coding

### Workflow

1. **Input** — Read `.vibe/NNN.md` as the source of truth. If only a title exists, infer scope from the codebase and project docs.
2. **Inspect codebase** — Existing packages under `internal/` and `cmd/`, current types, interfaces, and call patterns.
3. **Produce design** — Write `.vibe/NNN-design.md` using the [design template](templates/design-template.md). Fill: modules, structure, method design, and how they work together.
4. **Summarize changes** — End with a "Changes for review" section listing new/edited packages, files, types, and methods.

### Design rules

- **Align with task scope.** Only design what is in scope.
- **Respect project layout.** Follow AGENTS.md and existing `internal/` conventions.
- **Interfaces over concrete types** where a component is injected or mocked.
- **One place per responsibility.** No cross-package duplication.
- **Method design = contract.** Signatures must be specific enough to implement and test.

### Output

- **Primary**: `.vibe/NNN-design.md` with modules, structure, method design, flows, and "Changes for review".
- **Optional**: Short summary in chat for confirmation.

---

## `/vibe code` — Implement from Design

Takes the design document and writes the code.

### When to use

- The design (`.vibe/NNN-design.md`) has been reviewed and approved
- The user says "implement", "write the code", or "code it up"

### Workflow

1. **Input**
   - **Required**: `.vibe/NNN-design.md` (Goal, Modules, Structure, Method design, How they work together, Changes for review).
   - **Optional**: `.vibe/NNN.md` for deliverables, out-of-scope, and acceptance criteria.
2. **Resolve order** — Implement in dependency order: interfaces and types first, then constructors, then methods. Follow "How they work together" for call direction.
3. **Write code** — Match Method design exactly: receiver, name, signature, and responsibility. Follow AGENTS.md conventions.
4. **Tests** — Add tests as specified. No real API calls in tests. Table-driven tests for multiple cases.
5. **Verify**
   - Build: `go build ./...`
   - Tests: `go test ./...`
   - Confirm each acceptance criterion.

### Rules

- **Design is the contract.** Implement only what appears in the design and task spec.
- **Match signatures.** Method names, parameters, and return types must match.
- **No new dependencies** unless the design or task explicitly introduces them.
- **Tests are required** where the task or design says so.

### Output

- **Primary**: Code changes — new or modified files, plus tests. All acceptance criteria addressed.
- **Optional**: Short summary in chat (files touched, main types/methods, test status).

---

## `/vibe done` — Finish Task

Moves one task from the **TODO** section to the **Finished** section in `.vibe/000-TOC.md`.

### Workflow

1. **Identify the task** — Use the resolved task id (explicit or current).
2. **Read the TOC** — Read `.vibe/000-TOC.md`.
3. **Find the task in TODO** — Locate the row. If TODO has only the placeholder, there is nothing to move; tell the user.
4. **Update the TOC**:
   - **Finished**: Append one row with link format: `| NNN | [Title](NNN.md) |`.
   - **TODO**: Remove that row. If no rows remain, replace the table with the placeholder: `*(Planned tasks will be added here.)*`
5. **Write** — Save the updated `.vibe/000-TOC.md`.

### TOC structure

- **Finished** table: `| # | Title |` header, then rows `| NNN | [Title](NNN.md) |`.
- **TODO** either has the same table format or the placeholder `*(Planned tasks will be added here.)*`.

Only the specified task row is moved; all other rows stay unchanged.

---

## `/vibe smell` — Detect Code Smells

Analyzes code for smells and refactor opportunities. Produces a written proposal document — does **not** implement the refactors.

### When to use

- The user asks for a refactor review, code smell detection, or refactor opportunities
- They want to analyze recent implementations or code structure and get concrete proposals

### Workflow

1. **Scope** — Determine what to analyze:
   - If the user specifies a path or package, use that.
   - If unspecified, focus on recently modified or added code under `internal/` and `cmd/`.
2. **Inspect** — Read the relevant files and call graphs. Note: package boundaries, coupling, duplication, naming, error handling, test coverage, and alignment with AGENTS.md.
3. **Detect smells** — Look for:
   - Long functions, large types, duplicated logic
   - Unclear names, deep nesting
   - Missing interfaces for testability
   - Mixed concerns, dead or redundant code
   - For each smell: note **where** (file/line or symbol) and **why** it's a problem.
4. **Proposals** — For each refactor opportunity, write one proposal with:
   - **Title**: Short name (e.g. "Extract X", "Introduce interface Y").
   - **Location**: File(s) and symbols.
   - **Current state**: What the code does now and why it's a smell.
   - **Proposed change**: What to do (extract, rename, split, introduce interface, etc.).
   - **Benefit**: Readability, testability, maintainability, or consistency.
   - **Priority** (optional): High / Medium / Low.
5. **Write proposal file** — Produce a single markdown file using the [smell proposal template](templates/smell-template.md).
   - Default path: `.vibe/smell-YYYYMMDD.md` or a path the user specifies.

### Rules

- **Propose, don't implement.** Output is the proposal file only; no code changes.
- **Be specific.** Each proposal must point to concrete locations and describe the change in enough detail to implement later.
- **Explain the smell.** State why the current code is a problem.
- **Respect project conventions.** Proposals should align with AGENTS.md and existing style.
- **One file per run.** Write exactly one proposal document per invocation.

### Output

- **Primary**: One markdown file (e.g. `.vibe/smell-20260210.md`) with scope, summary, and numbered proposals.
- **Optional**: Short summary in chat (number of proposals, high-priority items).

---

## `/vibe kb` — Codebase Knowledge Base

Analyzes the codebase and produces organized knowledge documents under `.vibe/kb/`. Each document covers one topic — a package, a subsystem, a pattern, or an architectural concept.

### When to use

- The user wants to document how a part of the codebase works
- They say "document X", "explain package Y", "build knowledge base", or just `/vibe kb`
- They want a new team member (or future AI agent) to quickly understand the project

### Topic resolution

1. If the user specifies a topic (e.g. `/vibe kb agent loop`, `/vibe kb internal/tools`), produce or update the document for that topic.
2. If no topic is given (`/vibe kb`), scan the codebase and produce an **index** (`.vibe/kb/index.md`) listing all packages and subsystems, with links to individual topic files. Then ask which topics to expand, or generate the most important ones.

### Workflow

1. **Identify topic** — Determine the scope: a package (e.g. `internal/agent`), a subsystem (e.g. "tool calling"), a cross-cutting concern (e.g. "configuration"), or the full project overview.
2. **Inspect codebase** — Read the relevant source files, types, interfaces, and call patterns. Also check AGENTS.md and existing `.vibe/kb/` docs to avoid duplication.
3. **Write the knowledge doc** — Create or update `.vibe/kb/<topic-slug>.md` using the [knowledge template](templates/kb-template.md). Fill: purpose, key types, how it works, dependencies, and examples.
4. **Update the index** — If `.vibe/kb/index.md` exists, add or update the entry for this topic. If it doesn't exist, create it with at least this one entry.

### Naming convention

- File names use kebab-case slugs derived from the topic: e.g. `agent-loop.md`, `llm-client.md`, `tool-calling.md`, `project-overview.md`.
- The index file is always `.vibe/kb/index.md`.

### Rules

- **Accuracy over volume.** Only document what the code actually does; don't speculate.
- **Keep it current.** If a doc already exists, update it rather than creating a duplicate.
- **One topic per file.** Each file covers one cohesive subject.
- **Link between docs.** Use relative links (e.g. `[LLM client](llm-client.md)`) to cross-reference related topics.
- **Code examples welcome.** Include short code snippets or signatures to illustrate key points, but don't dump entire files.

### Output

- **Primary**: One or more markdown files under `.vibe/kb/` plus an updated `index.md`.
- **Optional**: Short summary in chat listing which docs were created or updated.
