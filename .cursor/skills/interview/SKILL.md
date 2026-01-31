---
name: interview
description: Takes a user's task description or requirement, clarifies the design through context and scope, and produces an implementation-ready task spec. Use when the user provides a vague task, feature request, or requirement; when they ask to clarify design or make something ready for implementation; or when they want to turn an idea into a concrete task document.
---

# Interview Requirements

## When to use

Apply this skill when:
- The user gives a short or vague task (e.g. "add basic agent loop", "we need auth")
- They ask to "clarify design", "make it ready for implementation", or "write a task spec"
- They want to turn a requirement into something an implementer (or agent) can execute without guessing

## Interview workflow

1. **Capture the raw requirement**  
   Use the user’s words. If they only gave a title, treat that as the requirement.

2. **Gather context**  
   From the codebase and project docs (e.g. AGENTS.md, README, existing tasks):
   - What already exists (APIs, packages, config)?
   - Where does this task fit (which package, which layer)?
   - What conventions or constraints apply (language, layout, testing)?

3. **Clarify scope**  
   Define what **is** and **is not** in this task:
   - One clear **goal** (one sentence).
   - **In scope**: concrete deliverables (types, functions, files, tests).
   - **Out of scope**: explicitly list follow-ups or adjacent work so the spec is bounded.

4. **Write the spec**  
   Produce a single task document (e.g. `tasks/NNN.md`) using the [spec template](template.md). Fill every section; no placeholders like "TBD" unless the user explicitly defers a decision.

5. **Add acceptance**  
   End with checkboxes the implementer can tick: each deliverable or constraint that must hold for the task to be "done".

## Spec structure (template)

Use this structure for the implementation-ready document. See [template.md](template.md) for a copy-paste template.

| Section    | Purpose |
|-----------|---------|
| **Title** | Task ID and short name (e.g. "Task 002 - Basic Agent Loop"). |
| **Goal**  | One sentence: what we achieve with this task. |
| **Context** | Bullet list: existing code/config, where this fits, what we’re building on. |
| **&lt;Concept&gt; (this task)** | Optional. Define ambiguous terms so "this task" has a single meaning (e.g. "agent loop = one iteration: input → LLM → output"). |
| **Deliverables** | Numbered list: concrete artifacts (structs, functions, files, tests). Be specific enough to implement without guessing. |
| **Out of scope** | Bullet list: what we are **not** doing in this task (e.g. "Tool calling", "TUI integration"). |
| **Acceptance** | Checkboxes: "Done when …" criteria the implementer can verify. |

## Clarification rules

- **Infer from code when possible.** Prefer reading `internal/`, config, and existing tasks over asking the user for details that are already in the repo.
- **One task, one goal.** If the requirement spans multiple features, either pick the first slice for this spec or split into multiple task files (002a, 002b or 003, 004).
- **No hidden scope.** If you’re not sure whether something is in or out, put it in "Out of scope" with a note like "Consider in a follow-up task."
- **Acceptance = testable.** Each acceptance item should be something you can check (e.g. "Unit test with mock LLM passes", "Config loads from YAML").

## Output

- **Primary**: One markdown file (e.g. `tasks/003.md`) containing the full spec, ready for implementation.
- **Optional**: Short summary in chat (goal + deliverables + out of scope) so the user can confirm before implementation starts.
